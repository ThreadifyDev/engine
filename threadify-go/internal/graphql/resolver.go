package graphql

import (
	"context"
	"errors"
	"fmt"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

type Resolver struct {
	threadRepo          *valkey.ThreadRepository
	stepStateRepo       *valkey.StepStateRepository
	validationRepo      *valkey.ValidationRepository
	accessRepo          *valkey.AccessRepository // For permission checks (hot path)
	threadAccessService *service.ThreadAccessService
	contractValidator   interfaces.ContractGraphValidator
	contractRepo        *postgres.ContractRepository
	refsRepo            *postgres.ThreadRefsRepository         // For batch loading refs
	stepStatePostgres   *postgres.StepStateRepository          // For batch loading steps
	activityRepo        *postgres.ActivityRepository           // For hash chain verification
	actorRepo           *postgres.ActorRepository              // For resolving actor names
	notificationRepo    *postgres.ThreadNotificationRepository // For querying thread notifications
	subStepRepo         *postgres.SubStepRepository            // For querying sub-steps
	planService         interfaces.PlanService
	logger              *zap.Logger
}

func NewResolver(
	threadRepo *valkey.ThreadRepository,
	stepStateRepo *valkey.StepStateRepository,
	validationRepo *valkey.ValidationRepository,
	accessRepo *valkey.AccessRepository,
	threadAccessService *service.ThreadAccessService,
	contractValidator interfaces.ContractGraphValidator,
	contractRepo *postgres.ContractRepository,
	refsRepo *postgres.ThreadRefsRepository,
	stepStatePostgres *postgres.StepStateRepository,
	activityRepo *postgres.ActivityRepository,
	actorRepo *postgres.ActorRepository,
	notificationRepo *postgres.ThreadNotificationRepository,
	subStepRepo *postgres.SubStepRepository,
	planService interfaces.PlanService,
	logger *zap.Logger,
) *Resolver {
	return &Resolver{
		threadRepo:          threadRepo,
		stepStateRepo:       stepStateRepo,
		validationRepo:      validationRepo,
		accessRepo:          accessRepo,
		threadAccessService: threadAccessService,
		contractValidator:   contractValidator,
		contractRepo:        contractRepo,
		refsRepo:            refsRepo,
		stepStatePostgres:   stepStatePostgres,
		activityRepo:        activityRepo,
		actorRepo:           actorRepo,
		notificationRepo:    notificationRepo,
		subStepRepo:         subStepRepo,
		planService:         planService,
		logger:              logger,
	}
}

func (r *Resolver) requireCredit(ctx context.Context, companyID string) error {
	if r.planService == nil {
		return fmt.Errorf("billing unavailable")
	}
	if err := r.planService.CheckCreditAvailable(ctx, companyID, "", 0); err != nil {
		if errors.Is(err, service.ErrInsufficientCredit) || errors.Is(err, service.ErrNoAccount) {
			return fmt.Errorf("payment required: insufficient credits")
		}
		return fmt.Errorf("failed to verify credit balance: %w", err)
	}
	return nil
}
