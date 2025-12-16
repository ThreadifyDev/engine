package service

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

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for contract loading
	mockContractValidator.On("LoadContractGraphIntoCache", "test-contract", 1).Return(nil)

	// Setup mock for repository save and cache
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)
	mockCache.On("SetThread", mock.AnythingOfType("string"), mock.AnythingOfType("*models.Thread")).Return()

	// Test successful thread start
	req := &models.StartThreadRequest{
		ContractID: "test-contract:1",
	}

	response := service.HandleStartThread(req, "owner-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "Thread started successfully", response.Message)
	assert.NotEmpty(t, response.ThreadID)
	assert.Equal(t, "test-contract:1", response.ContractID)

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

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator)

	// Setup mock for authentication - user not connected
	mockConnection.On("IsConnected", "unauthorized-user").Return(false)

	req := &models.StartThreadRequest{
		ContractID: "test-contract:1",
	}

	response := service.HandleStartThread(req, "unauthorized-user")

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

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for contract loading - returns error
	mockContractValidator.On("LoadContractGraphIntoCache", "invalid-contract", 1).Return(assert.AnError)

	req := &models.StartThreadRequest{
		ContractID: "invalid-contract:1",
	}

	response := service.HandleStartThread(req, "owner-123")

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

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator)

	// Setup mock for authentication
	mockConnection.On("IsConnected", "owner-123").Return(true)

	// Setup mock for repository save and cache (no contract loading)
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)
	mockCache.On("SetThread", mock.AnythingOfType("string"), mock.AnythingOfType("*models.Thread")).Return()

	// Test thread start without contract
	req := &models.StartThreadRequest{
		ContractID: "", // No contract specified
	}

	response := service.HandleStartThread(req, "owner-123")

	assert.Equal(t, "startThread", response.Action)
	assert.Equal(t, "success", response.Status)
	assert.Equal(t, "Thread started successfully", response.Message)
	assert.NotEmpty(t, response.ThreadID)
	assert.Empty(t, response.ContractID)

	// Verify authentication, repository save, and cache were called
	mock.AssertExpectationsForObjects(t, mockConnection, mockValkeyRepo, mockCache)
}
