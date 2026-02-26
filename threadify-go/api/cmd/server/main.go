package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"threadify-go/api/internal/database"
	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/middleware"
	"threadify-go/api/internal/repository"
	"threadify-go/api/internal/service"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/config"
	"threadify-go/shared/logger"
	"threadify-go/shared/nats"
	"threadify-go/shared/rbac"
)

func main() {
	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	cfg, err := config.Load(resolveConfigPath())
	if err != nil {
		appLogger.Fatal("load config", zap.Error(err))
	}

	db, err := initDB(cfg.Postgres.URL)
	if err != nil {
		appLogger.Fatal("init database", zap.Error(err))
	}
	defer db.Close()

	if err := database.InitSchema(context.Background(), db); err != nil {
		appLogger.Fatal("init schema", zap.Error(err))
	}

	rbacLoader, err := rbac.NewLoader(resolveRBACPaths())
	if err != nil {
		appLogger.Fatal("load rbac", zap.Error(err))
	}

	svcs, err := initServices(cfg, db, appLogger)
	if err != nil {
		appLogger.Fatal("init services", zap.Error(err))
	}
	defer svcs.close()

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebAPI.Port),
		Handler:      buildRouter(cfg, db, svcs, rbacLoader, appLogger),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		appLogger.Info("web API starting", zap.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			appLogger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	appLogger.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		appLogger.Error("server forced shutdown", zap.Error(err))
	}
	appLogger.Info("shutdown complete")
}

type services struct {
	natsClient  *nats.Client
	authService *service.AuthService
}

func (s *services) close() {
	if s.natsClient != nil {
		s.natsClient.Close()
	}
}

func initServices(cfg *config.Config, db *sql.DB, logger *zap.Logger) (*services, error) {
	emailSvc, err := service.NewEmailService(
		cfg.WebAPI.Email.PlunkAPIKey,
		cfg.WebAPI.Email.PlunkAPIURL,
		cfg.WebAPI.FrontendURL,
	)
	if err != nil {
		return nil, fmt.Errorf("init email service: %w", err)
	}

	authClient, err := sharedauth.NewAuthClientFromSharedConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("init auth client: %w", err)
	}

	encryptionKey := strings.TrimSpace(os.Getenv("OUTBOX_ENCRYPTION_KEY"))
	outboxRepo := repository.NewOutboxRepository(db)

	var outboxTrigger service.OutboxWorkerTrigger
	natsClient, err := nats.NewClient(&cfg.NATS, logger)
	if err != nil {
		logger.Warn("NATS unavailable — outbox triggers disabled", zap.Error(err))
	} else {
		outboxTrigger = service.NewNatsOutboxTrigger(natsClient.JetStream(), nats.SubjectOutboxTrigger, logger)
	}

	authSvc := service.NewAuthService(db, emailSvc, authClient, outboxRepo, outboxTrigger, encryptionKey, logger)

	if cfg.JWKS.URL != "" {
		authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
	}

	return &services{natsClient: natsClient, authService: authSvc}, nil
}

