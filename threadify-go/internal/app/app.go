package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"
	sharedrepo "threadify-go/shared/repository"

	"github.com/threadify/engine/internal/archiver"
	"github.com/threadify/engine/internal/broker"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/dashboard"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/graphql"
	"github.com/threadify/engine/internal/graphql/generated"
	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/managedvalkey"
	"github.com/threadify/engine/internal/middleware"
	"github.com/threadify/engine/internal/perf"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/workerpool"
	"threadify-go/shared/registry"
)

type App struct {
	closeOnce sync.Once
	closeErr  error
	Handler   http.Handler
	infra     *infra
	svcs      *services
	hdlrs     *appHandlers
	logger    *zap.Logger
	cfg       *config.Config
}

type infra struct {
	cleanupOnce   sync.Once
	valkeyRuntime *managedvalkey.Runtime
	registry      *registry.Runtime
	broker        *broker.Runtime
	persistence   *archiver.Runtime
	db            *database.PostgresDB
	valkey        *database.ValkeyService
	natsPool      *natsrepo.Pool
	workerPools   *workerpool.Pools
	shuttingDown  atomic.Bool
}

func (i *infra) close() {
	i.cleanupOnce.Do(func() {
		i.registry.Close()
		if i.persistence != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = i.persistence.Close(ctx)
			cancel()
		}
		if i.workerPools != nil {
			i.workerPools.ShutdownNow()
		}
		if i.natsPool != nil {
			i.natsPool.Close()
		}
		if i.broker != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = i.broker.Close(ctx)
			cancel()
		}
		if i.valkey != nil {
			i.valkey.Close()
		}
		if i.valkeyRuntime != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = i.valkeyRuntime.Close(ctx)
			cancel()
		}
		if i.db != nil {
			i.db.Close()
		}
	})
}

type services struct {
	manager          *service.ServiceManager
	auth             *service.AuthService
	thread           *service.ThreadService
	otelTrace        *service.OTelTraceService
	stepEvent        *service.StepEventService
	threadAccess     *service.ThreadAccessService
	plan             domain.PlanService
	contract         *service.ContractService
	invitation       *service.InvitationTokenService
	luaScriptManager *valkey.LuaScriptManager
	rbacLoader       *rbac.Loader
}

type appHandlers struct {
	wsHandler       *handlers.WebSocketHandler
	otlpTrace       *handlers.OTLPTraceHandler
	contractHandler *handlers.ContractHandler
	notifRouter     *handlers.NotificationRouter
	graphqlResolver *graphql.Resolver
}

func (s *services) stopAll(ctx context.Context) error {
	if s == nil {
		return nil
	}
	var result error
	if s.thread != nil {
		result = errors.Join(result, s.thread.StopContext(ctx), s.thread.WaitBackground(ctx))
	}
	if s.auth != nil {
		s.auth.Stop()
	}
	if s.manager != nil {
		result = errors.Join(result, s.manager.StopAllContext(ctx))
	}
	return result
}

func (s *services) cleanup() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.stopAll(ctx)
}

func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (_ *App, retErr error) {
	inf, err := initInfra(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			inf.close()
		}
	}()
	licensed, err := registry.Start(ctx, cfg.Registry, inf.db.Pool, inf.natsPool.GetClient().JetStream())
	if err != nil {
		return nil, err
	}
	inf.registry = licensed
	registry.SetDefault(licensed)
	if cfg.RuntimeMode == "writer" {
		metricsRepo := postgres.NewMetricsRepository(inf.db.Pool, inf.valkey, logger)
		if err := startPersistence(ctx, cfg, inf, metricsRepo, logger); err != nil {
			return nil, err
		}
		router := gin.New()
		router.Use(gin.Recovery())
		router.GET("/health", healthHandler(inf))
		router.GET("/metrics", gin.WrapH(promhttp.Handler()))
		return &App{Handler: router, infra: inf, logger: logger, cfg: cfg}, nil
	}

	rbacLoader, err := loadRBAC()
	if err != nil {
		return nil, fmt.Errorf("load rbac: %w", err)
	}

	luaScriptManager := valkey.NewLuaScriptManager(inf.valkey)
	if err := luaScriptManager.LoadScripts(ctx); err != nil {
		return nil, fmt.Errorf("load lua scripts: %w", err)
	}

	repos, err := initRepositories(cfg, inf, luaScriptManager, rbacLoader, logger)
	if err != nil {
		return nil, err
	}

	if cfg.RuntimeMode != "engine" && cfg.Archiver.Enabled {
		if err := startPersistence(ctx, cfg, inf, repos.metrics, logger); err != nil {
			return nil, err
		}
	}

	svcs, err := initServices(ctx, cfg, inf, repos, luaScriptManager, rbacLoader, logger)
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			svcs.cleanup()
		}
	}()
	hdlrs, err := initHandlers(cfg, inf, svcs, repos, logger)
	if err != nil {
		return nil, err
	}

	browser, err := sharedauth.NewBrowserService(ctx, inf.db.Pool, licensed)
	if err != nil {
		return nil, fmt.Errorf("initialize browser authentication: %w", err)
	}
	sharedauth.SetBrowserService(browser)
	ingestionRules := valkey.NewIngestionRules(inf.valkey)
	hdlrs.otlpTrace.WithIngestionRules(ingestionRules)
	router := browser.IngestionRulesHandler(ingestionRules, browser.EngineSettingsHandler(cfg.Server.PublicURL, browser.UserManagement(buildRouter(cfg, inf, svcs, repos, hdlrs, logger))))
	return &App{
		Handler: dashboard.Wrap(browser.Wrap(licensed.WrapEngine(router))),
		infra:   inf,
		svcs:    svcs,
		hdlrs:   hdlrs,
		logger:  logger,
		cfg:     cfg,
	}, nil
}

