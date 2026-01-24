package main

import (
	"database/sql"
	"fmt"
	"log"
	"threadify-web-api/config"
	"threadify-web-api/internal/middleware"
	"threadify-web-api/internal/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Connect to database
	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("✅ Connected to PostgreSQL database")

	// Run database migrations
	if err := utils.RunMigrations(db, "./migrations"); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Setup Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.CORSOrigins},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Request logging middleware
	router.Use(middleware.RequestLogger())

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "threadify-web-api",
		})
	})

	// API routes (to be implemented in Phase 1)
	api := router.Group("/api")
	{
		// Auth routes (Phase 1)
		auth := api.Group("/auth")
		{
			auth.POST("/signup", func(c *gin.Context) {
				c.JSON(501, gin.H{"error": "Not implemented yet"})
			})
			auth.POST("/login", func(c *gin.Context) {
				c.JSON(501, gin.H{"error": "Not implemented yet"})
			})
			auth.POST("/verify-otp", func(c *gin.Context) {
				c.JSON(501, gin.H{"error": "Not implemented yet"})
			})
		}
	}

	// Start server
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("🚀 Server starting on http://localhost%s\n", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
