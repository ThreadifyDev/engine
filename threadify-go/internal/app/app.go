package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
	"threadify-go/shared/rbac"
	sharedrepo "threadify-go/shared/repository"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/graphql"
	"github.com/threadify/engine/internal/graphql/generated"
	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/middleware"
	"github.com/threadify/engine/internal/perf"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/workerpool"
)

type App struct {
	Handler http.Handler
	deps    *deps
	logger  *zap.Logger
	cfg     *config.Config
}

type deps struct {
	db             *database.PostgresDB
	valkey         *database.ValkeyService
	natsPool       *natsrepo.Pool
	workerPools    *workerpool.Pools
	serviceManager *service.ServiceManager
	shuttingDown   atomic.Bool
}

func (d *deps) close() {
	if d.serviceManager != nil {
		d.serviceManager.StopAll()
	}
	if d.natsPool != nil {
		d.natsPool.Close()
	}
	if d.valkey != nil {
		d.valkey.Close()
	}
	if d.db != nil {
		d.db.Close()
	}
}

func New(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*App, error) {
	d, err := initDependencies(cfg, logger)
	if err != nil {
		return nil, err
	}

	handler := buildRouter(cfg, d, logger)

	return &App{
		Handler: handler,
		deps:    d,
		logger:  logger,
		cfg:     cfg,
	}, nil
}