func startPersistence(ctx context.Context, cfg *config.Config, inf *infra, metrics archiver.MetricsInvalidator, logger *zap.Logger) error {
	runtime, err := archiver.NewRuntime(inf.natsPool.GetClient().JetStream(), inf.db.Pool, metrics, cfg, logger)
	if err != nil {
		return fmt.Errorf("initialize persistence: %w", err)
	}
	inf.persistence = runtime
	if err := runtime.Start(ctx); err != nil {
		return fmt.Errorf("start persistence: %w", err)
	}
	return nil
}

func (a *App) BeginShutdown() { a.infra.shuttingDown.Store(true) }

func (a *App) Close(ctx context.Context) error {
	a.closeOnce.Do(func() {
		a.infra.shuttingDown.Store(true)
		if a.hdlrs != nil && a.hdlrs.wsHandler != nil {
			a.closeErr = errors.Join(a.closeErr, a.hdlrs.wsHandler.Shutdown(ctx))
		}
		if a.hdlrs != nil && a.hdlrs.notifRouter != nil {
			_ = a.hdlrs.notifRouter.Stop()
		}
		a.closeErr = errors.Join(a.closeErr, a.svcs.stopAll(ctx))
		// Producers finish while the broker and persistence consumers are still live.
		if a.infra.workerPools != nil {
			pools := a.infra.workerPools
			for _, pool := range []*workerpool.Pool{pools.Validation, pools.Activity, pools.Notification, pools.Archival, pools.WriteBack} {
				a.closeErr = errors.Join(a.closeErr, pool.Shutdown(ctx))
			}
		}
		if a.infra.persistence != nil {
			a.closeErr = errors.Join(a.closeErr, a.infra.persistence.Close(ctx))
		}
		if a.infra.registry != nil {
			a.infra.registry.Close()
		}
		if a.infra.natsPool != nil {
			a.closeErr = errors.Join(a.closeErr, a.infra.natsPool.Drain(ctx))
		}
		if a.infra.broker != nil {
			a.closeErr = errors.Join(a.closeErr, a.infra.broker.Close(ctx))
		}
		if a.infra.valkey != nil {
			_ = a.infra.valkey.Close()
		}
		if a.infra.valkeyRuntime != nil {
			a.closeErr = errors.Join(a.closeErr, a.infra.valkeyRuntime.Close(ctx))
		}
		a.infra.close()
	})
	return a.closeErr
}

func (a *App) Valkey() *database.ValkeyService {
	return a.infra.valkey
}

