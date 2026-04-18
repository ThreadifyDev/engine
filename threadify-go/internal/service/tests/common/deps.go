package common

import (
	"testing"

	sharedmocks "threadify-go/shared/mocks"

	"github.com/golang/mock/gomock"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

// MockedDependencies holds mocked versions of service dependencies shared across tests.
type MockedDependencies struct {
	Ctrl *gomock.Controller

	PlanRepo      *sharedmocks.MockPlanRepository
	ContractRepo  *enginemocks.MockContractRepository
	ActorRepo     *enginemocks.MockActorRepository
	AuthRepo      *enginemocks.MockAuthRepository
	StepStateRepo *enginemocks.MockStepStateRepository
	ThreadRepo    *enginemocks.MockThreadRepository
	ActivityRepo  *enginemocks.MockActivityRepository
	BillingRepo   *enginemocks.MockBillingRepository

	Valkey       *enginemocks.MockValkeyClient
	Lua          *enginemocks.MockLuaScriptManager
	CacheManager *enginemocks.MockCacheManager

	PlanSvc *sharedmocks.MockPlanService

	Validator *enginemocks.MockContractValidator
	Logger    *zap.Logger
}

func NewMockDeps(t *testing.T) *MockedDependencies {
	ctrl := gomock.NewController(t)
	return &MockedDependencies{
		Ctrl: ctrl,

		PlanRepo:      sharedmocks.NewMockPlanRepository(ctrl),
		ContractRepo:  enginemocks.NewMockContractRepository(ctrl),
		ActorRepo:     enginemocks.NewMockActorRepository(ctrl),
		AuthRepo:      enginemocks.NewMockAuthRepository(ctrl),
		StepStateRepo: enginemocks.NewMockStepStateRepository(ctrl),
		ThreadRepo:    enginemocks.NewMockThreadRepository(ctrl),
		ActivityRepo:  enginemocks.NewMockActivityRepository(ctrl),
		BillingRepo:   enginemocks.NewMockBillingRepository(ctrl),

		Valkey:       enginemocks.NewMockValkeyClient(ctrl),
		Lua:          enginemocks.NewMockLuaScriptManager(ctrl),
		CacheManager: enginemocks.NewMockCacheManager(ctrl),

		PlanSvc: sharedmocks.NewMockPlanService(ctrl),

		Validator: enginemocks.NewMockContractValidator(ctrl),
		Logger:    zap.NewNop(),
	}
}

func (d *MockedDependencies) NewPlanService(subCfg *config.SubscriptionConfig) *service.PlanService {
	return service.NewPlanService(
		d.PlanRepo,
		d.ContractRepo,
		d.ActorRepo,
		subCfg,
		d.Valkey,
		d.Lua,
		d.Logger,
		0,
	)
}

func (d *MockedDependencies) NewContractService() *service.ContractService {
	return service.NewContractServiceWithValidator(
		d.ContractRepo,
		d.PlanSvc,
		d.Validator,
		d.Logger,
	)
}

func (d *MockedDependencies) NewValidationService() *service.ValidationService {
	return service.NewValidationService(d.Valkey, d.ThreadRepo)
}

func (d *MockedDependencies) NewAuthService(cacheTTL int) *service.AuthService {
	return service.NewAuthService(d.AuthRepo, cacheTTL)
}
