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
	"syscall"
	"time"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/logger"
	"threadify-go/shared/rbac"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
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
	db           *database.PostgresDB
	valkey       *database.ValkeyService
	natsPool     *natsrepo.Pool
	workerPools  *workerpool.Pools
	stepEventSvc *service.StepEventService
	notifRouter  *handlers.NotificationRouter
}

func (d *deps) close() {
	if d.stepEventSvc != nil {
		d.stepEventSvc.Stop()
	}
	if d.notifRouter != nil {
		d.notifRouter.Stop()
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
		logger.Warn("NATS unavailable — notifications and archival disabled", zap.Error(err))
	} else {
		d.natsPool = natsPool
	}

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

	var natsArchival *natsrepo.ArchivalPublisher
	var natsNotification *natsrepo.Publisher
	if d.natsPool != nil {
		natsArchival = natsrepo.NewArchivalPublisher(d.natsPool.GetClient(), logger)
		natsNotification = natsrepo.NewPublisher(d.natsPool.GetClient())
	}

	stepEventSvc, err := service.NewStepEventService(d.valkey, threadRepo, natsArchival, cfg, logger)
	if err != nil {
		logger.Fatal("failed to create step event service", zap.Error(err))
	}

	stepEventSvc.Start()
	d.stepEventSvc = stepEventSvc

	threadAccessSvc := service.NewThreadAccessService(accessRepo, cacheManager, luaScriptManager, rbacLoader, logger)

	planRepo := postgres.NewPlanRepository(d.db.Pool)
	planSvc := service.NewPlanService(planRepo, &cfg.Subscription, d.valkey, logger)
	if natsArchival != nil {
		planSvc.SetNATSPublisher(natsArchival)
	}

	contractSvc := service.NewContractService(d.db, logger)
	threadSvc := service.NewThreadService(cfg, d.db, d.valkey, stepEventSvc, threadRepo, int(contractTTL.Seconds()), natsNotification, natsArchival, authSvc, d.workerPools, logger)
	invitationSvc := service.NewInvitationTokenService(cfg.JWT.Secret, cfg.JWT.Issuer)

	contractHandler := handlers.NewContractHandler(contractSvc, logger)

	var notifRouter *handlers.NotificationRouter
	if d.natsPool != nil {
		notifRouter, err = handlers.NewNotificationRouter(d.natsPool.GetClient().Conn(), &cfg.NATS, logger)
		if err != nil {
			logger.Fatal("failed to create notification router", zap.Error(err))
		}
		d.notifRouter = notifRouter
	}

	wsHandler := handlers.NewWebSocketHandler(
		threadSvc, stepEventSvc, invitationSvc,
		threadSvc.GetNotificationConsumer(), notifRouter,
		planSvc,
		d.valkey, luaScriptManager,
		&cfg.RateLimit, &cfg.WebSocket,
		logger,
	)

	graphqlResolver := graphql.NewResolver(
		threadRepo, stepStateRepo, validationRepo, accessRepo,
		threadAccessSvc, threadSvc.GetContractValidator(), contractRepo,
		refsRepo, postgresStepRepo, activityRepo, actorRepo,
		notificationRepo, subStepRepo,
		logger,
	)

	gqlHandler := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: graphqlResolver}))
	gqlHandler.Use(extension.FixedComplexityLimit(1000))
	gqlHandler.Use(extension.Introspection{})

	ipRateLimiter := middleware.NewIPRateLimiter(&cfg.RateLimit)
	if cfg.RateLimit.CleanupInterval != "" {
		if interval, err := time.ParseDuration(cfg.RateLimit.CleanupInterval); err == nil {
			ipRateLimiter.Cleanup(interval)
		}
	}
	botScanner := middleware.NewBotScanner(&cfg.BotScanner)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger(logger))
	r.Use(middleware.PrometheusMiddleware())
	r.Use(botScanner.Middleware())
	r.Use(ipRateLimiter.Middleware())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", healthHandler(d))
	r.GET("/threads", wsHandler.HandleWebSocket)

	r.POST("/graphql",
		middleware.AuthMiddleware(authSvc, middleware.AuthDual),
		middleware.CompanyRateLimiter(luaScriptManager, &cfg.RateLimit),
		middleware.SubscriptionMiddleware(planSvc, luaScriptManager),
		graphqlMiddleware(gqlHandler),
	)
	r.GET("/graphql/playground", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))

	mcpGroup := r.Group("/mcp")
	mcpGroup.Use(middleware.AuthMiddleware(authSvc, middleware.AuthAPIKey))
	mcpGroup.Use(middleware.CompanyRateLimiter(luaScriptManager, &cfg.RateLimit))
	mountMCPServer(mcpGroup, cfg, logger)

	v1 := r.Group("/v1")
	v1.Use(middleware.CompanyRateLimiter(luaScriptManager, &cfg.RateLimit))
	v1.Use(middleware.SubscriptionMiddleware(planSvc, luaScriptManager))

	contracts := v1.Group("/contracts")
	contracts.Use(middleware.AuthMiddleware(authSvc, middleware.AuthDual))
	{
		contracts.GET("", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetAllContracts)
		contracts.POST("",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.create"),
			middleware.ContractQuotaMiddleware(planSvc, contractSvc),
			middleware.PayloadSizeMiddleware(),
			contractHandler.CreateContract,
		)
		contracts.POST("/preview", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.PreviewContract)
		contracts.GET("/:id", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetContract)
		contracts.PUT("/:id", middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"), contractHandler.UpdateContract)
		contracts.DELETE("/:id", middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"), contractHandler.DeleteContract)
		contracts.GET("/:id/versions", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetAllContractVersions)
		contracts.GET("/:id/versions/:version", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetContractVersion)
		contracts.DELETE("/:id/versions/:version", middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"), contractHandler.DeleteContractVersion)
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