func initInfra(ctx context.Context, cfg *config.Config, logger *zap.Logger) (_ *infra, retErr error) {
	inf := &infra{}
	defer func() {
		if retErr != nil {
			inf.close()
		}
	}()

	db, err := database.NewPostgresDB(cfg.Postgres.URL, cfg.Postgres.MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	inf.db = db

	perf.Initialize(cfg.Performance.MonitoringEnabled, logger)
	if cfg.Performance.MonitoringEnabled {
		logger.Info("performance monitoring enabled")
	}

	if err := db.InitSchema(ctx); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	if err := db.InitDefaultMetrics(ctx); err != nil {
		return nil, fmt.Errorf("init default metrics: %w", err)
	}

	ownedValkey, err := managedvalkey.Start(ctx, managedvalkey.Options{
		Mode: cfg.Redis.Mode, URL: cfg.Redis.URL, Bind: cfg.Redis.Bind,
		StoreDir: cfg.Redis.StoreDir, BinaryPath: cfg.Redis.BinaryPath,
		StartupTimeout: time.Duration(cfg.Redis.StartupTimeoutSeconds) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("start Valkey: %w", err)
	}
	inf.valkeyRuntime = ownedValkey
	valkeyService, err := database.NewValkeyService(
		cfg.Redis.URL,
		cfg.Redis.PoolSize,
		cfg.Redis.MinIdleConns,
		cfg.Redis.MaxIdleConns,
		cfg.Redis.MaxRetries,
		cfg.Redis.DialTimeoutMs,
		cfg.Redis.ReadTimeoutMs,
		cfg.Redis.WriteTimeoutMs,
		cfg.Redis.PoolTimeoutMs,
		cfg.Redis.ConnMaxIdleTimeMs,
	)
	if err != nil {
		return nil, fmt.Errorf("connect valkey: %w", err)
	}
	inf.valkey = valkeyService

	embedded, err := broker.Start(ctx, broker.Options{
		Mode: cfg.NATS.Mode, StoreDir: cfg.NATS.StoreDir,
		MaxMemoryBytes: cfg.NATS.MaxMemoryBytes, MaxStoreBytes: cfg.NATS.MaxStoreBytes,
	})
	if err != nil {
		return nil, fmt.Errorf("start broker: %w", err)
	}
	inf.broker = embedded
	natsPool, err := natsrepo.NewPool(&cfg.NATS, cfg.NATS.PoolSize, logger, embedded.ClientOptions()...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	inf.natsPool = natsPool

	inf.workerPools = workerpool.NewPools(workerpool.NewPrometheusMetrics())
	logger.Info("worker pools initialized",
		zap.Int32("validation", inf.workerPools.Validation.Stats().TotalWorkers),
		zap.Int32("notification", inf.workerPools.Notification.Stats().TotalWorkers),
		zap.Int32("writeback", inf.workerPools.WriteBack.Stats().TotalWorkers),
		zap.Int32("activity", inf.workerPools.Activity.Stats().TotalWorkers),
	)

	return inf, nil
}

// repositories holds all data-access objects — no business logic.
type repositories struct {
	// postgres
	auth              *postgres.AuthRepository
	thread            *postgres.ThreadRepository
	stepState         *postgres.StepStateRepository
	validation        *postgres.ValidationRepository
	contract          *postgres.ContractRepository
	refs              *postgres.ThreadRefsRepository
	activity          *postgres.ActivityRepository
	actor             *postgres.ActorRepository
	notification      *postgres.ThreadNotificationRepository
	subStep           *postgres.SubStepRepository
	entityProfile     *sharedrepo.EntityProfileRepo
	entityProfileType sharedrepo.EntityProfileTypeRepository
	metrics           *postgres.MetricsRepository

	// valkey
	threadCache     *valkey.ThreadRepository
	stepStateCache  *valkey.StepStateRepository
	validationCache *valkey.ValidationRepository
	access          *valkey.AccessRepository
	otelTrace       *valkey.OTelTraceRepository

	// nats
	natsArchival     *natsrepo.ArchivalPublisher
	natsNotification *natsrepo.Publisher
}

func initRepositories(
	cfg *config.Config,
	inf *infra,
	luaScriptManager *valkey.LuaScriptManager,
	rbacLoader domain.RBACLoader,
	logger *zap.Logger) (*repositories, error) {
	r := &repositories{}

	// --- postgres ---
	r.auth = postgres.NewAuthRepository(inf.db.Pool)
	r.thread = postgres.NewThreadRepository(inf.db.Pool)
	r.stepState = postgres.NewStepStateRepository(inf.db.Pool)
	r.validation = postgres.NewValidationRepository(inf.db.Pool)
	r.contract = postgres.NewContractRepository(inf.db.Pool)
	r.refs = postgres.NewThreadRefsRepository(inf.db.Pool)
	r.activity = postgres.NewActivityRepository(inf.db.Pool, cfg)
	r.actor = postgres.NewActorRepository(inf.db.Pool)
	r.notification = postgres.NewThreadNotificationRepository(inf.db.Pool)
	r.subStep = postgres.NewSubStepRepository(inf.db.Pool)
	r.entityProfile = sharedrepo.NewEntityProfileRepo(inf.db.Pool)
	r.entityProfileType = sharedrepo.NewEntityProfileTypeRepository(inf.db.Pool)
	r.metrics = postgres.NewMetricsRepository(inf.db.Pool, inf.valkey, logger)

	// --- valkey ---
	threadTTL := int(time.Duration(cfg.Cache.ThreadTTLMs) * time.Millisecond / time.Second)
	stepEventTTL := int(time.Duration(cfg.Cache.StepEventTTLMs) * time.Millisecond / time.Second)

	cacheManager := service.NewCacheService(logger)

	threadCache := valkey.NewThreadRepository(
		threadTTL, inf.valkey,
		r.thread, r.stepState,
		cacheManager, luaScriptManager, logger,
	)
	threadCache.SetWriteBackPool(inf.workerPools.WriteBack)
	r.threadCache = threadCache

	r.stepStateCache = valkey.NewStepStateRepositoryWithPostgres(stepEventTTL, inf.valkey, r.stepState, logger)

	r.validationCache = valkey.NewValidationRepositoryWithPostgres(inf.valkey, r.validation, logger)

	accessRepo := valkey.NewAccessRepository(inf.valkey, threadTTL, logger)
	accessRepo.SetRBACLoader(rbacLoader)
	r.access = accessRepo
	r.otelTrace = valkey.NewOTelTraceRepository(inf.valkey, 24*time.Hour, 30*time.Second)

	// --- nats ---
	natsClient := inf.natsPool.GetClient()
	r.natsArchival = natsrepo.NewArchivalPublisher(natsClient, logger)
	r.natsNotification = natsrepo.NewPublisher(natsClient)

	return r, nil
}

func initServices(
	ctx context.Context,
	cfg *config.Config,
	inf *infra,
	repos *repositories,
	luaScriptManager *valkey.LuaScriptManager,
	rbacLoader *rbac.Loader,
	logger *zap.Logger,
) (_ *services, retErr error) {
	svcs := &services{
		luaScriptManager: luaScriptManager,
		rbacLoader:       rbacLoader,
	}
	defer func() {
		if retErr != nil {
			svcs.cleanup()
		}
	}()
	sm := service.NewServiceManager(logger)
	svcs.manager = sm

	// --- auth ---
	authSvc := service.NewAuthService(repos.auth, cfg.Auth.CacheTTLSeconds)
	svcs.auth = authSvc
	authSvc.SetWriteBackPool(inf.workerPools.WriteBack)
	svcs.auth = authSvc

	// --- step events ---
	stepEventSvc, err := service.NewStepEventService(
		inf.valkey, repos.threadCache, repos.natsArchival, cfg, logger,
	)
	if err != nil {
		return nil, fmt.Errorf("create step event service: %w", err)
	}
	sm.Register(stepEventSvc)
	svcs.stepEvent = stepEventSvc

	// --- thread access ---
	svcs.threadAccess = service.NewThreadAccessService(
		repos.access, service.NewCacheService(logger),
		luaScriptManager, rbacLoader, logger,
	)

	// Registry supplies all limits; no local credit billing services are started.
	planSvc := service.NewRegistryPlanService()
	svcs.plan = planSvc

	// --- contract ---
	svcs.contract = service.NewContractService(repos.contract, planSvc, logger)

	// --- thread ---
	natsClient := inf.natsPool.GetClient()
	contractTTL := int(time.Duration(cfg.Cache.ContractTTLMs) * time.Millisecond / time.Second)
	svcs.thread = service.NewThreadService(cfg,
		inf.db, inf.valkey, stepEventSvc,
		repos.threadCache, contractTTL,
		natsClient,
		repos.natsNotification, repos.natsArchival,
		authSvc, planSvc, service.NewCacheService(logger),
		inf.workerPools, logger,
	)
	svcs.otelTrace = service.NewOTelTraceService(svcs.thread, repos.otelTrace, logger)

	svcs.invitation = service.NewInvitationTokenService(cfg.JWT.Secret, cfg.JWT.Issuer)

	if err := sm.StartAll(); err != nil {
		return nil, fmt.Errorf("start background services: %w", err)
	}

	return svcs, nil
}

func initHandlers(
	cfg *config.Config,
	inf *infra,
	svcs *services,
	repos *repositories,
	logger *zap.Logger,
) (*appHandlers, error) {
	h := &appHandlers{}

	h.contractHandler = handlers.NewContractHandler(svcs.contract, logger)

	natsClient := inf.natsPool.GetClient()
	notifRouter, err := handlers.NewNotificationRouter(natsClient.Conn(), &cfg.NATS, logger)
	if err != nil {
		return nil, fmt.Errorf("create notification router: %w", err)
	}
	svcs.manager.Register(notifRouter)
	h.notifRouter = notifRouter

	h.wsHandler = handlers.NewWebSocketHandler(
		svcs.thread, svcs.stepEvent, svcs.invitation,
		svcs.thread.GetNotificationConsumer(), notifRouter,
		svcs.plan, inf.valkey, svcs.luaScriptManager,
		&cfg.WebSocket, logger,
	)
	h.otlpTrace = handlers.NewOTLPTraceHandler(svcs.otelTrace, svcs.auth, svcs.plan, logger)

	h.graphqlResolver = graphql.NewResolver(
		repos.threadCache, repos.stepStateCache, repos.validationCache, repos.access,
		svcs.threadAccess, svcs.thread.GetContractValidator(), repos.contract,
		repos.refs, repos.stepState, repos.activity, repos.actor,
		repos.notification, repos.subStep,
		repos.entityProfile, repos.entityProfileType, repos.metrics,
		svcs.plan, logger,
	)

	return h, nil
}

func loadRBAC() (*rbac.Loader, error) {
	return rbac.NewEmbeddedLoader()
}

func buildRouter(cfg *config.Config, inf *infra, svcs *services, repos *repositories, hdlrs *appHandlers, logger *zap.Logger) http.Handler {
	gqlHandler := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: hdlrs.graphqlResolver}))
	gqlHandler.Use(extension.FixedComplexityLimit(1000))
	gqlHandler.Use(extension.Introspection{})
	gqlHandler.SetErrorPresenter(sanitizeGraphQLError)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(shutdownGuard(&inf.shuttingDown))
	r.Use(requestLogger(logger))
	r.Use(middleware.PrometheusMiddleware())

	// Infrastructure endpoints.
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", healthHandler(inf))

	// WebSocket.
	r.GET("/threads", hdlrs.wsHandler.HandleWebSocket)

	// Standard OTLP/HTTP trace ingestion (API key authentication is handled by
	// the OTLP handler so protocol errors remain protobuf-encoded).
	r.POST("/v1/traces", hdlrs.otlpTrace.HandleTraces)

	// GraphQL.
	r.POST("/graphql",
		middleware.AuthMiddleware(svcs.auth, middleware.AuthDual),
		middleware.CreditUsageMiddleware(svcs.plan, logger),
		middleware.EgressMiddleware(svcs.plan, logger),
		graphqlMiddleware(gqlHandler),
	)
	r.GET("/graphql/playground", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))

	// MCP.
	sseGroup := r.Group("/sse")
	sseGroup.Use(middleware.AuthMiddleware(svcs.auth, middleware.AuthAPIKey))
	sseGroup.Use(middleware.CreditUsageMiddleware(svcs.plan, logger))
	mountMCPServer(sseGroup, cfg, svcs.plan, logger)

	// Public v1.
	v1Public := r.Group("/v1")
	v1Public.GET("/pricing", pricingHandler(cfg))

	// Authenticated v1.
	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware(svcs.auth, middleware.AuthDual))
	v1.Use(middleware.EgressMiddleware(svcs.plan, logger))
	mountContractRoutes(v1, hdlrs, svcs.rbacLoader, svcs.plan, logger)
	mountManagementRoutes(v1, inf, repos, svcs.rbacLoader, logger)
	v1.POST("/entity-profile-types", middleware.CreditUsageMiddleware(svcs.plan, logger),
		middleware.ContractRBACMiddleware(svcs.rbacLoader, "entity_profile_type.create"),
		handlers.CreateProfileType(repos.entityProfileType))
	v1.PUT("/entity-profiles", middleware.CreditUsageMiddleware(svcs.plan, logger),
		middleware.ContractRBACMiddleware(svcs.rbacLoader, "entity_profile_type.update"),
		handlers.PutProfile(repos.entityProfileType, repos.entityProfile))

	return r
}

