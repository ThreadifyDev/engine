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
	"threadify-go/api/internal/worker"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
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

	configPath := resolveConfigPath(appLogger)
	cfg, err := config.Load(configPath)
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

	permissionsPath, rolesPath := resolveRBACPaths(appLogger)
	rbacLoader, err := rbac.NewLoader(permissionsPath, rolesPath)
	if err != nil {
		appLogger.Fatal("load rbac", zap.Error(err))
	}

	svcs, err := initServices(cfg, db, appLogger)
	if err != nil {
		appLogger.Fatal("init services", zap.Error(err))
	}
	defer svcs.close()

	hdlrs := initHandlers(cfg, db, svcs, rbacLoader, appLogger)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebAPI.Port),
		Handler:      buildRouter(cfg, db, svcs, rbacLoader, hdlrs, appLogger),
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
	natsClient      *nats.Client
	authService     *service.AuthService
	invoiceProvider billing.BillingProvider
	outboxRepo      *repository.OutboxRepository
	outboxTrigger   service.OutboxWorkerTrigger
	workerCancel    context.CancelFunc
}

func (s *services) close() {
	if s.workerCancel != nil {
		s.workerCancel()
	}
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

	encryptionKey := strings.TrimSpace(cfg.WebAPI.OutboxEncryptionKey)
	if encryptionKey == "" {
		return nil, errors.New("outbox_encryption_key is required in config.yaml (or set OUTBOX_ENCRYPTION_KEY env var)")
	}

	outboxRepo := repository.NewOutboxRepository(db)
	userRepo := repository.NewUserRepository(db)
	companyRepo := repository.NewCompanyRepository(db)

	var (
		natsClient   *nats.Client
		workerCancel context.CancelFunc
	)
	nc, natsErr := nats.NewClient(&cfg.NATS, logger)
	if natsErr != nil {
		logger.Warn("NATS unavailable — outbox worker disabled", zap.Error(natsErr))
	} else {
		if err := nc.InitializeOutboxStream(); err != nil {
			logger.Error("failed to initialize NATS outbox stream — outbox worker disabled",
				zap.Error(err))
			nc.Close()
		} else {
			natsClient = nc

			outboxWorker := worker.NewOutboxWorker(outboxRepo, userRepo, companyRepo, authClient, emailSvc, encryptionKey, logger)

			workerCtx, cancel := context.WithCancel(context.Background())
			workerCancel = cancel

			go outboxWorker.Run(workerCtx, nc.JetStream())
			go runPruner(workerCtx, outboxRepo, logger)

			logger.Info("outbox worker started (in-process)")
		}
	}

	var outboxTrigger service.OutboxWorkerTrigger
	if natsClient != nil {
		outboxTrigger = service.NewNatsOutboxTrigger(natsClient.JetStream(), nats.SubjectOutboxTrigger, logger)
	}

	teamInvitationRepo := repository.NewTeamInvitationRepository(db)
	authSvc := service.NewAuthService(db, emailSvc, authClient, outboxRepo, teamInvitationRepo, outboxTrigger, encryptionKey, logger)

	if cfg.JWKS.URL != "" {
		authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
		logger.Info("JWKS verifier configured", zap.String("url", cfg.JWKS.URL))
	} else {
		logger.Warn("JWKS URL not configured — token verification disabled")
	}

	invoiceProvider, err := billing.InitializeProvider(cfg.Billing)
	if err != nil {
		return nil, fmt.Errorf("init billing provider: %w", err)
	}

	return &services{
		natsClient:      natsClient,
		authService:     authSvc,
		invoiceProvider: invoiceProvider,
		outboxRepo:      outboxRepo,
		outboxTrigger:   outboxTrigger,
		workerCancel:    workerCancel,
	}, nil
}

type appHandlers struct {
	auth           *handlers.AuthHandler
	user           *handlers.UserHandler
	apiKey         *handlers.APIKeyHandler
	serviceAccount *handlers.ServiceAccountHandler
	role           *handlers.RoleHandler
	codeSamples    *handlers.CodeSamplesHandler
	contractProxy  *handlers.ContractProxyHandler
	graphqlProxy   *handlers.GraphQLProxyHandler
	agent          *handlers.AgentHandler
	billing        *handlers.BillingHandler
	teamInvitation *handlers.TeamInvitationHandler
}

