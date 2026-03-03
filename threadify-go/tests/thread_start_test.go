package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/models"
)

func TestThreadService_HandleStartThread_Success(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator, nil)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for contract loading
	mockContractValidator.On("LoadContractGraphIntoCache", "test-contract", 1).Return(nil)

	// Setup mock for repository save and cache
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)
	mockCache.On("SetThread", mock.AnythingOfType("string"), mock.AnythingOfType("*models.Thread")).Return()

	// Test successful thread start
	req := &models.StartThreadRequest{
		ContractName: "test-contract",
		Role:         "payment_processor",
	}

	response := service.HandleStartThread(context.Background(), req, "owner-123", "company-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "Thread started successfully", response.Message)
	assert.NotEmpty(t, response.ThreadID)

	// Verify all mocks were called
	mock.AssertExpectationsForObjects(t, mockConnection, mockContractValidator, mockValkeyRepo, mockCache)
}

func TestThreadService_HandleStartThread_NotAuthenticated(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator, nil)

	// Setup mock for authentication - user not connected
	mockConnection.On("IsConnected", "unauthorized-user").Return(false)

	req := &models.StartThreadRequest{
		ContractName: "test-contract",
		Role:         "payment_processor",
	}

	response := service.HandleStartThread(context.Background(), req, "unauthorized-user", "company-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "error", response.Status)
	assert.Equal(t, "Not authenticated. Please connect first.", response.Message)
	assert.Empty(t, response.ThreadID)

	// Verify authentication was checked
	mock.AssertExpectationsForObjects(t, mockConnection)
}

func TestThreadService_HandleStartThread_ContractLoadingError(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator, nil)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for contract loading - returns error
	mockContractValidator.On("LoadContractGraphIntoCache", "invalid-contract", 1).Return(assert.AnError)

	req := &models.StartThreadRequest{
		ContractName: "invalid-contract",
		Role:         "payment_processor",
	}

	response := service.HandleStartThread(context.Background(), req, "owner-123", "company-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "error", response.Status)
	assert.Contains(t, response.Message, "Failed to load contract")
	assert.Empty(t, response.ThreadID)

	// Verify all mocks were called
	mock.AssertExpectationsForObjects(t, mockConnection, mockContractValidator)
}

func TestThreadService_HandleStartThread_NoContract(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator, nil)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for repository save and cache (no contract loading for non-contract)
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)
	mockCache.On("SetThread", mock.AnythingOfType("string"), mock.AnythingOfType("*models.Thread")).Return()

	// Test thread start without contract (non-contract workflow)
	req := &models.StartThreadRequest{
		ContractName: "", // Empty contract name for non-contract workflow
		Role:         "", // No role for non-contract workflow
	}

	response := service.HandleStartThread(context.Background(), req, "owner-123", "company-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "Thread started successfully", response.Message)
	assert.NotEmpty(t, response.ThreadID)

	// Verify authentication, repository save, and cache were called (but not contract validator)
	mock.AssertExpectationsForObjects(t, mockConnection, mockValkeyRepo, mockCache)
}

func TestThreadService_HandleStartThread_WithContract(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator, nil)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for contract loading
	mockContractValidator.On("LoadContractGraphIntoCache", "simple-contract", 1).Return(nil)

	// Setup mock for repository save and cache
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)
	mockCache.On("SetThread", mock.AnythingOfType("string"), mock.AnythingOfType("*models.Thread")).Return()

	// Test thread start with contract
	req := &models.StartThreadRequest{
		ContractName: "simple-contract",
		Role:         "processor",
	}

	response := service.HandleStartThread(context.Background(), req, "owner-123", "company-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "Thread started successfully", response.Message)
	assert.NotEmpty(t, response.ThreadID)

	// Verify authentication, contract validator, repository save, and cache were called
	mock.AssertExpectationsForObjects(t, mockConnection, mockContractValidator, mockValkeyRepo, mockCache)
}
