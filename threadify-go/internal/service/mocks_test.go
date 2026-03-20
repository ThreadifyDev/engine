package service

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/pkg/validator"
	"threadify-go/shared/billing"
)

type MockContractRepository struct {
	mock.Mock
}

func (m *MockContractRepository) Create(ctx context.Context, contract *models.Contract) error {
	args := m.Called(ctx, contract)
	return args.Error(0)
}

func (m *MockContractRepository) CreateVersion(ctx context.Context, version *models.ContractVersion) error {
	args := m.Called(ctx, version)
	return args.Error(0)
}

func (m *MockContractRepository) Get(ctx context.Context, id string) (*models.Contract, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Contract), args.Error(1)
}

func (m *MockContractRepository) GetByNameAndCompany(ctx context.Context, name, companyID string) (*models.Contract, error) {
	args := m.Called(ctx, name, companyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Contract), args.Error(1)
}

func (m *MockContractRepository) GetVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	args := m.Called(ctx, contractID, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ContractVersion), args.Error(1)
}

func (m *MockContractRepository) ListByCompany(ctx context.Context, companyID string) ([]*models.Contract, error) {
	args := m.Called(ctx, companyID)
	return args.Get(0).([]*models.Contract), args.Error(1)
}

func (m *MockContractRepository) ListVersions(ctx context.Context, contractID string) ([]*models.ContractVersion, error) {
	args := m.Called(ctx, contractID)
	return args.Get(0).([]*models.ContractVersion), args.Error(1)
}

func (m *MockContractRepository) Update(ctx context.Context, contract *models.Contract) error {
	args := m.Called(ctx, contract)
	return args.Error(0)
}

func (m *MockContractRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockContractRepository) DeleteVersion(ctx context.Context, contractID string, version int) error {
	args := m.Called(ctx, contractID, version)
	return args.Error(0)
}

// MockPlanService
type MockPlanService struct {
	mock.Mock
}

func (m *MockPlanService) CanAfford(ctx context.Context, companyID, meter string) (bool, error) {
	args := m.Called(ctx, companyID, meter)
	return args.Bool(0), args.Error(1)
}

func (m *MockPlanService) ChargeContract(ctx context.Context, companyID string) error {
	args := m.Called(ctx, companyID)
	return args.Error(0)
}

func (m *MockPlanService) ChargeContractVersion(ctx context.Context, companyID string) error {
	args := m.Called(ctx, companyID)
	return args.Error(0)
}

func (m *MockPlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	args := m.Called(ctx, companyID, bytes)
	return args.Error(0)
}

func (m *MockPlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	args := m.Called(ctx, companyID, count)
	return args.Error(0)
}

func (m *MockPlanService) GetCurrentLimits(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	args := m.Called(ctx, companyID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.CreditAccount), args.Error(1)
}

func (m *MockPlanService) GetExternalCustomerID(ctx context.Context, companyID string) (string, error) {
	args := m.Called(ctx, companyID)
	return args.String(0), args.Error(1)
}

func (m *MockPlanService) ProvisionSubscription(ctx context.Context, companyID, externalCustomerID string, initialAmount, maxMonthly int64) error {
	args := m.Called(ctx, companyID, externalCustomerID, initialAmount, maxMonthly)
	return args.Error(0)
}

func (m *MockPlanService) InvalidatePlanCache(ctx context.Context, companyID string) {
	m.Called(ctx, companyID)
}

func (m *MockPlanService) ProcessRollovers(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockPlanService) CheckPayloadSize(ctx context.Context, account *billing.CreditAccount, payloadBytes int64) error {
	args := m.Called(ctx, account, payloadBytes)
	return args.Error(0)
}

func (m *MockPlanService) CheckRateLimit(ctx context.Context, account *billing.CreditAccount) (bool, error) {
	args := m.Called(ctx, account)
	return args.Bool(0), args.Error(1)
}

// MockContractValidator
type MockLowLevelValidator struct {
	mock.Mock
}

func (m *MockLowLevelValidator) Validate(yamlString string) (*validator.Contract, *validator.ValidationResult) {
	args := m.Called(yamlString)
	if args.Get(0) == nil {
		return nil, args.Get(1).(*validator.ValidationResult)
	}
	return args.Get(0).(*validator.Contract), args.Get(1).(*validator.ValidationResult)
}

func (m *MockLowLevelValidator) SerializeContract(contract *validator.Contract) (string, string, error) {
	args := m.Called(contract)
	return args.String(0), args.String(1), args.Error(2)
}

// Ensure mocks implement interfaces
var _ interfaces.ContractRepository = (*MockContractRepository)(nil)
var _ interfaces.PlanService = (*MockPlanService)(nil)
var _ interfaces.ContractValidator = (*MockLowLevelValidator)(nil)
