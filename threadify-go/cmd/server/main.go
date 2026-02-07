package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"threadify-go/shared/jwt"
	"threadify-go/shared/rbac"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/graphql"
	"github.com/threadify/engine/internal/graphql/generated"
	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/middleware"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

func main() {
	// Load config
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../../config")

	// Enable automatic environment variable support
	viper.AutomaticEnv()
	// Map environment variables with underscores to config keys with dots
	// e.g., DB_HOST -> postgres.host, VALKEY_HOST -> redis.host
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	// Load config into struct
	cfg, err := config.LoadFromViper()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Connect to PostgreSQL
	pgURL := viper.GetString("postgres.url")
	maxConns := viper.GetInt("postgres.max_connections")
	db, err := database.NewPostgresDB(pgURL, maxConns)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer db.Close()

	// Initialize schema
	if err := db.InitSchema(context.Background()); err != nil {
		logger.Fatal("Failed to initialize schema", zap.Error(err))
	}
	// Connected to PostgreSQL

	// Connect to Redis/Valkey
	redisHost := viper.GetString("redis.host")
	redisPort := viper.GetInt("redis.port")
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")
	redisPoolSize := viper.GetInt("redis.pool_size")
	redisMinIdleConns := viper.GetInt("redis.min_idle_conns")
	redisMaxIdleConns := viper.GetInt("redis.max_idle_conns")
	redisMaxRetries := viper.GetInt("redis.max_retries")
	redisDialTimeoutMs := viper.GetInt("redis.dial_timeout_ms")
	redisReadTimeoutMs := viper.GetInt("redis.read_timeout_ms")
	redisWriteTimeoutMs := viper.GetInt("redis.write_timeout_ms")
	redisPoolTimeoutMs := viper.GetInt("redis.pool_timeout_ms")
	redisConnMaxIdleTimeMs := viper.GetInt("redis.conn_max_idle_time_ms")

	valkeyService, err := database.NewValkeyService(
		redisHost, redisPort, redisPassword, redisDB,
		redisPoolSize, redisMinIdleConns, redisMaxIdleConns, redisMaxRetries,
		redisDialTimeoutMs, redisReadTimeoutMs, redisWriteTimeoutMs,
		redisPoolTimeoutMs, redisConnMaxIdleTimeMs,
	)
	if err != nil {
		logger.Fatal("Failed to connect to Redis/Valkey", zap.Error(err))
	}
	defer valkeyService.Close()

	// Initialize services
	jwtSecret := viper.GetString("jwt.secret")
	jwtIssuer := viper.GetString("jwt.issuer")
	jwtAudience := viper.GetString("jwt.audience")
	jwtExpHours := viper.GetInt("jwt.expiration_hours")
	authService := service.NewAuthService(jwtSecret, jwtIssuer, jwtAudience, jwtExpHours)

	// Inject database connection for API key validation
	authService.SetDB(db.Pool)

	// Initialize shared JWT validator for contract endpoints
	jwtValidator := jwt.NewValidator(jwtSecret, jwtIssuer, jwtAudience)

	// Initialize RBAC loader for contract permissions
	rbacLoader, err := rbac.NewLoader("./shared/rbac/permissions.json", "./shared/rbac/roles.json")
	if err != nil {
		logger.Fatal("Failed to load RBAC roles", zap.Error(err))
	}

	// Initialize worker pools for bounded concurrency
	workerPools := workerpool.NewPools(workerpool.NewPrometheusMetrics())
	logger.Info("Worker pools initialized",
		zap.Int32("validation_workers", workerPools.Validation.Stats().TotalWorkers),
		zap.Int32("notification_workers", workerPools.Notification.Stats().TotalWorkers),
		zap.Int32("writeback_workers", workerPools.WriteBack.Stats().TotalWorkers),
		zap.Int32("activity_workers", workerPools.Activity.Stats().TotalWorkers),
	)

	contractService := service.NewContractService(db)

	// Initialize NATS connection pool for notifications and archival (graceful degradation if unavailable)
	var natsArchivalPublisher *natsrepo.ArchivalPublisher
	var natsNotificationPublisher *natsrepo.Publisher
	natsPoolSize := viper.GetInt("nats.pool_size")
	if natsPoolSize <= 0 {
		natsPoolSize = 5 // Default to 5 connections
	}

	natsPool, err := natsrepo.NewPool(&cfg.NATS, natsPoolSize)
	if err != nil {
		logger.Warn("Failed to connect to NATS - notifications and archival will be disabled", zap.Error(err))
		natsArchivalPublisher = nil
		natsNotificationPublisher = nil
	} else {
		// Use round-robin client selection from pool
		natsArchivalPublisher = natsrepo.NewArchivalPublisher(natsPool.GetClient())
		natsNotificationPublisher = natsrepo.NewPublisher(natsPool.GetClient())
		logger.Info("NATS connection pool initialized successfully",
			zap.Int("pool_size", natsPoolSize),
			zap.Bool("healthy", natsPool.IsHealthy()))
		defer natsPool.Close()
	}

	// Initialize step event service first
	threadTTLMs := viper.GetInt("cache.thread_ttl_ms")
	threadTTL := time.Duration(threadTTLMs) * time.Millisecond

	// Create PostgreSQL repositories for fallback
	postgresThreadRepo := postgres.NewThreadRepository(db.Pool)
	stepStatePostgres := postgres.NewStepStateRepository(db.Pool)

	// Create cache manager for duplicate detection
	cacheManager := service.NewCacheService()

	threadRepo := valkey.NewThreadRepository(valkeyService, int(threadTTL.Seconds()), postgresThreadRepo, stepStatePostgres, cacheManager)

	// Initialize step event service with config
	batchSize := viper.GetInt("thread_activities.batch_size")
	batchTimeoutMs := viper.GetInt("thread_activities.batch_timeout_ms")
	batchTimeout := time.Duration(batchTimeoutMs) * time.Millisecond

	stepEventService := service.NewStepEventService(valkeyService, threadRepo, natsArchivalPublisher, cfg, 4, batchSize, batchTimeout)

	// Initialize thread service with step event service and TTL configs
	contractTTLMs := viper.GetInt("cache.contract_ttl_ms")
	contractTTL := time.Duration(contractTTLMs) * time.Millisecond
	threadService := service.NewThreadService(cfg, db, valkeyService, stepEventService, threadRepo, int(contractTTL.Seconds()), natsNotificationPublisher, natsArchivalPublisher, authService, workerPools)

	// Start step event service
	stepEventService.Start()
	defer stepEventService.Stop()

	// Initialize handlers
	contractHandler := handlers.NewContractHandler(contractService, authService)

	// Load invitation configuration
	var invitationConfig service.InvitationConfig
	if err := viper.UnmarshalKey("invitations", &invitationConfig); err != nil {
		log.Fatalf("Failed to load invitation config: %v", err)
	}

	invitationService := service.NewInvitationTokenService(jwtSecret, jwtIssuer)
	// Invitation service initialized

	// Setup rate limiters with config
	var rateLimitCfg config.RateLimitConfig
	if err := viper.UnmarshalKey("rate_limit", &rateLimitCfg); err != nil {
		log.Fatalf("Failed to load rate limit config: %v", err)
	}

	// Create IP rate limiter (in-memory, per-pod)
	ipRateLimiter := middleware.NewIPRateLimiter(&rateLimitCfg)
	if rateLimitCfg.CleanupInterval != "" {
		cleanupInterval, err := time.ParseDuration(rateLimitCfg.CleanupInterval)
		if err == nil {
			ipRateLimiter.Cleanup(cleanupInterval)
		}
	}
	// IP rate limiter initialized

	// Create bot scanner
	var botScannerCfg config.BotScannerConfig
	if err := viper.UnmarshalKey("bot_scanner", &botScannerCfg); err != nil {
		log.Fatalf("Failed to load bot scanner config: %v", err)
	}
	botScanner := middleware.NewBotScanner(&botScannerCfg)
	// Bot scanner initialized

	// Initialize notification router with NATS
	var notificationRouter *handlers.NotificationRouter
	if natsPool != nil {
		notificationRouter, err = handlers.NewNotificationRouter(natsPool.GetClient().Conn(), &cfg.NATS)
		if err != nil {
			log.Fatalf("Failed to create notification router: %v", err)
		}
		defer notificationRouter.Stop()
		// Notification router initialized
	} else {
		// Notification router disabled (NATS not available)
	}

	// Initialize GraphQL handler with cached thread repository and step state repository with PostgreSQL fallback
	stepEventTTLMs := viper.GetInt("cache.step_event_ttl_ms")
	stepEventTTL := time.Duration(stepEventTTLMs) * time.Millisecond
	postgresStepRepo := postgres.NewStepStateRepository(db.Pool)
	stepStateRepo := valkey.NewStepStateRepositoryWithPostgres(valkeyService, postgresStepRepo, int(stepEventTTL.Seconds()))

	// Initialize validation repository with cache-aside pattern
	postgresValidationRepo := postgres.NewValidationRepository(db.Pool)
	validationRepo := valkey.NewValidationRepositoryWithPostgres(valkeyService, postgresValidationRepo)

	// Initialize contract repository for GraphQL access control
	contractRepo := postgres.NewContractRepository(db.Pool)

	// Initialize thread access service for invitation-based authentication
	// Use thread TTL for access keys (same as thread metadata)
	accessRepo := valkey.NewAccessRepository(valkeyService, int(threadTTL.Seconds()))
	accessRepo.SetRBACLoader(rbacLoader) // Enable dynamic permission-to-role mapping
	luaScriptManager := valkey.NewLuaScriptManager(valkeyService)

	// Initialize access batcher for bulk operations
	accessBatchSize := viper.GetInt("thread_access.batch_size")
	accessBatchTimeoutMs := viper.GetInt("thread_access.batch_timeout_ms")
	accessBufferSize := viper.GetInt("thread_access.buffer_size")
	if accessBatchSize == 0 {
		accessBatchSize = 25
	}
	if accessBatchTimeoutMs == 0 {
		accessBatchTimeoutMs = 50
	}
	if accessBufferSize == 0 {
		accessBufferSize = 100
	}

	threadAccessService := service.NewThreadAccessService(accessRepo, cacheManager, luaScriptManager, rbacLoader)

	// Create WebSocket handler with notification consumer, router, and rate limiting
	wsHandler := handlers.NewWebSocketHandler(threadService, stepEventService, invitationService, threadService.GetNotificationConsumer(), notificationRouter, valkeyService, luaScriptManager, &rateLimitCfg, &cfg.WebSocket)

	// Initialize refs repository for batch loading
	refsRepo := postgres.NewThreadRefsRepository(db.Pool)

	// Initialize activity repository for hash chain verification
	activityRepo := postgres.NewActivityRepository(db.Pool, cfg)
	actorRepo := postgres.NewActorRepository(db.Pool)
	notificationRepo := postgres.NewThreadNotificationRepository(db.Pool)
	subStepRepo := postgres.NewSubStepRepository(db.Pool)

	// Initialize GraphQL resolver with batch loading repos and access repo for permission checks
	graphqlResolver := graphql.NewResolver(threadRepo, stepStateRepo, validationRepo, accessRepo, threadAccessService, threadService.GetContractValidator(), contractRepo, refsRepo, postgresStepRepo, activityRepo, actorRepo, notificationRepo, subStepRepo)
	// GraphQL resolver created

	graphqlHandler := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: graphqlResolver}))

	// Add query complexity limit to prevent expensive nested queries
	// Complexity budget: 1000 points
	// This prevents queries like: threads(100) { steps { history(1000) } } which would be ~500k complexity
	complexityLimit := extension.FixedComplexityLimit(1000)
	graphqlHandler.Use(complexityLimit)

	// Enable introspection for development (includes schema documentation)
	graphqlHandler.Use(extension.Introspection{})

	// GraphQL handler created with complexity limit: 1000

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Add recovery middleware
	r.Use(gin.Recovery())

	// Add custom logger that skips bot/rate limit responses
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/metrics"},
		Formatter: func(param gin.LogFormatterParams) string {
			// Skip logging for bot blocks (403), rate limits (429), and scanner 404s
			if param.StatusCode == 403 || param.StatusCode == 429 {
				return ""
			}
			// Only log non-bot 404s (legitimate missing endpoints)
			if param.StatusCode == 404 && param.Path != "/" {
				return ""
			}
			return fmt.Sprintf("[GIN] %v | %3d | %13v | %15s | %-7s %#v\n",
				param.TimeStamp.Format("2006/01/02 - 15:04:05"),
				param.StatusCode,
				param.Latency,
				param.ClientIP,
				param.Method,
				param.Path,
			)
		},
	}))

	// Apply Prometheus metrics middleware
	r.Use(middleware.PrometheusMiddleware())

	// Apply bot scanner first (before rate limiting)
	r.Use(botScanner.Middleware())

	// Apply IP rate limiting globally
	r.Use(ipRateLimiter.Middleware())

	// Prometheus metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":    "healthy",
			"postgres":  "connected",
			"redis":     "connected",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Public routes
	r.POST("/v1/contracts/login", contractHandler.Login)

	// WebSocket route (no auth at connection level)
	r.GET("/threads", wsHandler.HandleWebSocket)

	// GraphQL endpoints with dual auth (API Key or JWT)
	r.POST("/graphql",
		middleware.DualAuthMiddleware(authService, jwtValidator),
		middleware.UserRateLimiter(luaScriptManager, &rateLimitCfg),
		func(c *gin.Context) {
			// Extract user info from Gin context
			ownerID, _ := c.Get("ownerID")
			companyID, _ := c.Get("companyID")
			role, _ := c.Get("role")

			// Create new context with user info for GraphQL resolvers
			ctx := context.WithValue(c.Request.Context(), "ownerID", ownerID)
			ctx = context.WithValue(ctx, "companyID", companyID)
			ctx = context.WithValue(ctx, "role", role)

			// Update request with enriched context
			c.Request = c.Request.WithContext(ctx)

			// Serve GraphQL with enriched context
			graphqlHandler.ServeHTTP(c.Writer, c.Request)
		})
	r.GET("/graphql/playground", gin.WrapH(playground.Handler("GraphQL Playground", "/graphql")))

	v1 := r.Group("/v1")
	v1.Use(middleware.UserRateLimiter(luaScriptManager, &rateLimitCfg))

	// Contract endpoints with JWT authentication and RBAC
	contracts := v1.Group("/contracts")
	contracts.Use(middleware.ContractJWTMiddleware(jwtValidator))
	{
		contracts.GET("",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
			contractHandler.GetAllContracts)

		contracts.POST("",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.create"),
			contractHandler.CreateContract)

		contracts.POST("/preview",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
			contractHandler.PreviewContract)

		contracts.GET("/:id",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
			contractHandler.GetContract)

		contracts.PUT("/:id",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"),
			contractHandler.UpdateContract)

		contracts.DELETE("/:id",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"),
			contractHandler.DeleteContract)

		contracts.GET("/:id/versions",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
			contractHandler.GetAllContractVersions)

		contracts.GET("/:id/versions/:version",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.read.*"),
			contractHandler.GetContractVersion)

		contracts.DELETE("/:id/versions/:version",
			middleware.ContractRBACMiddleware(rbacLoader, "contract.update.*"),
			contractHandler.DeleteContractVersion)
	}

	// Start server
	port := viper.GetInt("server.port")
	host := viper.GetString("server.host")
	addr := fmt.Sprintf("%s:%d", host, port)

	logger.Info("Starting server", zap.String("address", addr))

	// Start pprof server on separate port
	go func() {
		pprofAddr := fmt.Sprintf("%s:%d", "localhost", 6060)
		logger.Info("Starting pprof server", zap.String("address", pprofAddr))
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			logger.Error("Failed to start pprof server", zap.Error(err))
		}
	}()

	// Start server in a goroutine
	go func() {
		if err := r.Run(addr); err != nil {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// Gracefully shutdown worker pools (wait up to 30s for in-flight jobs)
	logger.Info("Shutting down worker pools...")
	if err := workerPools.Shutdown(30 * time.Second); err != nil {
		logger.Warn("Worker pools shutdown with error", zap.Error(err))
	} else {
		logger.Info("Worker pools shutdown complete")
	}

	// StepEventService.Stop() is handled by defer at function exit
	// No need to call it explicitly here to avoid double shutdown

	logger.Info("Server stopped")
}
