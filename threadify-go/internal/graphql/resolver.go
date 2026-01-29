package graphql

import (
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/service"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	threadRepo          *valkey.ThreadRepository
	stepStateRepo       *valkey.StepStateRepository
	validationRepo      *valkey.ValidationRepository
	accessRepo          *valkey.AccessRepository // For permission checks (hot path)
	threadAccessService *service.ThreadAccessService
	contractValidator   interfaces.ContractValidator
	contractRepo        *postgres.ContractRepository
	refsRepo            *postgres.ThreadRefsRepository // For batch loading refs
	stepStatePostgres   *postgres.StepStateRepository  // For batch loading steps
	activityRepo        *postgres.ActivityRepository   // For hash chain verification
}

func NewResolver(threadRepo *valkey.ThreadRepository, stepStateRepo *valkey.StepStateRepository, validationRepo *valkey.ValidationRepository, accessRepo *valkey.AccessRepository, threadAccessService *service.ThreadAccessService, contractValidator interfaces.ContractValidator, contractRepo *postgres.ContractRepository, refsRepo *postgres.ThreadRefsRepository, stepStatePostgres *postgres.StepStateRepository, activityRepo *postgres.ActivityRepository) *Resolver {
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
	}
}