func mountContractRoutes(rg *gin.RouterGroup, hdlrs *appHandlers, rbac domain.RBACLoader, plan domain.PlanService, logger *zap.Logger) {
	ch := hdlrs.contractHandler

	contracts := rg.Group("/contracts")
	readGuard := middleware.ContractRBACMiddleware(rbac, "contract.read.*")
	writeGuard := middleware.ContractRBACMiddleware(rbac, "contract.update.*")
	creditGuard := middleware.CreditUsageMiddleware(plan, logger)

	contracts.GET("", readGuard, ch.GetAllContracts)
	contracts.GET("/:id", readGuard, ch.GetContract)
	contracts.GET("/:id/versions", readGuard, ch.GetAllContractVersions)
	contracts.GET("/:id/versions/:version", readGuard, ch.GetContractVersion)
	contracts.POST("/preview", readGuard, ch.PreviewContract)
	contracts.DELETE("/:id", writeGuard, ch.DeleteContract)
	contracts.DELETE("/:id/versions/:version", writeGuard, ch.DeleteContractVersion)
	contracts.POST("", creditGuard, middleware.ContractRBACMiddleware(rbac, "contract.create"), ch.CreateContract)
	contracts.PUT("/:id", creditGuard, writeGuard, ch.UpdateContract)
}

