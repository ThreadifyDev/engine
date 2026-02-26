package service

import (
	"context"
	"errors"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
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
	natsPublisher         NotificationPublisher
	natsArchivalPublisher *natsrepo.ArchivalPublisher
	authService           *AuthService
	workerPools           *workerpool.Pools
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

func (b *ThreadServiceBuilder) WithNATSPublisher(publisher NotificationPublisher) *ThreadServiceBuilder {
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

func (b *ThreadServiceBuilder) WithWorkerPools(pools *workerpool.Pools) *ThreadServiceBuilder {
	b.workerPools = pools
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
	accessRepo := valkey.NewAccessRepository(b.valkeyService, accessTTLSeconds)
	postgresAccessRepo := postgres.NewAccessRepository(b.db.Pool)
	accessRepoWithPostgres := valkey.NewAccessRepositoryWithPostgres(b.valkeyService, postgresAccessRepo, accessTTLSeconds)

	stepStateRepo := valkey.NewStepStateRepository(b.valkeyService, stepStateTTLSeconds)
	activityRepo := valkey.NewActivityRepository(b.valkeyService, b.natsArchivalPublisher, b.logger)

	// --- Lua scripts ---

	luaScripts := valkey.NewLuaScriptManager(b.valkeyService)
	if err := luaScripts.LoadScripts(context.Background()); err != nil {
		b.logger.Warn("failed to load Lua scripts", zap.Error(err))
	}

	if err := stepStateRepo.LoadScripts(context.Background()); err != nil {
		b.logger.Warn("failed to load step state repository scripts", zap.Error(err))
	}

	// --- RBAC ---

	rbacLoader, err := rbac.NewLoader(rbacPermissionsPath, rbacRolesPath)
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

	cacheService := NewCacheService(b.logger)
	accessService := NewThreadAccessService(accessRepoWithPostgres, cacheService, luaScripts, rbacLoader, b.logger)
	validationService := NewValidationService(b.valkeyService, b.threadRepo)
	notificationService := NewNotificationService(
		validationService, activityRepo, stepStateRepo, cacheService,
		b.natsPublisher, b.natsArchivalPublisher,
		accessService, rbacLoader,
		validationPool, notificationPool,
		b.logger,
	)

	return &ThreadService{
		repo:                  b.threadRepo,
		accessRepo:            accessRepo,
		activityRepo:          activityRepo,
		graphRepo:             valkeyGraphRepo,
		stepEventService:      b.stepEventService,
		cacheManager:          cacheService,
		connectionMgr:         NewConnectionService(b.logger),
		contractValidator:     NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService, b.logger),
		authService:           b.authService,
		accessService:         accessService,
		validationService:     validationService,
		notificationService:   notificationService,
		invitationService:     NewInvitationTokenService(b.cfg.JWT.Secret, b.cfg.JWT.Issuer),
		scopeResolver:         NewScopeResolver(b.cfg, valkeyGraphRepo, b.threadRepo, b.logger),
		notificationConsumer:  nil,
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
