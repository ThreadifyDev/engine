package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
	"threadify-go/shared/logger"
	"threadify-go/shared/rbac"
	sharedrepo "threadify-go/shared/repository"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"

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

const (
	shutdownTimeout = 30 * time.Second
	pprofAddr       = "localhost:6060"
)

type contextKey string

const (
	contextKeyOwnerID   contextKey = "ownerID"
	contextKeyCompanyID contextKey = "companyID"
	contextKeyRole      contextKey = "role"
)

func main() {
	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	deps, err := initDependencies(cfg, appLogger)
	if err != nil {
		appLogger.Fatal("failed to initialize dependencies", zap.Error(err))
	}
	defer deps.close()

	srv := buildServer(cfg, deps, appLogger)

	go startPprof(appLogger)

	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		appLogger.Info("starting server", zap.String("address", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("server error", zap.Error(err))
		}
	}()

	waitForShutdown(appLogger, srv, deps)
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

func loadConfig() (*config.Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../../config")
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

func buildServer(cfg *config.Config, d *deps, logger *zap.Logger) *http.Server {
	authRepo := postgres.NewAuthRepository(d.db.Pool)
	authSvc := service.NewAuthService(authRepo, cfg.Auth.CacheTTLSeconds)
	if strings.TrimSpace(cfg.JWKS.URL) == "" {
		logger.Fatal("jwks.url not configured — JWT authentication unavailable")
	}
	authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
	authSvc.SetWriteBackPool(d.workerPools.WriteBack)

	rbacLoader, err := rbac.NewLoader("./shared/rbac/permissions.json", "./shared/rbac/roles.json")
	if err != nil {
		logger.Fatal("failed to load RBAC roles", zap.Error(err))
	}

	// Initialize Lua script manager early (needed by threadRepo)
	luaScriptManager := valkey.NewLuaScriptManager(d.valkey)
	if err := luaScriptManager.LoadScripts(context.Background()); err != nil {
		logger.Fatal("failed to load lua scripts", zap.Error(err))
	}

	postgresThreadRepo := postgres.NewThreadRepository(d.db.Pool)
	stepStatePostgres := postgres.NewStepStateRepository(d.db.Pool)
	cacheManager := service.NewCacheService(logger)

	threadTTL := time.Duration(cfg.Cache.ThreadTTLMs) * time.Millisecond
	threadRepo := valkey.NewThreadRepository(d.valkey, int(threadTTL.Seconds()), postgresThreadRepo, stepStatePostgres, cacheManager, luaScriptManager, logger)
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
		d.natsPool.GetClient(), // ← ADD THIS LINE
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
		// Read routes — no credit check required
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

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func waitForShutdown(logger *zap.Logger, srv *http.Server, d *deps) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	d.shuttingDown.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}
	if err := d.workerPools.Shutdown(shutdownTimeout); err != nil {
		logger.Warn("worker pools shutdown with error", zap.Error(err))
	}

	d.close()
	logger.Info("shutdown complete")
}

func startPprof(logger *zap.Logger) {
	srv := &http.Server{Addr: pprofAddr}
	logger.Info("starting pprof server", zap.String("address", srv.Addr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("pprof server error", zap.Error(err))
	}
}

func graphqlMiddleware(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, sharedauth.CtxUserID, mustGet(c, sharedauth.CtxUserID))
		ctx = context.WithValue(ctx, sharedauth.CtxCompanyID, mustGet(c, sharedauth.CtxCompanyID))
		ctx = context.WithValue(ctx, sharedauth.CtxRoles, mustGet(c, sharedauth.CtxRoles))
		c.Request = c.Request.WithContext(ctx)
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func healthHandler(d *deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"postgres":  "connected",
			"redis":     "connected",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}

func requestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		if status == 403 || status == 429 {
			return
		}
		if status == 404 && c.Request.URL.Path != "/" {
			return
		}

		logger.Info("request",
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", time.Since(start)),
			zap.String("user_agent", c.Request.UserAgent()),
		)
	}
}

func mustGet(c *gin.Context, key string) any {
	v, _ := c.Get(key)
	return v
}

func shutdownGuard(flag *atomic.Bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if flag != nil && flag.Load() {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server shutting down"})
			c.Abort()
			return
		}
		c.Next()
	}
}
