package graphql

import (
	sharedrepo "threadify-go/shared/repository"

	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/internal/types"
	"go.uber.org/zap"
)

type Resolver struct {
	threadRepo            *valkey.ThreadRepository
	stepStateRepo         *valkey.StepStateRepository
	validationRepo        *valkey.ValidationRepository
	accessRepo            *valkey.AccessRepository // For permission checks (hot path)
	threadAccessService   *service.ThreadAccessService
	contractValidator     types.ContractGraphValidator
	contractRepo          *postgres.ContractRepository
	refsRepo              *postgres.ThreadRefsRepository         // For batch loading refs
	stepStatePostgres     *postgres.StepStateRepository          // For batch loading steps
	activityRepo          *postgres.ActivityRepository           // For hash chain verification
	actorRepo             *postgres.ActorRepository              // For resolving actor names
	notificationRepo      *postgres.ThreadNotificationRepository // For querying thread notifications
	subStepRepo           *postgres.SubStepRepository            // For querying sub-steps
	entityProfileRepo     sharedrepo.EntityProfileRepository
	entityProfileTypeRepo sharedrepo.EntityProfileTypeRepository
	metricsRepo           *postgres.MetricsRepository
	planService           types.PlanService
	logger                *zap.Logger
}

func NewResolver(
	threadRepo *valkey.ThreadRepository,
	stepStateRepo *valkey.StepStateRepository,
	validationRepo *valkey.ValidationRepository,
	accessRepo *valkey.AccessRepository,
	threadAccessService *service.ThreadAccessService,
	contractValidator types.ContractGraphValidator,
	contractRepo *postgres.ContractRepository,
	refsRepo *postgres.ThreadRefsRepository,
	stepStatePostgres *postgres.StepStateRepository,
	activityRepo *postgres.ActivityRepository,
	actorRepo *postgres.ActorRepository,
	notificationRepo *postgres.ThreadNotificationRepository,
	subStepRepo *postgres.SubStepRepository,
	entityProfileRepo sharedrepo.EntityProfileRepository,
	entityProfileTypeRepo sharedrepo.EntityProfileTypeRepository,
	metricsRepo *postgres.MetricsRepository,
	planService types.PlanService,
	logger *zap.Logger,
) *Resolver {
	return &Resolver{
		threadRepo:            threadRepo,
		stepStateRepo:         stepStateRepo,
		validationRepo:        validationRepo,
		accessRepo:            accessRepo,
		threadAccessService:   threadAccessService,
		contractValidator:     contractValidator,
		contractRepo:          contractRepo,
		refsRepo:              refsRepo,
		stepStatePostgres:     stepStatePostgres,
		activityRepo:          activityRepo,
		actorRepo:             actorRepo,
		notificationRepo:      notificationRepo,
		subStepRepo:           subStepRepo,
		entityProfileRepo:     entityProfileRepo,
		entityProfileTypeRepo: entityProfileTypeRepo,
		metricsRepo:           metricsRepo,
		planService:           planService,
		logger:                logger,
	}
}