func buildRouter(cfg *config.Config, db *sql.DB, svcs *services, rbacLoader *rbac.Loader, logger *zap.Logger) http.Handler {
	userRepo := repository.NewUserRepository(db)
	companyRepo := repository.NewCompanyRepository(db)
	userRoleRepo := repository.NewUserRoleRepository(db)
	apiKeyRepo := repository.NewAPIKeyRepository(db)
	serviceAccountRepo := repository.NewServiceAccountRepository(db)

	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, serviceAccountRepo, userRoleRepo, rbacLoader)
	serviceAccountSvc := service.NewServiceAccountService(serviceAccountRepo, userRoleRepo)

	authHandler := handlers.NewAuthHandler(svcs.authService)
	userHandler := handlers.NewUserHandler(userRepo, companyRepo, apiKeySvc)
	apiKeyHandler := handlers.NewAPIKeyHandler(apiKeySvc)
	serviceAccountHandler := handlers.NewServiceAccountHandler(serviceAccountSvc, rbacLoader)
	roleHandler := handlers.NewRoleHandler(rbacLoader)
	codeSamplesHandler := handlers.NewCodeSamplesHandler("./code_samples")
	contractProxyHandler := handlers.NewContractProxyHandler(cfg.WebAPI.ThreadifyEngine.URL)
	graphqlProxyHandler := handlers.NewGraphQLProxyHandler(cfg.WebAPI.ThreadifyEngine.GraphQLURL, logger)

	// Agent AI Chat
	agentRepo := repository.NewAgentRepository(db)
	agentHandler := handlers.NewAgentHandler(
		cfg.WebAPI.ThreadifyEngine.GraphQLURL,
		cfg.WebAPI.OpenAIAPIKey,
		agentRepo,
		cfg.WebAPI.Agent.MaxMessages,
		cfg.WebAPI.Agent.MaxTokens,
		cfg.WebAPI.Agent.SummaryMaxTokens,
		logger,
	)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(cfg.WebAPI.CORSOrigins))
	r.Use(middleware.RequestLogger(logger))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "threadify-web-api"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Public routes
	auth := r.Group("/api/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
		auth.POST("/forgot-password", authHandler.ForgotPassword)
		auth.POST("/reset-password", authHandler.ResetPassword)
		auth.POST("/verify-email", authHandler.VerifyEmail)
	}
	r.GET("/api/code-samples", codeSamplesHandler.GetCodeSample)
	r.GET("/api/roles", roleHandler.GetRoles)
	r.GET("/api/roles/:level", roleHandler.GetRolesByLevel)

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.AuthAccessTokenAuth(svcs.authService))

	user := api.Group("/user")
	{
		user.POST("/profile", userHandler.UpdateProfile)
		user.POST("/mark-instrumentation-done", userHandler.MarkInstrumentationDone)
	}

	requirePerm := func(perm string) gin.HandlerFunc {
		return rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, perm)
	}

	api.POST("/api-keys", requirePerm("apikey.create"), apiKeyHandler.CreateAPIKey)
	api.GET("/api-keys", requirePerm("apikey.read"), apiKeyHandler.ListAPIKeys)
	api.DELETE("/api-keys/:id", requirePerm("apikey.delete"), apiKeyHandler.RevokeAPIKey)

	api.POST("/service-accounts", requirePerm("serviceaccount.create"), serviceAccountHandler.CreateServiceAccount)
	api.GET("/service-accounts", requirePerm("serviceaccount.read"), serviceAccountHandler.ListServiceAccounts)
	api.GET("/service-accounts/:id", requirePerm("serviceaccount.read"), serviceAccountHandler.GetServiceAccount)
	api.PUT("/service-accounts/:id", requirePerm("serviceaccount.update"), serviceAccountHandler.UpdateServiceAccount)
	api.DELETE("/service-accounts/:id", requirePerm("serviceaccount.delete"), serviceAccountHandler.DeleteServiceAccount)
	api.GET("/service-accounts/scopes/:scope/permissions", requirePerm("serviceaccount.read"), serviceAccountHandler.GetPermissions)

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

	api.POST("/graphql", graphqlProxyHandler.ProxyGraphQL)

	// Agent AI Chat routes
	api.POST("/chat/ask", agentHandler.Chat)
	api.GET("/chat/conversations", agentHandler.GetConversations)
	api.GET("/chat/conversations/:id", agentHandler.GetConversation)
	api.POST("/chat/conversations/:id/continue", agentHandler.ContinueConversation)
	api.DELETE("/chat/conversations/:id", agentHandler.DeleteConversation)

	return r
}

func corsMiddleware(originsCSV string) gin.HandlerFunc {
	origins := strings.Split(originsCSV, ",")
	for i, o := range origins {
		origins[i] = strings.TrimSpace(o)
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	})
}

func initDB(url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

func resolveConfigPath() string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		return p
	}
	if _, err := os.Stat("/app/config/config.yaml"); err == nil {
		return "/app/config/config.yaml"
	}
	return "../config/config.yaml"
}

func resolveRBACPaths() (string, string) {
	if _, err := os.Stat("/app/shared/rbac/permissions.json"); err == nil {
		return "/app/shared/rbac/permissions.json", "/app/shared/rbac/roles.json"
	}
	if _, err := os.Stat("./shared/rbac/permissions.json"); err == nil {
		return "./shared/rbac/permissions.json", "./shared/rbac/roles.json"
	}
	return "../shared/rbac/permissions.json", "../shared/rbac/roles.json"
}
