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
		mockRepo.On("GetByNameAndCompany", ctx, name, companyID).Return(nil, nil)
		mockValidator.On("SerializeContract", contract).Return("{}", "{}", nil)
		mockRepo.On("Create", ctx, mock.AnythingOfType("*models.Contract")).Return(nil)
		mockRepo.On("CreateVersion", ctx, mock.AnythingOfType("*models.ContractVersion")).Return(nil)
		mockPlanSvc.On("ChargeContract", ctx, companyID).Return(nil)

		status, _ := svc.CreateContract(ctx, ownerID, companyID, ownerID, name, "test", yaml)

		assert.Equal(t, 201, status)
		mockPlanSvc.AssertExpectations(t)
		mockRepo.AssertExpectations(t)
	})

	t.Run("fails when insufficient credit", func(t *testing.T) {
		mockRepo := new(MockContractRepository)
		mockPlanSvc := new(MockPlanService)
		svc := NewContractService(mockRepo, mockPlanSvc, logger)

		companyID := "company-1"

		mockPlanSvc.On("CanAfford", ctx, companyID, MeterContractCreate).Return(false, nil)

		status, resp := svc.CreateContract(ctx, "owner-1", companyID, "owner-1", "test", "test", "...")

		assert.Equal(t, 402, status)
		assert.Contains(t, resp.(map[string]string)["error"], "insufficient credit")
		mockPlanSvc.AssertExpectations(t)
	})
}
