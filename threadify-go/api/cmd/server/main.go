package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
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
	sharedrepo "threadify-go/shared/repository"
)

func main() {
	appLogger, err := logger.NewLogger(os.Getenv("GO_ENV") == "production")
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer appLogger.Sync() //nolint:errcheck

	rootCtx, rootCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer rootCancel()

	cfg, err := loadConfig(appLogger)
	if err != nil {
		appLogger.Fatal("load config", zap.Error(err))
	}

	dbCtx, dbCancel := context.WithTimeout(rootCtx, 15*time.Second)
	defer dbCancel()

	pool, err := initDB(dbCtx, cfg.Postgres.URL)
	if err != nil {
		appLogger.Fatal("init database", zap.Error(err))
	}
	defer pool.Close()

	if err := database.InitSchema(rootCtx, pool); err != nil {
		appLogger.Fatal("init schema", zap.Error(err))
	}

	permissionsPath, rolesPath, err := resolveRBACPaths(appLogger)
	if err != nil {
		appLogger.Fatal("resolve rbac paths", zap.Error(err))
	}
	rbacLoader, err := rbac.NewLoader(permissionsPath, rolesPath)
	if err != nil {
		appLogger.Fatal("load rbac", zap.Error(err))
	}

	repos := initRepositories(pool)

	svcs, err := initServices(cfg, pool, repos, appLogger)
	if err != nil {
		appLogger.Fatal("init services", zap.Error(err))
	}
	defer svcs.close(appLogger)

	hdlrs := initHandlers(cfg, svcs, repos, rbacLoader, appLogger)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.WebAPI.Port),
		Handler:      buildRouter(cfg, svcs, repos, rbacLoader, hdlrs, appLogger),
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

	<-rootCtx.Done()

	appLogger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("server forced shutdown", zap.Error(err))
	}
	appLogger.Info("shutdown complete")
}

type repositories struct {
	user              *repository.UserRepository
	company           *repository.CompanyRepository
	userRole          *repository.UserRoleRepository
	apiKey            *repository.APIKeyRepository
	serviceAccount    *repository.ServiceAccountRepository
	outbox            *repository.OutboxRepository
	agent             *repository.AgentRepository
	plan              sharedrepo.PlanRepository
	entityProfileType sharedrepo.EntityProfileTypeRepository
}

func initRepositories(pool *pgxpool.Pool) *repositories {
	userRepo := repository.NewUserRepository(pool)
	companyRepo := repository.NewCompanyRepository(pool)
	userRoleRepo := repository.NewUserRoleRepository(pool)
	apiKeyRepo := repository.NewAPIKeyRepository(pool)
	serviceAccountRepo := repository.NewServiceAccountRepository(pool)
	planRepo := sharedrepo.NewPlanRepo(pool)
	outboxRepo := repository.NewOutboxRepository(pool)
	agentRepo := repository.NewAgentRepository(pool)
	entityProfileTypeRepo := sharedrepo.NewEntityProfileTypeRepository(pool)

	return &repositories{
		user:              userRepo,
		company:           companyRepo,
		userRole:          userRoleRepo,
		apiKey:            apiKeyRepo,
		serviceAccount:    serviceAccountRepo,
		plan:              planRepo,
		outbox:            outboxRepo,
		agent:             agentRepo,
		entityProfileType: entityProfileTypeRepo,
	}
}

type services struct {
	natsClient            *nats.Client
	authService           *service.AuthService
	agentService          *service.AgentService
	billingService        *billing.BillingService
	teamInvitationService *service.TeamInvitationService
	invoiceProvider       billing.BillingProvider
	outboxRepo            *repository.OutboxRepository
	outboxTrigger         service.OutboxWorkerTrigger
	workerCancel          context.CancelFunc
	workerWg              sync.WaitGroup
}

func (s *services) close(logger *zap.Logger) {
	if s.workerCancel != nil {
		logger.Info("stopping background workers...")
		s.workerCancel()
		s.workerWg.Wait()
		logger.Info("background workers stopped")
	}
	if s.natsClient != nil {
		s.natsClient.Close()
		logger.Info("NATS connection closed")
	}
}

