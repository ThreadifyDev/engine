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

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"

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

type contextKey string

const (
	contextKeyOwnerID   contextKey = "ownerID"
	contextKeyCompanyID contextKey = "companyID"
	contextKeyRole      contextKey = "role"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync() //nolint:errcheck

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	deps, err := initDependencies(cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize dependencies", zap.Error(err))
	}
	defer deps.close()

	srv := buildServer(cfg, deps, logger)

	go startPprof(logger)

	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		logger.Info("starting server", zap.String("address", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	waitForShutdown(logger, srv, deps)
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
	return config.LoadFromViper()
}

func initDependencies(cfg *config.Config, logger *zap.Logger) (*deps, error) {
	d := &deps{}

	// PostgreSQL
	db, err := database.NewPostgresDB(cfg.Postgres.URL, cfg.Postgres.MaxConnections)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	// Initialize performance monitoring (must be done early, before any perf calls)
	perf.Initialize(cfg.Performance.MonitoringEnabled)
	if cfg.Performance.MonitoringEnabled {
		log.Println("✅ Performance monitoring ENABLED - time.Now() calls and [PERF] logs active")
	} else {
		log.Println("⚡ Performance monitoring DISABLED - zero overhead mode for production")
	}
	if err := db.InitSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	d.db = db

	// Valkey / Redis
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

	// NATS (optional — graceful degradation)
	natsPool, err := natsrepo.NewPool(&cfg.NATS, cfg.NATS.PoolSize)
	if err != nil {
		logger.Warn("NATS unavailable — notifications and archival disabled", zap.Error(err))
	} else {
		d.natsPool = natsPool
	}

	// Worker pools
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
	// Auth service
	authSvc := service.NewAuthService()
	if strings.TrimSpace(cfg.JWKS.URL) == "" {
		logger.Fatal("jwks.url not configured — JWT authentication unavailable")
	}
	authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
	authSvc.SetDB(d.db.Pool)
	authSvc.SetWriteBackPool(d.workerPools.WriteBack)

	rbacLoader, err := rbac.NewLoader("./shared/rbac/permissions.json", "./shared/rbac/roles.json")
	if err != nil {
		logger.Fatal("failed to load RBAC roles", zap.Error(err))
	}

	// Repositories
	postgresThreadRepo := postgres.NewThreadRepository(d.db.Pool)
	stepStatePostgres := postgres.NewStepStateRepository(d.db.Pool)
	cacheManager := service.NewCacheService()

	threadTTL := time.Duration(cfg.Cache.ThreadTTLMs) * time.Millisecond
	threadRepo := valkey.NewThreadRepository(d.valkey, int(threadTTL.Seconds()), postgresThreadRepo, stepStatePostgres, cacheManager)
	threadRepo.SetWriteBackPool(d.workerPools.WriteBack)

	contractTTL := time.Duration(cfg.Cache.ContractTTLMs) * time.Millisecond
	stepEventTTL := time.Duration(cfg.Cache.StepEventTTLMs) * time.Millisecond

	postgresStepRepo := postgres.NewStepStateRepository(d.db.Pool)
	stepStateRepo := valkey.NewStepStateRepositoryWithPostgres(d.valkey, postgresStepRepo, int(stepEventTTL.Seconds()))

	postgresValidationRepo := postgres.NewValidationRepository(d.db.Pool)
	validationRepo := valkey.NewValidationRepositoryWithPostgres(d.valkey, postgresValidationRepo)

	contractRepo := postgres.NewContractRepository(d.db.Pool)
	refsRepo := postgres.NewThreadRefsRepository(d.db.Pool)
	activityRepo := postgres.NewActivityRepository(d.db.Pool, cfg)
	actorRepo := postgres.NewActorRepository(d.db.Pool)
	notificationRepo := postgres.NewThreadNotificationRepository(d.db.Pool)
	subStepRepo := postgres.NewSubStepRepository(d.db.Pool)

	accessRepo := valkey.NewAccessRepository(d.valkey, int(threadTTL.Seconds()))
	accessRepo.SetRBACLoader(rbacLoader)
	luaScriptManager := valkey.NewLuaScriptManager(d.valkey)

	// Services
	var natsArchival *natsrepo.ArchivalPublisher
	var natsNotification *natsrepo.Publisher
	if d.natsPool != nil {
		natsArchival = natsrepo.NewArchivalPublisher(d.natsPool.GetClient())
		natsNotification = natsrepo.NewPublisher(d.natsPool.GetClient())
	}

	batchTimeout := time.Duration(cfg.ThreadActivities.BatchTimeoutMs) * time.Millisecond
	stepEventSvc := service.NewStepEventService(d.valkey, threadRepo, natsArchival, cfg, 4, cfg.ThreadActivities.BatchSize, batchTimeout)
	stepEventSvc.Start()
	d.stepEventSvc = stepEventSvc

	threadAccessSvc := service.NewThreadAccessService(accessRepo, cacheManager, luaScriptManager, rbacLoader)
	contractSvc := service.NewContractService(d.db)
	threadSvc := service.NewThreadService(cfg, d.db, d.valkey, stepEventSvc, threadRepo, int(contractTTL.Seconds()), natsNotification, natsArchival, authSvc, d.workerPools)

	invitationSvc := service.NewInvitationTokenService(cfg.JWT.Secret, cfg.JWT.Issuer)

	// Handlers
	contractHandler := handlers.NewContractHandler(contractSvc)

	var notifRouter *handlers.NotificationRouter
	if d.natsPool != nil {
		notifRouter, err = handlers.NewNotificationRouter(d.natsPool.GetClient().Conn(), &cfg.NATS)
		if err != nil {
			logger.Fatal("failed to create notification router", zap.Error(err))
		}
		d.notifRouter = notifRouter
	}

	wsHandler := handlers.NewWebSocketHandler(
		threadSvc, stepEventSvc, invitationSvc,
		threadSvc.GetNotificationConsumer(), notifRouter,
		d.valkey, luaScriptManager,
		&cfg.RateLimit, &cfg.WebSocket,
	)

	graphqlResolver := graphql.NewResolver(
		threadRepo, stepStateRepo, validationRepo, accessRepo,
		threadAccessSvc, threadSvc.GetContractValidator(), contractRepo,
		refsRepo, postgresStepRepo, activityRepo, actorRepo,
		notificationRepo, subStepRepo,
	)

	gqlHandler := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: graphqlResolver}))
	gqlHandler.Use(extension.FixedComplexityLimit(1000))
	gqlHandler.Use(extension.Introspection{})

	// Rate limiter & bot scanner
	ipRateLimiter := middleware.NewIPRateLimiter(&cfg.RateLimit)
	if cfg.RateLimit.CleanupInterval != "" {
		if interval, err := time.ParseDuration(cfg.RateLimit.CleanupInterval); err == nil {
			ipRateLimiter.Cleanup(interval)
		}
	}
	botScanner := middleware.NewBotScanner(&cfg.BotScanner)

	// Router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	r.Use(middleware.PrometheusMiddleware())
	r.Use(botScanner.Middleware())
	r.Use(ipRateLimiter.Middleware())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/health", healthHandler(d))
	r.GET("/threads", wsHandler.HandleWebSocket)

	r.POST("/graphql",
		middleware.DualAuthMiddleware(authSvc),
		middleware.UserRateLimiter(luaScriptManager, &cfg.RateLimit),
		graphqlMiddleware(gqlHandler),
	)
	r.GET("/graphql/playground", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))

	v1 := r.Group("/v1")
	v1.Use(middleware.UserRateLimiter(luaScriptManager, &cfg.RateLimit))

	contracts := v1.Group("/contracts")
	contracts.Use(middleware.ContractDualAuthMiddleware(authSvc))
	{
		contracts.GET("", middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"), contractHandler.GetAllContracts)
		contracts.POST("", middleware.ContractRBACMiddleware(rbacLoader, "contract.create"), contractHandler.CreateContract)
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}

	if err := d.workerPools.Shutdown(30 * time.Second); err != nil {
		logger.Warn("worker pools shutdown with error", zap.Error(err))
	}

	d.close()
	logger.Info("shutdown complete")
}

func startPprof(logger *zap.Logger) {
	srv := &http.Server{Addr: "localhost:6060"}
	logger.Info("starting pprof server", zap.String("address", srv.Addr))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("pprof server error", zap.Error(err))
	}
}

func graphqlMiddleware(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, contextKeyOwnerID, mustGet(c, "ownerID"))
		ctx = context.WithValue(ctx, contextKeyCompanyID, mustGet(c, "companyID"))
		ctx = context.WithValue(ctx, contextKeyRole, mustGet(c, "role"))
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

func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/metrics"},
		Formatter: func(p gin.LogFormatterParams) string {
			if p.StatusCode == 403 || p.StatusCode == 429 {
				return ""
			}
			if p.StatusCode == 404 && p.Path != "/" {
				return ""
			}
			return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n",
				p.TimeStamp.Format("2006/01/02 - 15:04:05"),
				p.StatusCode, p.Latency, p.ClientIP,
				p.Method, p.Path,
			)
		},
	})
}

func mustGet(c *gin.Context, key string) any {
	v, _ := c.Get(key)
	return v
}
