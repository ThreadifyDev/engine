package service

import (
	"context"
	"errors"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

const (
	accessTTLSeconds    = 259200 // 72 hours
	stepStateTTLSeconds = 604800 // 7 days

	rbacPermissionsPath = "./shared/rbac/permissions.json"
	rbacRolesPath       = "./shared/rbac/roles.json"
)

// ThreadServiceBuilder builds a ThreadService with dependency injection.
type ThreadServiceBuilder struct {
	cfg                   *config.Config
	db                    *database.PostgresDB
	valkeyService         *database.ValkeyService
	stepEventService      *StepEventService
	threadRepo            *valkey.ThreadRepository
	contractTTLSeconds    int
	natsClient            *natsrepo.Client
	natsPublisher         domain.NotificationPublisher
	natsArchivalPublisher *natsrepo.ArchivalPublisher
	authService           *AuthService
	planService           domain.PlanService
	workerPools           *workerpool.Pools
	cacheManager          domain.CacheManager
	logger                *zap.Logger
}

// NewThreadServiceBuilder creates a new builder.
func NewThreadServiceBuilder() *ThreadServiceBuilder {
	return &ThreadServiceBuilder{}
}

func (b *ThreadServiceBuilder) WithConfig(cfg *config.Config) *ThreadServiceBuilder {
	b.cfg = cfg
	return b
}

func (b *ThreadServiceBuilder) WithDatabase(db *database.PostgresDB) *ThreadServiceBuilder {
	b.db = db
	return b
}

func (b *ThreadServiceBuilder) WithValkey(valkeyService *database.ValkeyService) *ThreadServiceBuilder {
	b.valkeyService = valkeyService
	return b
}

func (b *ThreadServiceBuilder) WithStepEventService(ses *StepEventService) *ThreadServiceBuilder {
	b.stepEventService = ses
	return b
}

func (b *ThreadServiceBuilder) WithThreadRepository(threadRepo *valkey.ThreadRepository) *ThreadServiceBuilder {
	b.threadRepo = threadRepo
	return b
}

func (b *ThreadServiceBuilder) WithContractTTL(ttl int) *ThreadServiceBuilder {
	b.contractTTLSeconds = ttl
	return b
}

func (b *ThreadServiceBuilder) WithNATSClient(client *natsrepo.Client) *ThreadServiceBuilder {
	b.natsClient = client
	return b
}

func (b *ThreadServiceBuilder) WithNATSPublisher(publisher domain.NotificationPublisher) *ThreadServiceBuilder {
	b.natsPublisher = publisher
	return b
}

func (b *ThreadServiceBuilder) WithNATSArchivalPublisher(publisher *natsrepo.ArchivalPublisher) *ThreadServiceBuilder {
	b.natsArchivalPublisher = publisher
	return b
}

func (b *ThreadServiceBuilder) WithAuthService(authService *AuthService) *ThreadServiceBuilder {
	b.authService = authService
	return b
}

func (b *ThreadServiceBuilder) WithPlanService(planService domain.PlanService) *ThreadServiceBuilder {
	b.planService = planService
	return b
}

func (b *ThreadServiceBuilder) WithWorkerPools(pools *workerpool.Pools) *ThreadServiceBuilder {
	b.workerPools = pools
	return b
}

func (b *ThreadServiceBuilder) WithCacheManager(cm domain.CacheManager) *ThreadServiceBuilder {
	b.cacheManager = cm
	return b
}

func (b *ThreadServiceBuilder) WithLogger(logger *zap.Logger) *ThreadServiceBuilder {
	b.logger = logger
	return b
}

// Build constructs a ThreadService with all dependencies wired up.
func (b *ThreadServiceBuilder) Build() (*ThreadService, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}

	// --- Repositories ---

	contractRepo := postgres.NewContractRepository(b.db.Pool)
	valkeyGraphRepo := valkey.NewContractGraphRepository(b.valkeyService, b.contractTTLSeconds)

	// accessRepo: raw Valkey access (no PostgreSQL fallback), used for direct thread ops.
	// accessRepoWithPostgres: Valkey + PostgreSQL fallback, used for permission resolution.
	accessRepo := valkey.NewAccessRepository(b.valkeyService, accessTTLSeconds, b.logger)
	postgresAccessRepo := postgres.NewAccessRepository(b.db.Pool)
	accessRepoWithPostgres := valkey.NewAccessRepositoryWithPostgres(b.valkeyService, postgresAccessRepo, accessTTLSeconds, b.logger)

	stepStateRepo := valkey.NewStepStateRepository(b.valkeyService, stepStateTTLSeconds, b.logger)
	activityRepo := valkey.NewActivityRepository(b.natsArchivalPublisher, b.logger)

	// --- Lua scripts ---

	luaScripts := valkey.NewLuaScriptManager(b.valkeyService)
	if err := luaScripts.LoadScripts(context.Background()); err != nil {
		b.logger.Warn("failed to load Lua scripts", zap.Error(err))
	}

	if err := stepStateRepo.LoadScripts(context.Background()); err != nil {
		b.logger.Warn("failed to load step state repository scripts", zap.Error(err))
	}

	// --- RBAC ---

	rbacLoader, err := rbac.NewEmbeddedLoader()
	if err != nil {
		b.logger.Warn("failed to load RBAC, permissions will be empty", zap.Error(err))
		rbacLoader = nil
	}
	accessRepoWithPostgres.SetRBACLoader(rbacLoader)

	// --- Worker pools ---

	var validationPool, notificationPool, writeBackPool *workerpool.Pool
	if b.workerPools != nil {
		validationPool = b.workerPools.Validation
		notificationPool = b.workerPools.Notification
		writeBackPool = b.workerPools.WriteBack
		accessRepoWithPostgres.SetWriteBackPool(writeBackPool)
	}

	// --- Services ---

	cacheService := b.cacheManager
	if cacheService == nil {
		cacheService = NewCacheService(b.logger)
	}
	accessService := NewThreadAccessService(accessRepoWithPostgres, cacheService, luaScripts, rbacLoader, b.logger)
	validationService := NewValidationService(b.threadRepo)

	// Initialize timeout monitor if NATS client is available
	var timeoutMonitor *TimeoutMonitor
	if b.natsClient != nil && b.natsPublisher != nil {
		var err error
		timeoutMonitor, err = InitializeTimeoutMonitor(
			b.natsClient.Conn(),
			b.natsPublisher,
			b.logger,
		)
		if err != nil {
			b.logger.Warn("failed to initialize timeout monitor", zap.Error(err))
		}
	}

	notificationService := NewNotificationService(
		validationService, activityRepo, stepStateRepo, b.threadRepo, cacheService,
		b.natsPublisher, b.natsArchivalPublisher,
		accessService, rbacLoader,
		validationPool, notificationPool,
		timeoutMonitor,
		b.logger,
	)

	waitRepo := valkey.NewWaitRepository(b.valkeyService.Client)
	notificationService.waitRepo = waitRepo

	scopeResolver := NewScopeResolver(b.cfg, valkeyGraphRepo, b.threadRepo, b.logger)
	var notificationConsumer *NotificationConsumer
	if b.natsClient != nil {
		notificationConsumer = NewNotificationConsumer(b.natsClient, scopeResolver, b.logger)
	}

	return &ThreadService{
		waitRepo:              waitRepo,
		timeoutMonitor:        timeoutMonitor,
		repo:                  b.threadRepo,
		accessRepo:            accessRepo,
		activityRepo:          activityRepo,
		graphRepo:             valkeyGraphRepo,
		stepEventService:      b.stepEventService,
		cacheManager:          cacheService,
		connectionMgr:         NewConnectionService(b.logger),
		contractValidator:     NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService, b.logger, valkey.NewSuccessfulContentRepository(b.valkeyService, b.db.Pool)),
		authService:           b.authService,
		accessService:         accessService,
		validationService:     validationService,
		notificationService:   notificationService,
		invitationService:     NewInvitationTokenService(b.cfg.JWT.Secret, b.cfg.JWT.Issuer),
		scopeResolver:         scopeResolver,
		notificationConsumer:  notificationConsumer,
		planService:           b.planService,
		valkeyClient:          b.valkeyService,
		luaScripts:            luaScripts,
		natsArchivalPublisher: b.natsArchivalPublisher,
		rbacLoader:            rbacLoader,
		writeBackPool:         writeBackPool,
		logger:                b.logger,
	}, nil
}

// validate checks that all required fields are set before building.
func (b *ThreadServiceBuilder) validate() error {
	switch {
	case b.cfg == nil:
		return errors.New("config is required")
	case b.db == nil:
		return errors.New("database is required")
	case b.valkeyService == nil:
		return errors.New("valkey service is required")
	case b.threadRepo == nil:
		return errors.New("thread repository is required")
	case b.logger == nil:
		return errors.New("logger is required")
	}
	return nil
}
