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
	threadAccessService *service.ThreadAccessService
	contractValidator   interfaces.ContractValidator
	contractRepo        *postgres.ContractRepository
	refsRepo            *postgres.ThreadRefsRepository // For batch loading refs
	stepStatePostgres   *postgres.StepStateRepository  // For batch loading steps
}

func NewResolver(threadRepo *valkey.ThreadRepository, stepStateRepo *valkey.StepStateRepository, validationRepo *valkey.ValidationRepository, threadAccessService *service.ThreadAccessService, contractValidator interfaces.ContractValidator, contractRepo *postgres.ContractRepository, refsRepo *postgres.ThreadRefsRepository, stepStatePostgres *postgres.StepStateRepository) *Resolver {
	return &Resolver{
		threadRepo:          threadRepo,
		stepStateRepo:       stepStateRepo,
		validationRepo:      validationRepo,
		threadAccessService: threadAccessService,
		contractValidator:   contractValidator,
		contractRepo:        contractRepo,
		refsRepo:            refsRepo,
		stepStatePostgres:   stepStatePostgres,
	}
}
