package service

import (
	"context"
	"fmt"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/workerpool"
)

// ThreadServiceBuilder builds ThreadService with dependency injection
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
}

// NewThreadServiceBuilder creates a new builder
func NewThreadServiceBuilder() *ThreadServiceBuilder {
	return &ThreadServiceBuilder{}
}

// WithConfig sets the configuration
func (b *ThreadServiceBuilder) WithConfig(cfg *config.Config) *ThreadServiceBuilder {
	b.cfg = cfg
	return b
}

// WithDatabase sets the PostgreSQL database
func (b *ThreadServiceBuilder) WithDatabase(db *database.PostgresDB) *ThreadServiceBuilder {
	b.db = db
	return b
}

// WithValkey sets the Valkey service
func (b *ThreadServiceBuilder) WithValkey(valkeyService *database.ValkeyService) *ThreadServiceBuilder {
	b.valkeyService = valkeyService
	return b
}

// WithStepEventService sets the step event service
func (b *ThreadServiceBuilder) WithStepEventService(stepEventService *StepEventService) *ThreadServiceBuilder {
	b.stepEventService = stepEventService
	return b
}

// WithThreadRepository sets the thread repository
func (b *ThreadServiceBuilder) WithThreadRepository(threadRepo *valkey.ThreadRepository) *ThreadServiceBuilder {
	b.threadRepo = threadRepo
	return b
}

// WithContractTTL sets the contract TTL in seconds
func (b *ThreadServiceBuilder) WithContractTTL(ttl int) *ThreadServiceBuilder {
	b.contractTTLSeconds = ttl
	return b
}

// WithNATSPublisher sets the NATS publisher
func (b *ThreadServiceBuilder) WithNATSPublisher(publisher NotificationPublisher) *ThreadServiceBuilder {
	b.natsPublisher = publisher
	return b
}

// WithNATSArchivalPublisher sets the NATS archival publisher
func (b *ThreadServiceBuilder) WithNATSArchivalPublisher(publisher *natsrepo.ArchivalPublisher) *ThreadServiceBuilder {
	b.natsArchivalPublisher = publisher
	return b
}

// WithAuthService sets the auth service
func (b *ThreadServiceBuilder) WithAuthService(authService *AuthService) *ThreadServiceBuilder {
	b.authService = authService
	return b
}

// WithWorkerPools sets the worker pools for bounded concurrency
func (b *ThreadServiceBuilder) WithWorkerPools(pools *workerpool.Pools) *ThreadServiceBuilder {
	b.workerPools = pools
	return b
}

// Build constructs the ThreadService with all dependencies
func (b *ThreadServiceBuilder) Build() (*ThreadService, error) {
	// Validate required dependencies
	if b.cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if b.db == nil {
		return nil, fmt.Errorf("database is required")
	}
	if b.valkeyService == nil {
		return nil, fmt.Errorf("valkey service is required")
	}
	if b.threadRepo == nil {
		return nil, fmt.Errorf("thread repository is required")
	}

	// Create cache service
	cacheService := NewCacheService()

	// Create repositories
	contractRepo := postgres.NewContractRepository(b.db.Pool)
	valkeyGraphRepo := valkey.NewContractGraphRepository(b.valkeyService, b.contractTTLSeconds)
	accessRepo := valkey.NewAccessRepository(b.valkeyService, 259200) // 72 hours

	// Create and load Lua scripts
	luaScripts := valkey.NewLuaScriptManager(b.valkeyService)
	if err := luaScripts.LoadScripts(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to load Lua scripts: %v\n", err)
	}

	// Create step state repository and load its scripts
	stepStateRepo := valkey.NewStepStateRepository(b.valkeyService, 604800) // 7 days
	if err := stepStateRepo.LoadScripts(context.Background()); err != nil {
		fmt.Printf("Warning: Failed to load step state repository scripts: %v\n", err)
	}

	// Create invitation service with JWT config
	invitationService := NewInvitationTokenService(b.cfg.JWT.Secret, b.cfg.JWT.Issuer)

	// Create scope resolver
	scopeResolver := NewScopeResolver(b.cfg, valkeyGraphRepo, b.threadRepo)

	// Create activity repository
	activityRepo := valkey.NewActivityRepository(b.valkeyService, b.natsArchivalPublisher)

	// Create PostgreSQL access repository
	postgresAccessRepo := postgres.NewAccessRepository(b.db.Pool)
	accessRepoWithPostgres := valkey.NewAccessRepositoryWithPostgres(b.valkeyService, postgresAccessRepo, 259200)

	// Load RBAC for permissions resolution
	rbacLoader, err := rbac.NewLoader("./shared/rbac/permissions.json", "./shared/rbac/roles.json")
	if err != nil {
		fmt.Printf("Warning: Failed to load RBAC: %v\n", err)
		rbacLoader = nil
	}

	// Set RBAC loader on access repository
	accessRepoWithPostgres.SetRBACLoader(rbacLoader)

	// Create thread access service
	accessService := NewThreadAccessService(accessRepoWithPostgres, cacheService, luaScripts, rbacLoader)

	// Create validation and notification services
	validationService := NewValidationService(b.valkeyService, b.threadRepo)

	// Get worker pools (use nil-safe access - pools may be nil in tests)
	var validationPool, notificationPool *workerpool.Pool
	if b.workerPools != nil {
		validationPool = b.workerPools.Validation
		notificationPool = b.workerPools.Notification
	}
	notificationService := NewNotificationService(validationService, activityRepo, stepStateRepo, cacheService, b.natsPublisher, b.natsArchivalPublisher, accessService, rbacLoader, validationPool, notificationPool)

	// Construct and return the service
	return &ThreadService{
		repo:                  b.threadRepo,
		accessRepo:            accessRepo,
		activityRepo:          activityRepo,
		graphRepo:             valkeyGraphRepo,
		stepEventService:      b.stepEventService,
		cacheManager:          cacheService,
		connectionMgr:         NewConnectionService(),
		contractValidator:     NewContractValidationService(valkeyGraphRepo, contractRepo, cacheService),
		authService:           b.authService,
		accessService:         accessService,
		validationService:     validationService,
		notificationService:   notificationService,
		invitationService:     invitationService,
		scopeResolver:         scopeResolver,
		notificationConsumer:  nil,
		valkeyClient:          b.valkeyService,
		luaScripts:            luaScripts,
		natsArchivalPublisher: b.natsArchivalPublisher,
		rbacLoader:            rbacLoader,
	}, nil
}