func initHandlers(cfg *config.Config, db *sql.DB, svcs *services, rbacLoader *rbac.Loader, logger *zap.Logger) *appHandlers {
	userRepo := repository.NewUserRepository(db)
	companyRepo := repository.NewCompanyRepository(db)
	userRoleRepo := repository.NewUserRoleRepository(db)
	apiKeyRepo := repository.NewAPIKeyRepository(db)
	serviceAccountRepo := repository.NewServiceAccountRepository(db)
	agentRepo := repository.NewAgentRepository(db)
	teamInvitationRepo := repository.NewTeamInvitationRepository(db)
	planRepo := repository.NewPlanRepository(db)

	apiKeySvc := service.NewAPIKeyService(apiKeyRepo, serviceAccountRepo, userRoleRepo, rbacLoader, logger)
	serviceAccountSvc := service.NewServiceAccountService(serviceAccountRepo, userRoleRepo)
	teamInvitationSvc := service.NewTeamInvitationService(
		teamInvitationRepo,
		svcs.outboxRepo,
		svcs.outboxTrigger,
		userRepo,
		cfg.WebAPI.OutboxEncryptionKey,
		cfg.WebAPI.FrontendURL,
		logger,
	)
	billingSvc := billing.NewBillingService(svcs.invoiceProvider, planRepo, &cfg.Subscription, &cfg.Billing, logger)

	return &appHandlers{
		auth:           handlers.NewAuthHandler(svcs.authService),
		user:           handlers.NewUserHandler(userRepo, companyRepo, apiKeySvc),
		apiKey:         handlers.NewAPIKeyHandler(apiKeySvc, userRepo),
		serviceAccount: handlers.NewServiceAccountHandler(serviceAccountSvc, rbacLoader),
		role:           handlers.NewRoleHandler(rbacLoader),
		codeSamples:    handlers.NewCodeSamplesHandler("./code_samples"),
		contractProxy:  handlers.NewContractProxyHandler(cfg.WebAPI.ThreadifyEngine.URL),
		graphqlProxy:   handlers.NewGraphQLProxyHandler(cfg.WebAPI.ThreadifyEngine.GraphQLURL, logger),
		agent: handlers.NewAgentHandler(
			cfg.WebAPI.ThreadifyEngine.GraphQLURL,
			cfg.WebAPI.ThreadifyEngine.URL,
			cfg.WebAPI.OpenAIAPIKey,
			agentRepo,
			userRepo,
			cfg.WebAPI.Agent.MaxMessages,
			cfg.WebAPI.Agent.MaxTokens,
			cfg.WebAPI.Agent.SummaryMaxTokens,
			logger,
		),
		billing: handlers.NewBillingHandler(
			billingSvc,
			logger,
		),
		teamInvitation: handlers.NewTeamInvitationHandler(teamInvitationSvc, companyRepo, logger),
	}
}