func pricingHandler(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot, err := registry.Default().Snapshot()
		if err != nil {
			c.JSON(registry.StatusCode(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"billing_source": "registry", "limits": snapshot.Entitlements})
	}
}

func healthHandler(inf *infra) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		status := http.StatusOK
		response := gin.H{
			"status":   "ok",
			"postgres": "ok",
			"valkey":   "ok",
		}
		if err := inf.db.Pool.Ping(ctx); err != nil {
			status = http.StatusServiceUnavailable
			response["postgres"] = "error"
		}
		if err := inf.valkey.Ping(ctx); err != nil || !inf.valkeyRuntime.IsHealthy() {
			status = http.StatusServiceUnavailable
			response["valkey"] = "error"
		}
		if inf.natsPool == nil || !inf.natsPool.IsHealthy() || (inf.broker != nil && !inf.broker.IsHealthy()) {
			status = http.StatusServiceUnavailable
			response["nats"] = "error"
		} else {
			response["nats"] = "ok"
		}
		if inf.persistence != nil {
			response["persistence"] = "ok"
			if !inf.persistence.IsHealthy() {
				status = http.StatusServiceUnavailable
				response["persistence"] = "error"
			}
		} else {
			response["persistence"] = "disabled"
		}
		if inf.shuttingDown.Load() {
			status = http.StatusServiceUnavailable
		}
		if status != http.StatusOK {
			response["status"] = "unavailable"
		}
		c.JSON(status, response)
	}
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

