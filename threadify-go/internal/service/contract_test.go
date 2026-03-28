package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/pkg/validator"
	"go.uber.org/zap"
)

func TestContractService_CreateContract(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("successful creation with credit check and charge", func(t *testing.T) {
		mockRepo := new(MockContractRepository)
		mockPlanSvc := new(MockPlanService)
		mockValidator := new(MockLowLevelValidator)

		svc := NewContractService(mockRepo, mockPlanSvc, logger)
		svc.validator = mockValidator // Override for mocking

		ownerID := "owner-1"
		companyID := "company-1"
		name := "test-contract"
		yaml := "contract_name: test-contract\nversion: 1\ndescription: test"

		contract := &validator.Contract{
			ContractName: name,
			Version:      1,
			Description:  "test",
		}

		// Expectations
		mockValidator.On("Validate", yaml).Return(contract, &validator.ValidationResult{IsValid: true})
		mockPlanSvc.On("CheckCreditAvailable", ctx, companyID, MeterContractCreate, int64(1)).Return(nil)
		mockValidator.On("SerializeContract", contract).Return("{}", "{}", nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Contract")).Return(nil)
		mockRepo.On("CreateVersion", ctx, mock.AnythingOfType("*models.ContractVersion")).Return(nil)
		mockPlanSvc.On("ChargeContract", ctx, companyID).Return(nil)

		status, _ := svc.CreateContract(ctx, ownerID, companyID, ownerID, yaml)

		assert.Equal(t, 200, status)
		mockPlanSvc.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("fails when insufficient credit", func(t *testing.T) {
		mockRepo := new(MockContractRepository)
		mockPlanSvc := new(MockPlanService)
		mockValidator := new(MockLowLevelValidator)
		svc := NewContractService(mockRepo, mockPlanSvc, logger)
		svc.validator = mockValidator

		companyID := "company-1"
		yaml := "contract_name: test-contract\nversion: 1\ndescription: test"
		contract := &validator.Contract{
			ContractName: "test-contract",
			Version:      1,
			Description:  "test",
		}

		mockValidator.On("Validate", yaml).Return(contract, &validator.ValidationResult{IsValid: true})
		mockPlanSvc.On("CheckCreditAvailable", ctx, companyID, MeterContractCreate, int64(1)).Return(ErrInsufficientCredit)

		status, resp := svc.CreateContract(ctx, "owner-1", companyID, "owner-1", yaml)

		assert.Equal(t, 402, status)
		assert.Contains(t, resp.(map[string]string)["message"], "insufficient credit balance")
		mockPlanSvc.AssertExpectations(t)
		mockRepo.AssertNotCalled(t, "Create", ctx, mock.Anything)
		mockRepo.AssertNotCalled(t, "CreateVersion", ctx, mock.Anything)
	})
}
