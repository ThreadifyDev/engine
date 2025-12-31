package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/middleware"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

func main() {
	// Load config
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../../config")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config: %v", err)
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
	logger.Info("Connected to PostgreSQL")

	// Connect to Redis/Valkey
	redisHost := viper.GetString("redis.host")
	redisPort := viper.GetInt("redis.port")
	redisPassword := viper.GetString("redis.password")
	redisDB := viper.GetInt("redis.db")
	valkeyService, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB)
	if err != nil {
		logger.Fatal("Failed to connect to Redis/Valkey", zap.Error(err))
	}
	defer valkeyService.Close()
	logger.Info("Connected to Redis/Valkey")

	// Initialize services
	jwtSecret := viper.GetString("jwt.secret")
	jwtIssuer := viper.GetString("jwt.issuer")
	jwtAudience := viper.GetString("jwt.audience")
	jwtExpHours := viper.GetInt("jwt.expiration_hours")
	authService := service.NewAuthService(jwtSecret, jwtIssuer, jwtAudience, jwtExpHours)

	contractService := service.NewContractService(db)

	// Initialize step event service first
	threadTTLHours := viper.GetInt("cache.thread_ttl_hours")
	threadTTL := time.Duration(threadTTLHours) * time.Hour
	threadRepo := valkey.NewThreadRepository(valkeyService, int(threadTTL.Seconds()))

	// Initialize step event service with config
	batchSize := viper.GetInt("step_events.batch_size")
	batchTimeoutMs := viper.GetInt("step_events.batch_timeout_ms")
	batchTimeout := time.Duration(batchTimeoutMs) * time.Millisecond

	stepEventService := service.NewStepEventService(valkeyService, threadRepo, 4, batchSize, batchTimeout) // 4 workers

	// Initialize thread service with step event service and TTL configs
	contractTTLHours := viper.GetInt("cache.contract_ttl_hours")
	contractTTL := time.Duration(contractTTLHours) * time.Hour
	threadService := service.NewThreadServiceWithDefaults(db, valkeyService, stepEventService, int(contractTTL.Seconds()), int(threadTTL.Seconds()))

	// Start step event service
	stepEventService.Start()
	defer stepEventService.Stop()

	// Initialize handlers
	contractHandler := handlers.NewContractHandler(contractService, authService)

	// Load audit queue configuration
	var auditConfig service.AuditQueueConfig
	if err := viper.UnmarshalKey("audit_queue", &auditConfig); err != nil {
		log.Fatalf("Failed to load audit queue config: %v", err)
	}

	// Initialize audit service
	auditService := service.NewAuditEventService(valkeyService, &auditConfig)
	log.Printf("Audit queue service initialized: %s (retention: %dh)", auditConfig.Name, auditConfig.RetentionHours)

	// Load invitation configuration
	var invitationConfig service.InvitationConfig
	if err := viper.UnmarshalKey("invitations", &invitationConfig); err != nil {
		log.Fatalf("Failed to load invitation config: %v", err)
	}

	// Initialize invitation token service
	invitationService := service.NewInvitationTokenService(jwtSecret)
	log.Printf("Invitation service initialized with %d allowed roles", len(invitationConfig.AllowedRoles))

	// Setup rate limiter with config
	rateLimitRPS := viper.GetFloat64("rate_limit.requests_per_second")
	rateLimitBurst := viper.GetInt("rate_limit.burst_size")
	rateLimitCleanupHours := viper.GetInt("rate_limit.cleanup_interval_hours")

	rateLimiter := middleware.NewRateLimiter(rateLimitRPS, rateLimitBurst)
	rateLimiter.Cleanup(time.Duration(rateLimitCleanupHours) * time.Hour)

	// Setup WebSocket handler with all services
	wsHandler := handlers.NewWebSocketHandler(threadService, stepEventService, invitationService, auditService, valkeyService)

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Apply Prometheus metrics middleware
	r.Use(middleware.PrometheusMiddleware())

	// Apply rate limiting globally
	r.Use(rateLimiter.Middleware())

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

	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware(authService))
	{
		v1.GET("/contracts", contractHandler.GetAllContracts)
		v1.POST("/contracts", contractHandler.CreateContract)
		v1.POST("/contracts/preview", contractHandler.PreviewContract)
		v1.GET("/contracts/:id", contractHandler.GetContract)
		v1.PUT("/contracts/:id", contractHandler.UpdateContract)
		v1.DELETE("/contracts/:id", contractHandler.DeleteContract)
		v1.GET("/contracts/:id/versions", contractHandler.GetAllContractVersions)
		v1.DELETE("/contracts/:id/versions/:version", contractHandler.DeleteContractVersion)
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

	// StepEventService.Stop() is handled by defer at function exit
	// No need to call it explicitly here to avoid double shutdown

	logger.Info("Server stopped")
}