func (a *App) Run() error {
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", a.cfg.Server.Host, a.cfg.Server.Port),
		Handler:      a.Handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	a.logger.Info("starting server", zap.String("address", srv.Addr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *App) Valkey() *database.ValkeyService {
	return a.deps.valkey
}

func (a *App) Close(ctx context.Context) error {
	a.deps.shuttingDown.Store(true)
	a.deps.close()
	return nil
}

func initDependencies(cfg *config.Config, logger *zap.Logger) (*deps, error) {
	d := &deps{}

	db, err := database.NewPostgresDB(cfg.Postgres.URL, cfg.Postgres.MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	d.db = db

	perf.Initialize(cfg.Performance.MonitoringEnabled, logger)
	if cfg.Performance.MonitoringEnabled {
		logger.Info("performance monitoring enabled")
	}

	if err := db.InitSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	valkeyService, err := database.NewValkeyService(
		cfg.Redis.Host,
		cfg.Redis.Port,
		cfg.Redis.Password,
		cfg.Redis.DB,
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
	d.valkey = valkeyService

	natsPool, err := natsrepo.NewPool(&cfg.NATS, cfg.NATS.PoolSize, logger)
	if err != nil {
		logger.Fatal("NATS unavailable — required for usage metering", zap.Error(err))
	}
	d.natsPool = natsPool

	d.workerPools = workerpool.NewPools(workerpool.NewPrometheusMetrics())
	logger.Info("worker pools initialized",
		zap.Int32("validation", d.workerPools.Validation.Stats().TotalWorkers),
		zap.Int32("notification", d.workerPools.Notification.Stats().TotalWorkers),
		zap.Int32("writeback", d.workerPools.WriteBack.Stats().TotalWorkers),
		zap.Int32("activity", d.workerPools.Activity.Stats().TotalWorkers),
	)

	return d, nil
}

func buildRouter(cfg *config.Config, d *deps, logger *zap.Logger) http.Handler {
	authRepo := postgres.NewAuthRepository(d.db.Pool)
	authSvc := service.NewAuthService(authRepo, cfg.Auth.CacheTTLSeconds)
	if cfg.JWKS.URL == "" {
		logger.Fatal("jwks.url not configured — JWT authentication unavailable")
	}
	authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
	authSvc.SetWriteBackPool(d.workerPools.WriteBack)

	rbacLoader, err := rbac.NewLoader("./shared/rbac/permissions.json", "./shared/rbac/roles.json")
	if err != nil {
		// Try fallback paths for tests or different execution context
		candidates := []string{"./shared/rbac", "../shared/rbac", "../../shared/rbac"}
		found := false
		for _, c := range candidates {
			rbacLoader, err = rbac.NewLoader(c+"/permissions.json", c+"/roles.json")
			if err == nil {
				found = true
				break
			}
		}
		if !found {
			logger.Fatal("failed to load RBAC roles", zap.Error(err))
		}
	}

	postgresThreadRepo := postgres.NewThreadRepository(d.db.Pool)
	stepStatePostgres := postgres.NewStepStateRepository(d.db.Pool)
	cacheManager := service.NewCacheService(logger)

	threadTTL := time.Duration(cfg.Cache.ThreadTTLMs) * time.Millisecond
	threadRepo := valkey.NewThreadRepository(d.valkey, int(threadTTL.Seconds()), postgresThreadRepo, stepStatePostgres, cacheManager, logger)
	threadRepo.SetWriteBackPool(d.workerPools.WriteBack)

	contractTTL := time.Duration(cfg.Cache.ContractTTLMs) * time.Millisecond
	stepEventTTL := time.Duration(cfg.Cache.StepEventTTLMs) * time.Millisecond

	postgresStepRepo := postgres.NewStepStateRepository(d.db.Pool)
	stepStateRepo := valkey.NewStepStateRepositoryWithPostgres(d.valkey, postgresStepRepo, int(stepEventTTL.Seconds()), logger)

	postgresValidationRepo := postgres.NewValidationRepository(d.db.Pool)
	validationRepo := valkey.NewValidationRepositoryWithPostgres(d.valkey, postgresValidationRepo, logger)

	contractRepo := postgres.NewContractRepository(d.db.Pool)
	refsRepo := postgres.NewThreadRefsRepository(d.db.Pool)
	activityRepo := postgres.NewActivityRepository(d.db.Pool, cfg)
	actorRepo := postgres.NewActorRepository(d.db.Pool)
	notificationRepo := postgres.NewThreadNotificationRepository(d.db.Pool)
	subStepRepo := postgres.NewSubStepRepository(d.db.Pool)

	accessRepo := valkey.NewAccessRepository(d.valkey, int(threadTTL.Seconds()), logger)
	accessRepo.SetRBACLoader(rbacLoader)
	luaScriptManager := valkey.NewLuaScriptManager(d.valkey)
	if err := luaScriptManager.LoadScripts(context.Background()); err != nil {
		logger.Fatal("failed to load lua scripts", zap.Error(err))
	}

	natsArchival := natsrepo.NewArchivalPublisher(d.natsPool.GetClient(), logger)
	natsNotification := natsrepo.NewPublisher(d.natsPool.GetClient())

	sm := service.NewServiceManager(logger)
	d.serviceManager = sm

	stepEventSvc, err := service.NewStepEventService(d.valkey, threadRepo, natsArchival, cfg, logger)
	if err != nil {
		logger.Fatal("failed to create step event service", zap.Error(err))
	}
	sm.Register(stepEventSvc)

	threadAccessSvc := service.NewThreadAccessService(accessRepo, cacheManager, luaScriptManager, rbacLoader, logger)

	planRepo := sharedrepo.NewPlanRepo(d.db.Pool)

	planSvc := service.NewPlanService(planRepo, contractRepo, actorRepo, &cfg.Subscription, d.valkey, luaScriptManager, logger, cfg.Cache.PlanTTLMs)
	usageOutboxRelay := service.NewUsageOutboxRelay(d.valkey, natsArchival, logger)
	sm.Register(usageOutboxRelay)

	billingRepo := postgres.NewBillingRepository(d.db.Pool)

	billingProvider, err := billing.InitializeProvider(cfg.Billing)
	if err != nil {
		logger.Fatal("failed to initialize billing provider", zap.Error(err))
	}

	billingOrchestrator := service.NewBillingOrchestrator(billingProvider, planRepo, billingRepo, &cfg.Subscription, &cfg.Billing, d.valkey, planSvc, logger)

	js, err := jetstream.New(d.natsPool.GetClient().Conn())
	if err != nil {
		logger.Fatal("failed to initialize NATS JetStream for billing", zap.Error(err))
	}

	billingCron := service.NewBillingCron(billingOrchestrator, d.valkey, js, logger)
	sm.Register(billingCron)

	webhookHandler := handlers.NewWebhookHandler(billingProvider, billingOrchestrator, logger)

	contractSvc := service.NewContractService(contractRepo, planSvc, logger)
	threadSvc := service.NewThreadService(cfg,
		d.db, d.valkey, stepEventSvc,
		threadRepo, int(contractTTL.Seconds()),
		d.natsPool.GetClient(),
		natsNotification, natsArchival,
		authSvc, planSvc, cacheManager,
		d.workerPools, logger,
	)
	invitationSvc := service.NewInvitationTokenService(cfg.JWT.Secret, cfg.JWT.Issuer)

	contractHandler := handlers.NewContractHandler(contractSvc, logger)

	var notifRouter *handlers.NotificationRouter
	if d.natsPool != nil {
		notifRouter, err = handlers.NewNotificationRouter(d.natsPool.GetClient().Conn(), &cfg.NATS, logger)
		if err != nil {
			logger.Fatal("failed to create notification router", zap.Error(err))
		}
		sm.Register(notifRouter)
	}

	if err := sm.StartAll(); err != nil {
		logger.Fatal("failed to start background services", zap.Error(err))
	}

	wsHandler := handlers.NewWebSocketHandler(
		threadSvc, stepEventSvc, invitationSvc,
		threadSvc.GetNotificationConsumer(), notifRouter,
		planSvc,
		d.valkey, luaScriptManager,
		&cfg.RateLimit, &cfg.WebSocket,
		logger,
	)

	entityProfileRepo := sharedrepo.NewEntityProfileRepo(d.db.Pool)
	entityProfileTypeRepo := sharedrepo.NewEntityProfileTypeRepository(d.db.Pool)

	graphqlResolver := graphql.NewResolver(
		threadRepo, stepStateRepo, validationRepo, accessRepo,
		threadAccessSvc, threadSvc.GetContractValidator(), contractRepo,
		refsRepo, postgresStepRepo, activityRepo, actorRepo,
		notificationRepo, subStepRepo,
		entityProfileRepo, entityProfileTypeRepo,
		planSvc,
		logger,
	)

	gqlHandler := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: graphqlResolver}))
	gqlHandler.Use(extension.FixedComplexityLimit(1000))
	gqlHandler.Use(extension.Introspection{})

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(shutdownGuard(&d.shuttingDown))

	// Add CORS middleware
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization", "X-API-Key", "Accept"}
	corsConfig.ExposeHeaders = []string{"Content-Length"}
	corsConfig.MaxAge = 12 * time.Hour
	r.Use(cors.New(corsConfig))

	r.Use(requestLogger(logger))
	r.Use(middleware.IPRateLimitMiddleware(luaScriptManager, &cfg.RateLimit))
	r.Use(middleware.PrometheusMiddleware())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", healthHandler(d))
	r.POST("/webhook", webhookHandler.HandleWebhook)
	r.GET("/threads", wsHandler.HandleWebSocket)

	r.POST("/graphql",
		middleware.AuthMiddleware(authSvc, middleware.AuthDual),
		middleware.CreditUsageMiddleware(planSvc, logger),
		middleware.EgressMiddleware(planSvc, logger),
		graphqlMiddleware(gqlHandler),
	)
	r.GET("/graphql/playground", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))

	mcpGroup := r.Group("/mcp")
	mcpGroup.Use(middleware.AuthMiddleware(authSvc, middleware.AuthAPIKey))
	mcpGroup.Use(middleware.CreditUsageMiddleware(planSvc, logger))
	mountMCPServer(mcpGroup, cfg, planSvc, logger)

	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware(authSvc, middleware.AuthDual))
	v1.Use(middleware.EgressMiddleware(planSvc, logger))

	contracts := v1.Group("/contracts")
	{
		contracts.GET("", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetAllContracts)
		contracts.GET("/:id", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetContract)
		contracts.GET("/:id/versions", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetAllContractVersions)
		contracts.GET("/:id/versions/:version", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetContractVersion)
		contracts.POST("/preview", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.PreviewContract)
		contracts.DELETE("/:id", middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"), contractHandler.DeleteContract)
		contracts.DELETE("/:id/versions/:version", middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"), contractHandler.DeleteContractVersion)

		contracts.POST("",
			middleware.CreditUsageMiddleware(planSvc, logger),
			middleware.ContractRBACMiddleware(rbacLoader, "contract.create"),
			contractHandler.CreateContract,
		)
		contracts.PUT("/:id",
			middleware.CreditUsageMiddleware(planSvc, logger),
			middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"),
			contractHandler.UpdateContract,
		)
	}

	return r
}

func healthHandler(d *deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := http.StatusOK
		response := gin.H{
			"status":   "ok",
			"postgres": "ok",
			"valkey":   "ok",
		}

		if err := d.db.Pool.Ping(c.Request.Context()); err != nil {
			status = http.StatusServiceUnavailable
			response["postgres"] = "error"
		}

		if err := d.valkey.Ping(c.Request.Context()); err != nil {
			status = http.StatusServiceUnavailable
			response["valkey"] = "error"
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

		latency := time.Since(start)
		status := c.Writer.Status()

		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Int("status", status),
			zap.String("latency", latency.String()),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

func shutdownGuard(shuttingDown *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if shuttingDown.Load() {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": "server is shutting down",
			})
			return
		}
		c.Next()
	}
}

func graphqlMiddleware(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func LoadConfig() (*config.Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../../config")
	// For tests running from tests/engine
	viper.AddConfigPath("../../../config")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	viper.SetConfigName("subscription")
	if err := viper.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("merge subscription config: %w", err)
	}

	return config.LoadFromViper()
}