func initServices(cfg *config.Config, pool *pgxpool.Pool, repos *repositories, logger *zap.Logger) (*services, error) {
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

	invoiceProvider, err := billing.InitializeProvider(cfg.Billing)
	if err != nil {
		return nil, fmt.Errorf("init billing provider: %w", err)
	}

	billingSvc := billing.NewBillingService(invoiceProvider, repos.plan, &cfg.Subscription, &cfg.Billing, logger)

	nc, err := nats.NewClient(&cfg.NATS, logger)
	if err != nil {
		return nil, fmt.Errorf("NATS unavailable: %w", err)
	}

	if err := nc.InitializeOutboxStream(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("initialize NATS outbox stream: %w", err)
	}

	svcs := &services{
		natsClient:      nc,
		billingService:  billingSvc,
		invoiceProvider: invoiceProvider,
		outboxRepo:      repos.outbox,
	}

	outboxWorker := worker.NewOutboxWorker(
		pool,
		repos.outbox,
		repos.user,
		repos.company,
		authClient,
		emailSvc,
		encryptionKey,
		logger,
	)

	workerCtx, cancel := context.WithCancel(context.Background())
	svcs.workerCancel = cancel

	svcs.workerWg.Add(2)
	go func() {
		defer svcs.workerWg.Done()
		outboxWorker.Run(workerCtx, nc.JetStream())
	}()
	go func() {
		defer svcs.workerWg.Done()
		runPruner(workerCtx, repos.outbox, logger)
	}()

	logger.Info("outbox worker started (in-process)")

	outboxTrigger := service.NewNatsOutboxTrigger(nc.JetStream(), nats.SubjectOutboxTrigger, logger)
	svcs.outboxTrigger = outboxTrigger

	teamInvitationRepo := repository.NewTeamInvitationRepository(pool)
	authSvc := service.NewAuthService(
		pool,
		emailSvc,
		authClient,
		repos.outbox,
		teamInvitationRepo,
		outboxTrigger,
		encryptionKey,
		logger,
	)

	if cfg.JWKS.URL != "" {
		authSvc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(cfg.JWKS.URL, cfg.JWKS.Audience, cfg.JWKS.Issuer))
		logger.Info("JWKS verifier configured", zap.String("url", cfg.JWKS.URL))
	} else {
		logger.Warn("JWKS URL not configured — token verification disabled")
	}

	agentSvc := service.NewAgentService(
		cfg.WebAPI.ThreadifyEngine.GraphQLURL,
		cfg.WebAPI.OpenAIAPIKey,
		repos.agent,
		cfg.WebAPI.Agent.MaxMessages,
		cfg.WebAPI.Agent.MaxTokens,
		cfg.WebAPI.Agent.SummaryMaxTokens,
		logger,
	)

	teamInvitationSvc := service.NewTeamInvitationService(
		teamInvitationRepo,
		repos.outbox,
		outboxTrigger,
		repos.user,
		encryptionKey,
		cfg.WebAPI.FrontendURL,
		logger,
	)

	svcs.authService = authSvc
	svcs.agentService = agentSvc
	svcs.teamInvitationService = teamInvitationSvc

	return svcs, nil
}

type appHandlers struct {
	auth               *handlers.AuthHandler
	user               *handlers.UserHandler
	apiKey             *handlers.APIKeyHandler
	serviceAccount     *handlers.ServiceAccountHandler
	role               *handlers.RoleHandler
	codeSamples        *handlers.CodeSamplesHandler
	contractProxy      *handlers.ContractProxyHandler
	graphqlProxy       *handlers.GraphQLProxyHandler
	agent              *handlers.AgentHandler
	billing            *handlers.BillingHandler
	teamInvitation     *handlers.TeamInvitationHandler
	entityProfileType  *handlers.EntityProfileTypeHandler
	entityProfileProxy *handlers.EntityProfileProxyHandler
}

func initHandlers(cfg *config.Config, svcs *services, repos *repositories, rbacLoader *rbac.Loader, logger *zap.Logger) *appHandlers {
	apiKeySvc := service.NewAPIKeyService(repos.apiKey, repos.serviceAccount, repos.userRole, rbacLoader, logger)
	serviceAccountSvc := service.NewServiceAccountService(repos.serviceAccount, repos.userRole)
	entityProfileTypeSvc := service.NewEntityProfileTypeService(repos.entityProfileType, logger)

	return &appHandlers{
		auth:               handlers.NewAuthHandler(svcs.authService),
		user:               handlers.NewUserHandler(repos.user, repos.company, apiKeySvc),
		apiKey:             handlers.NewAPIKeyHandler(apiKeySvc, repos.user),
		serviceAccount:     handlers.NewServiceAccountHandler(serviceAccountSvc, rbacLoader),
		role:               handlers.NewRoleHandler(rbacLoader),
		codeSamples:        handlers.NewCodeSamplesHandler("./code_samples"),
		contractProxy:      handlers.NewContractProxyHandler(cfg.WebAPI.ThreadifyEngine.URL),
		graphqlProxy:       handlers.NewGraphQLProxyHandler(cfg.WebAPI.ThreadifyEngine.GraphQLURL, logger),
		agent:              handlers.NewAgentHandler(svcs.agentService, logger),
		billing:            handlers.NewBillingHandler(svcs.billingService, logger),
		teamInvitation:     handlers.NewTeamInvitationHandler(svcs.teamInvitationService, repos.company, logger),
		entityProfileType:  handlers.NewEntityProfileTypeHandler(entityProfileTypeSvc),
		entityProfileProxy: handlers.NewEntityProfileProxyHandler(cfg.WebAPI.ThreadifyEngine.GraphQLURL, logger),
	}
}