func buildRouter(cfg *config.Config, db *sql.DB, svcs *services, rbacLoader *rbac.Loader, h *appHandlers, logger *zap.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(middleware.RecoveryWithLogger(logger))
	r.Use(corsMiddleware(cfg.WebAPI.CORSOrigins))
	r.Use(middleware.RequestLogger(logger))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "threadify-web-api"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Public routes
	auth := r.Group("/api/auth")
	{
		auth.POST("/signup", h.auth.Signup)
		auth.POST("/login", h.auth.Login)
		auth.POST("/forgot-password", h.auth.ForgotPassword)
		auth.POST("/reset-password", h.auth.ResetPassword)
		auth.POST("/logout", h.auth.Logout)
		auth.POST("/verify-otp", h.auth.VerifyEmail)
		auth.POST("/resend-verification", h.auth.ResendVerificationEmail)
	}

	r.GET("/api/code-samples", h.codeSamples.GetCodeSample)
	r.GET("/api/roles", h.role.GetRoles)
	r.GET("/api/roles/:level", h.role.GetRolesByLevel)

	// Public invitation validation endpoint (no auth required for signup flow)
	r.POST("/api/team/invitation/validate", h.teamInvitation.ValidateInvitation)

	userRoleRepo := repository.NewUserRoleRepository(db)
	serviceAccountRepo := repository.NewServiceAccountRepository(db)

	api := r.Group("/api")
	api.Use(middleware.AuthAccessTokenAuth(svcs.authService))

	user := api.Group("/user")
	{
		user.GET("/profile", h.user.GetProfile)
		user.POST("/profile", h.user.UpdateProfile)
		user.POST("/mark-instrumentation-done", h.user.MarkInstrumentationDone)
	}

	requirePerm := func(perm string) gin.HandlerFunc {
		return rbac.RequirePermission(rbacLoader, userRoleRepo, serviceAccountRepo, perm)
	}

	team := api.Group("/team")
	{
		team.GET("/members", requirePerm("member.view"), h.user.ListTeamMembers)
		team.POST("/invitations", requirePerm("member.invite"), h.teamInvitation.SendInvitation)
		team.GET("/invitations", requirePerm("member.view"), h.teamInvitation.ListInvitations)
		team.POST("/invitations/:id/resend", requirePerm("member.invite"), h.teamInvitation.ResendInvitation)
		team.DELETE("/invitations/:id", requirePerm("member.invite"), h.teamInvitation.CancelInvitation)
	}

	api.POST("/api-keys", requirePerm("apikey.create"), h.apiKey.CreateAPIKey)
	api.GET("/api-keys", requirePerm("apikey.read"), h.apiKey.ListAPIKeys)
	api.DELETE("/api-keys/:id", requirePerm("apikey.delete"), h.apiKey.RevokeAPIKey)

	api.POST("/service-accounts", requirePerm("serviceaccount.create"), h.serviceAccount.CreateServiceAccount)
	api.GET("/service-accounts", requirePerm("serviceaccount.read"), h.serviceAccount.ListServiceAccounts)
	api.GET("/service-accounts/:id", requirePerm("serviceaccount.read"), h.serviceAccount.GetServiceAccount)
	api.PUT("/service-accounts/:id", requirePerm("serviceaccount.update"), h.serviceAccount.UpdateServiceAccount)
	api.DELETE("/service-accounts/:id", requirePerm("serviceaccount.delete"), h.serviceAccount.DeleteServiceAccount)
	api.GET("/service-accounts/scopes/:scope/permissions", requirePerm("serviceaccount.read"), h.serviceAccount.GetPermissions)

	contracts := api.Group("/contracts")
	{
		contracts.GET("", h.contractProxy.GetAllContracts)
		contracts.POST("", h.contractProxy.CreateContract)
		contracts.POST("/preview", h.contractProxy.PreviewContract)
		contracts.GET("/:id", h.contractProxy.GetContract)
		contracts.PUT("/:id", h.contractProxy.UpdateContract)
		contracts.DELETE("/:id", h.contractProxy.DeleteContract)
		contracts.GET("/:id/versions", h.contractProxy.GetAllContractVersions)
		contracts.GET("/:id/versions/:version", h.contractProxy.GetContractVersion)
		contracts.DELETE("/:id/versions/:version", h.contractProxy.DeleteContractVersion)
	}

	api.POST("/graphql", h.graphqlProxy.ProxyGraphQL)

	api.POST("/chat/ask", h.agent.Chat)
	api.GET("/chat/conversations", h.agent.GetConversations)
	api.GET("/chat/conversations/:id", h.agent.GetConversation)
	api.POST("/chat/conversations/:id/continue", h.agent.ContinueConversation)
	api.DELETE("/chat/conversations/:id", h.agent.DeleteConversation)

	billing := api.Group("/billing")
	{
		billing.GET("/plan", h.billing.GetCurrentPlan)
		billing.GET("/tiers", h.billing.GetTiers)
		billing.POST("/checkout", h.billing.CreateCheckoutSession)
		billing.POST("/cancel", h.billing.CancelSubscription)
	}

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

func resolveConfigPath(logger *zap.Logger) string {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		logger.Info("using config path from CONFIG_PATH env", zap.String("path", p))
		return p
	}
	if _, err := os.Stat("/app/config/config.yaml"); err == nil {
		logger.Info("using config path", zap.String("path", "/app/config/config.yaml"))
		return "/app/config/config.yaml"
	}
	logger.Info("using config path (dev fallback)", zap.String("path", "../config/config.yaml"))
	return "../config/config.yaml"
}

func resolveRBACPaths(logger *zap.Logger) (string, string) {
	if _, err := os.Stat("/app/shared/rbac/permissions.json"); err == nil {
		logger.Info("using RBAC paths", zap.String("base", "/app/shared/rbac"))
		return "/app/shared/rbac/permissions.json", "/app/shared/rbac/roles.json"
	}
	if _, err := os.Stat("./shared/rbac/permissions.json"); err == nil {
		logger.Info("using RBAC paths", zap.String("base", "./shared/rbac"))
		return "./shared/rbac/permissions.json", "./shared/rbac/roles.json"
	}
	logger.Info("using RBAC paths (dev fallback)", zap.String("base", "../shared/rbac"))
	return "../shared/rbac/permissions.json", "../shared/rbac/roles.json"
}

func runPruner(ctx context.Context, repo *repository.OutboxRepository, logger *zap.Logger) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-7 * 24 * time.Hour)
			count, err := repo.PruneProcessed(cutoff)
			if err != nil {
				logger.Error("pruner: failed to prune old events", zap.Error(err))
			} else {
				logger.Info("pruner: removed events",
					zap.Int64("count", count),
					zap.String("cutoff", cutoff.Format(time.DateOnly)),
				)
			}
		}
	}
}
