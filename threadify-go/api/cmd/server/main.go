package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"threadify-go/api/internal/database"
	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/middleware"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	"threadify-go/shared/config"
	"threadify-go/shared/jwt"
	"threadify-go/shared/rbac"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Load configuration
	cfg, err := config.Load("../config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	db, err := sql.Open("postgres", cfg.Postgres.URL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("✅ Connected to PostgreSQL database")

	// Initialize database schema (idempotent)
	if err := database.InitSchema(context.Background(), db); err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}
	log.Println("✅ Database schema initialized")

	// Setup Gin router
	router := gin.Default()

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.WebAPI.CORSOrigins},
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

	// Initialize repositories
	userRepo := repository.NewUserRepository(db)
	companyRepo := repository.NewCompanyRepository(db)
	apiKeyRepo := repository.NewAPIKeyRepository(db)
	serviceAccountRepo := repository.NewServiceAccountRepository(db)

	// Load permissions and roles from JSON files
	rbacLoader, err := rbac.NewLoader("../shared/rbac/permissions.json", "../shared/rbac/roles.json")
	if err != nil {
		log.Fatalf("Failed to load permissions: %v", err)
	}
	log.Println("✅ Loaded permissions and roles from JSON")

	// Initialize JWT validator
	jwtValidator := jwt.NewValidator(
		cfg.JWT.Secret,
		cfg.JWT.Issuer,
		cfg.JWT.Audience,
	)

	// Initialize user role repository for role assignments
	userRoleRepo := repository.NewUserRoleRepository(db)

	// Initialize services
	emailService := service.NewEmailService(cfg.WebAPI.Email.PlunkAPIKey)
	authService := service.NewAuthService(db, emailService, jwtValidator, time.Duration(cfg.JWT.ExpirationHours)*time.Hour)
	serviceAccountService := service.NewServiceAccountService(serviceAccountRepo, userRoleRepo)
	apiKeyService := service.NewAPIKeyService(apiKeyRepo, serviceAccountRepo, userRoleRepo, rbacLoader)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	userHandler := handlers.NewUserHandler(userRepo, companyRepo, apiKeyService)
	apiKeyHandler := handlers.NewAPIKeyHandler(apiKeyService)
	serviceAccountHandler := handlers.NewServiceAccountHandler(serviceAccountService, rbacLoader)
	roleHandler := handlers.NewRoleHandler(rbacLoader)
	codeSamplesHandler := handlers.NewCodeSamplesHandler("./code_samples")
	// Initialize contract proxy handler with engine URL
	engineURL := "http://localhost:8081" // Engine default port
	contractProxyHandler := handlers.NewContractProxyHandler(engineURL)
	graphqlProxyHandler := handlers.NewGraphQLProxyHandler(engineURL)

	// JWT middleware
	jwtMiddleware := jwtValidator.AuthMiddleware()

	// Public routes (no JWT required)
	router.POST("/api/auth/signup", authHandler.Signup)
	router.POST("/api/auth/login", authHandler.Login)
	router.POST("/api/auth/verify-otp", authHandler.VerifyOTP)
	router.POST("/api/auth/forgot-password", authHandler.ForgotPassword)
	router.POST("/api/auth/reset-password", authHandler.ResetPassword)
	router.GET("/api/code-samples", codeSamplesHandler.GetCodeSample)
	router.GET("/api/roles", roleHandler.GetRoles)
	router.GET("/api/roles/:level", roleHandler.GetRolesByLevel)

	// Protected routes
	api := router.Group("/api")
	api.Use(jwtMiddleware)
	{

		// User routes (protected - no additional permissions needed for own profile)
		user := api.Group("/user")
		{
			user.POST("/profile", userHandler.UpdateProfile)
			user.POST("/mark-instrumentation-done", userHandler.MarkInstrumentationDone)
		}

		// API Key routes (require apikey permissions)
		api.POST("/api-keys",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "apikey.create"),
			apiKeyHandler.CreateAPIKey)
		api.GET("/api-keys",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "apikey.read"),
			apiKeyHandler.ListAPIKeys)
		api.DELETE("/api-keys/:id",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "apikey.delete"),
			apiKeyHandler.RevokeAPIKey)

		// Service Account routes (require serviceaccount permissions)
		api.POST("/service-accounts",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.create"),
			serviceAccountHandler.CreateServiceAccount)
		api.GET("/service-accounts",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.read"),
			serviceAccountHandler.ListServiceAccounts)
		api.GET("/service-accounts/:id",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.read"),
			serviceAccountHandler.GetServiceAccount)
		api.PUT("/service-accounts/:id",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.update"),
			serviceAccountHandler.UpdateServiceAccount)
		api.DELETE("/service-accounts/:id",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.delete"),
			serviceAccountHandler.DeleteServiceAccount)
		api.GET("/service-accounts/scopes/:scope/permissions",
			rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, "serviceaccount.read"),
			serviceAccountHandler.GetPermissions)

		// Contract routes (proxy to Engine - Engine handles JWT auth + RBAC)
		contracts := api.Group("/contracts")
		{
			contracts.GET("", contractProxyHandler.GetAllContracts)
			contracts.POST("", contractProxyHandler.CreateContract)
			contracts.POST("/preview", contractProxyHandler.PreviewContract)
			contracts.GET("/:id", contractProxyHandler.GetContract)
			contracts.PUT("/:id", contractProxyHandler.UpdateContract)
			contracts.DELETE("/:id", contractProxyHandler.DeleteContract)
			contracts.GET("/:id/versions", contractProxyHandler.GetAllContractVersions)
			contracts.GET("/:id/versions/:version", contractProxyHandler.GetContractVersion)
			contracts.DELETE("/:id/versions/:version", contractProxyHandler.DeleteContractVersion)
		}

		// GraphQL proxy route (proxy to Engine - Engine handles JWT auth + RBAC)
		api.POST("/graphql", graphqlProxyHandler.ProxyGraphQL)
	}

	// Add Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	log.Println("✅ Prometheus metrics endpoint enabled at /metrics")

	// Start server
	addr := fmt.Sprintf(":%d", cfg.WebAPI.Port)
	log.Printf("🚀 Web API starting on http://localhost%s\n", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