func buildRouter(cfg *config.Config, svcs *services, repos *repositories, rbacLoader *rbac.Loader, h *appHandlers, logger *zap.Logger) http.Handler {
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

	requirePerm := func(perm string) gin.HandlerFunc {
		return rbac.RequirePermission(rbacLoader, repos.userRole, repos.serviceAccount, perm)
	}

	api := r.Group("/api")
	api.Use(middleware.AuthAccessTokenAuth(svcs.authService))

	user := api.Group("/user")
	{
		user.GET("/profile", h.user.GetProfile)
		user.POST("/profile", h.user.UpdateProfile)
		user.POST("/mark-instrumentation-done", h.user.MarkInstrumentationDone)
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
	}

	api.POST("/graphql", h.graphqlProxy.ProxyGraphQL)

	chat := api.Group("/chat")
	chat.Use(middleware.AgentCreditCheckMiddleware(svcs.agentService))
	{
		chat.GET("/conversations", h.agent.GetConversations)
		chat.GET("/conversations/:id", h.agent.GetConversation)
		chat.DELETE("/conversations/:id", h.agent.DeleteConversation)
		chat.POST("/ask", h.agent.Chat)
		chat.POST("/conversations/:id/continue", h.agent.ContinueConversation)
	}

	billingGroup := api.Group("/billing")
	{
		billingGroup.GET("/plan", h.billing.GetCurrentPlan)
		billingGroup.POST("/checkout", h.billing.CreateCheckoutSession)
		billingGroup.PUT("/spending-limit", h.billing.UpdateMaxMonthlyCharge)
	}

	entityProfileType := api.Group("/entity-profile-types")
	{
		entityProfileType.POST("", h.entityProfileType.CreateEntityProfileType)
		entityProfileType.GET("", h.entityProfileType.ListEntityProfileTypes)
		entityProfileType.PUT("/:id", h.entityProfileType.UpdateEntityProfileType)
		entityProfileType.DELETE("/:id", h.entityProfileType.ArchiveEntityProfileType)
	}

	entityProfiles := api.Group("/entity-profiles")
	{
		entityProfiles.GET("", h.entityProfileProxy.GetEntityProfile)
		entityProfiles.GET("/types", h.entityProfileProxy.ListEntityProfileTypes)
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

func initDB(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

func loadConfig(logger *zap.Logger) (*config.Config, error) {
	path, err := resolveConfigPath(logger)
	if err != nil {
		return nil, err
	}
	return config.Load(path)
}

func resolveConfigPath(logger *zap.Logger) (string, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		logger.Info("using config path from CONFIG_PATH env", zap.String("path", p))
		return p, nil
	}
	if _, err := os.Stat("/app/config/config.yaml"); err == nil {
		logger.Info("using config path", zap.String("path", "/app/config/config.yaml"))
		return "/app/config/config.yaml", nil
	}

	devPath := "../config/config.yaml"
	if _, err := os.Stat(devPath); err == nil {
		logger.Info("using config path (dev fallback)", zap.String("path", devPath))
		return devPath, nil
	}
	return "", errors.New("config file not found: set CONFIG_PATH env var or provide /app/config/config.yaml")
}

func resolveRBACPaths(logger *zap.Logger) (string, string, error) {
	candidates := []string{
		"/app/shared/rbac",
		"./shared/rbac",
		"../shared/rbac",
	}
	for _, base := range candidates {
		perms := base + "/permissions.json"
		roles := base + "/roles.json"
		if _, err := os.Stat(perms); err != nil {
			continue
		}
		if _, err := os.Stat(roles); err != nil {
			logger.Warn("found permissions.json but roles.json missing", zap.String("base", base))
			continue
		}
		logger.Info("using RBAC paths", zap.String("base", base))
		return perms, roles, nil
	}
	return "", "", errors.New("RBAC files not found in any known location; check deployment configuration")
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
			count, err := repo.PruneProcessed(ctx, cutoff)
			if err != nil {
				logger.Error("pruner: failed to prune old events",
					zap.Error(err),
					zap.String("cutoff", cutoff.Format(time.DateOnly)),
				)
			} else {
				logger.Info("pruner: removed events",
					zap.Int64("count", count),
					zap.String("cutoff", cutoff.Format(time.DateOnly)),
				)
			}
		}
	}
}
