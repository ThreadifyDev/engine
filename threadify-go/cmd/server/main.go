package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/handlers"
	"github.com/threadify/engine/internal/middleware"
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
	valkey, err := database.NewValkeyService(redisHost, redisPort, redisPassword, redisDB)
	if err != nil {
		logger.Fatal("Failed to connect to Redis/Valkey", zap.Error(err))
	}
	defer valkey.Close()
	logger.Info("Connected to Redis/Valkey")

	// Initialize services
	jwtSecret := viper.GetString("jwt.secret")
	jwtIssuer := viper.GetString("jwt.issuer")
	jwtAudience := viper.GetString("jwt.audience")
	jwtExpHours := viper.GetInt("jwt.expiration_hours")
	authService := service.NewAuthService(jwtSecret, jwtIssuer, jwtAudience, jwtExpHours)

	contractService := service.NewContractService(db)

	threadService := service.NewThreadService(db, valkey)

	// Initialize handlers
	contractHandler := handlers.NewContractHandler(contractService, authService)
	wsHandler := handlers.NewWebSocketHandler(threadService)

	// Setup rate limiter
	rateLimiter := middleware.NewRateLimiter(100, 200) // 100 req/s, burst 200
	rateLimiter.Cleanup(time.Hour)                     // Cleanup every hour

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

	// Protected routes
	v1 := r.Group("/v1")
	v1.Use(middleware.AuthMiddleware(authService))
	{
		v1.GET("/contracts", contractHandler.GetAllContracts)
		v1.POST("/contracts", contractHandler.CreateContract)
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
	if err := r.Run(addr); err != nil {
		logger.Fatal("Failed to start server", zap.Error(err))
	}
}