func shutdownGuard(shuttingDown *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shuttingDown.Load() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "server is shutting down"})
			return
		}
		c.Next()
	}
}

func graphqlMiddleware(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		for _, key := range []any{
			sharedauth.CtxUserID,
			sharedauth.CtxCompanyID,
			sharedauth.CtxRoles,
			sharedauth.CtxAuthUserID,
			sharedauth.CtxEmail,
			sharedauth.CtxAuthSub,
		} {
			if v, ok := c.Get(key.(string)); ok {
				ctx = context.WithValue(ctx, key, v)
			}
		}
		h.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	}
}

var internalErrorPatterns = []string{
	"SQLSTATE",
	"violates foreign key",
	"connection refused",
	"threadify-go/",
	"supabase.co",
	"plunk.so",
	"api.plunk",
	"useplunk",
}

func sanitizeGraphQLError(ctx context.Context, err error) *gqlerror.Error {
	msg := err.Error()
	for _, pattern := range internalErrorPatterns {
		if strings.Contains(strings.ToLower(msg), strings.ToLower(pattern)) {
			return gqlerror.Errorf("An internal error occurred. Please try again or contact support.")
		}
	}
	return gqlerror.Wrap(err)
}

func LoadConfig() (*config.Config, error) {
	return LoadConfigPath(os.Getenv("CONFIG_PATH"))
}

// LoadConfigPath loads the selected Engine configuration independently of the
// source working directory. Plan allowances come exclusively from Registry.
func LoadConfigPath(path string) (*config.Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if path != "" {
		v.SetConfigFile(path)
	} else {
		for _, dir := range []string{"./config", "../../config", "../../../config"} {
			v.AddConfigPath(dir)
		}
		v.SetConfigName("config")
	}
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return config.LoadFromViper(v)
}
