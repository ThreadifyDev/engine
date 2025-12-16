package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/threadify/engine/internal/models"
)

func TestParseContractIdentifier(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedName    string
		expectedVersion int
	}{
		{
			name:            "contract name only",
			input:           "product_delivery",
			expectedName:    "product_delivery",
			expectedVersion: 0,
		},
		{
			name:            "contract name with version",
			input:           "product_delivery:2",
			expectedName:    "product_delivery",
			expectedVersion: 2,
		},
		{
			name:            "contract name with version 1",
			input:           "order_processing:1",
			expectedName:    "order_processing",
			expectedVersion: 1,
		},
		{
			name:            "UUID without version",
			input:           "550e8400-e29b-41d4-a716-446655440000",
			expectedName:    "550e8400-e29b-41d4-a716-446655440000",
			expectedVersion: 0,
		},
		{
			name:            "contract with colon in name but invalid version",
			input:           "test:invalid",
			expectedName:    "test:invalid",
			expectedVersion: 0,
		},
		{
			name:            "contract with zero version",
			input:           "test:0",
			expectedName:    "test:0",
			expectedVersion: 0,
		},
		{
			name:            "contract with negative version",
			input:           "test:-1",
			expectedName:    "test:-1",
			expectedVersion: 0,
		},
		{
			name:            "contract with multiple colons",
			input:           "namespace:contract:2",
			expectedName:    "namespace:contract",
			expectedVersion: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, version := parseContractIdentifier(tt.input)
			assert.Equal(t, tt.expectedName, name, "name mismatch")
			assert.Equal(t, tt.expectedVersion, version, "version mismatch")
		})
	}
}

// Mock repositories for tests
type MockContractRepository struct {
	mock.Mock
}

func (m *MockContractRepository) GetByID(ctx interface{}, contractID string) (*models.Contract, error) {
	args := m.Called(ctx, contractID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Contract), args.Error(1)
}

func (m *MockContractRepository) GetLatestVersion(ctx interface{}, contractID string) (*models.ContractVersion, error) {
	args := m.Called(ctx, contractID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ContractVersion), args.Error(1)
}

// TODO: Complete these tests once service initialization with mocks is implemented
/*
func TestThreadService_LoadContractGraph(t *testing.T) {
	t.Run("successfully loads contract graph into valkey", func(t *testing.T) {
		mockContractRepo := new(MockContractRepository)
		mockGraphRepo := new(MockContractGraphRepository)

		// Mock contract exists
		contract := &models.Contract{
			ID:            "contract123",
			Name:          "test-contract",
			LatestVersion: 2,
		}
		mockContractRepo.On("GetByID", mock.Anything, "contract123").Return(contract, nil)

		// Mock contract version with graph
		graphData := []byte(`{"contractId":"contract123","version":2,"graph":{"nodes":{},"finalStep":"end"}}`)
		contractVersion := &models.ContractVersion{
			ID:         "version123",
			Version:    2,
			ContractID: "contract123",
			Graph:      graphData,
		}
		mockContractRepo.On("GetLatestVersion", mock.Anything, "contract123").Return(contractVersion, nil)

		// Mock graph save to valkey (now takes contractID and version as separate params)
		mockGraphRepo.On("Save", mock.Anything, "contract123", 2, mock.AnythingOfType("*models.ContractGraph")).Return(nil)

		// Test would call the method here once implemented
		// err := service.LoadContractGraph(ctx, "contract123")
		// assert.NoError(t, err)

		mockContractRepo.AssertExpectations(t)
		mockGraphRepo.AssertExpectations(t)
	})

	t.Run("returns error when contract does not exist", func(t *testing.T) {
		mockContractRepo := new(MockContractRepository)
		mockGraphRepo := new(MockContractGraphRepository)

		// Mock contract not found
		mockContractRepo.On("GetByID", mock.Anything, "nonexistent").Return(nil, assert.AnError)

		// Test would call the method here once implemented
		// err := service.LoadContractGraph(ctx, "nonexistent")
		// assert.Error(t, err)
		// assert.Contains(t, err.Error(), "contract not found")

		mockContractRepo.AssertExpectations(t)
		mockGraphRepo.AssertNotCalled(t, "Save")
	})

	t.Run("returns error when contract version has no graph", func(t *testing.T) {
		mockContractRepo := new(MockContractRepository)
		mockGraphRepo := new(MockContractGraphRepository)

		contract := &models.Contract{
			ID:            "contract123",
			Name:          "test-contract",
			LatestVersion: 1,
		}
		mockContractRepo.On("GetByID", mock.Anything, "contract123").Return(contract, nil)

		// Mock contract version without graph
		contractVersion := &models.ContractVersion{
			ID:         "version123",
			Version:    1,
			ContractID: "contract123",
			Graph:      nil,
		}
		mockContractRepo.On("GetLatestVersion", mock.Anything, "contract123").Return(contractVersion, nil)

		// Test would call the method here once implemented
		// err := service.LoadContractGraph(ctx, "contract123")
		// assert.Error(t, err)
		// assert.Contains(t, err.Error(), "no graph data")

		mockContractRepo.AssertExpectations(t)
		mockGraphRepo.AssertNotCalled(t, "Save")
	})

	t.Run("returns error when graph save to valkey fails", func(t *testing.T) {
		mockContractRepo := new(MockContractRepository)
		mockGraphRepo := new(MockContractGraphRepository)

		contract := &models.Contract{
			ID:            "contract123",
			Name:          "test-contract",
			LatestVersion: 1,
		}
		mockContractRepo.On("GetByID", mock.Anything, "contract123").Return(contract, nil)

		graphData := []byte(`{"contractId":"contract123","version":1}`)
		contractVersion := &models.ContractVersion{
			ID:         "version123",
			Version:    1,
			ContractID: "contract123",
			Graph:      graphData,
		}
		mockContractRepo.On("GetLatestVersion", mock.Anything, "contract123").Return(contractVersion, nil)

		// Mock graph save failure
		mockGraphRepo.On("Save", mock.Anything, mock.Anything).Return(assert.AnError)

		// Test would call the method here once implemented
		// err := service.LoadContractGraph(ctx, "contract123")
		// assert.Error(t, err)
		// assert.Contains(t, err.Error(), "failed to save graph")

		mockContractRepo.AssertExpectations(t)
		mockGraphRepo.AssertExpectations(t)
	})
}
*/

// TODO: Complete these tests once service initialization with mocks is implemented
// func TestThreadService_HandleStartThread_WithContractValidation(t *testing.T) {
// 	t.Run("validates contract exists before starting thread", func(t *testing.T) {
// 		mockContractRepo := new(MockContractRepository)
// 		mockGraphRepo := new(MockContractGraphRepository)
//
// 		// Mock contract exists
// 		contract := &models.Contract{
// 			ID:            "contract123",
// 			Name:          "test-contract",
// 			LatestVersion: 1,
// 		}
// 		mockContractRepo.On("GetByID", mock.Anything, "contract123").Return(contract, nil)
//
// 		// Mock contract version
// 		graphData := []byte(`{"contractId":"contract123","version":1}`)
// 		contractVersion := &models.ContractVersion{
// 			ID:         "version123",
// 			Version:    1,
// 			ContractID: "contract123",
// 			Graph:      graphData,
// 		}
// 		mockContractRepo.On("GetLatestVersion", mock.Anything, "contract123").Return(contractVersion, nil)
//
// 		// Mock graph save
// 		mockGraphRepo.On("Save", mock.Anything, mock.Anything).Return(nil)
//
// 		// Test would verify contract validation happens
// 		// Once we have proper service initialization with mocks:
// 		// req := &models.StartThreadRequest{ContractID: "contract123"}
// 		// response := service.HandleStartThread(req, "owner123")
// 		// assert.Equal(t, "success", response.Status)
//
// 		mockContractRepo.AssertExpectations(t)
// 	})
//
// 	t.Run("returns error when contract does not exist", func(t *testing.T) {
// 		mockContractRepo := new(MockContractRepository)
//
// 		// Mock contract not found
// 		mockContractRepo.On("GetByID", mock.Anything, "nonexistent").Return(nil, assert.AnError)
//
// 		// Test would verify error is returned
// 		// Once we have proper service initialization with mocks:
// 		// req := &models.StartThreadRequest{ContractID: "nonexistent"}
// 		// response := service.HandleStartThread(req, "owner123")
// 		// assert.Equal(t, "error", response.Status)
// 		// assert.Contains(t, response.Message, "contract not found")
//
// 		mockContractRepo.AssertExpectations(t)
// 	})
// }

// MockThreadRepository for testing
type MockThreadRepository struct {
	mock.Mock
}

func (m *MockThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	args := m.Called(ctx, thread)
	return args.Error(0)
}

func (m *MockThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	args := m.Called(ctx, threadID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Thread), args.Error(1)
}

func (m *MockThreadRepository) Delete(ctx context.Context, threadID string) error {
	args := m.Called(ctx, threadID)
	return args.Error(0)
}

func (m *MockThreadRepository) Exists(ctx context.Context, threadID string) (bool, error) {
	args := m.Called(ctx, threadID)
	return args.Bool(0), args.Error(1)
}

func (m *MockThreadRepository) GetByOwner(ctx context.Context, ownerID string) ([]string, error) {
	args := m.Called(ctx, ownerID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockThreadRepository) ExtendTTL(ctx context.Context, threadID string) error {
	args := m.Called(ctx, threadID)
	return args.Error(0)
}

// MockContractGraphRepository for testing
type MockContractGraphRepository struct {
	mock.Mock
}

func (m *MockContractGraphRepository) Get(ctx context.Context, contractID string, version int) (*models.ContractGraph, error) {
	args := m.Called(ctx, contractID, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ContractGraph), args.Error(1)
}

func (m *MockContractGraphRepository) Save(ctx context.Context, contractID string, version int, graph *models.ContractGraph) error {
	args := m.Called(ctx, contractID, version, graph)
	return args.Error(0)
}

func (m *MockContractGraphRepository) Delete(ctx context.Context, contractID string, version int) error {
	args := m.Called(ctx, contractID, version)
	return args.Error(0)
}

func (m *MockContractGraphRepository) Exists(ctx context.Context, contractID string, version int) (bool, error) {
	args := m.Called(ctx, contractID, version)
	return args.Bool(0), args.Error(1)
}

// MockStepEventService for testing
type MockStepEventService struct {
	mock.Mock
}

func (m *MockStepEventService) ProcessStepEvent(event models.StepEvent) error {
	args := m.Called(event)
	return args.Error(0)
}

func (m *MockStepEventService) Start() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStepEventService) Stop() error {
	args := m.Called()
	return args.Error(0)
}

// MockCacheManager is a mock implementation of CacheManager interface
type MockCacheManager struct {
	mock.Mock
}

func (m *MockCacheManager) GetThread(threadID string) (*models.Thread, bool) {
	args := m.Called(threadID)
	return args.Get(0).(*models.Thread), args.Bool(1)
}

func (m *MockCacheManager) SetThread(threadID string, thread *models.Thread) {
	m.Called(threadID, thread)
}

func (m *MockCacheManager) GetContractGraph(contractID string, version int) (*models.ContractGraph, bool) {
	args := m.Called(contractID, version)
	return args.Get(0).(*models.ContractGraph), args.Bool(1)
}

func (m *MockCacheManager) SetContractGraph(contractID string, version int, graph *models.ContractGraph) {
	m.Called(contractID, version, graph)
}

func (m *MockCacheManager) ClearThreadCache(threadID string) {
	m.Called(threadID)
}

func (m *MockCacheManager) ClearContractCache(contractID string, version int) {
	m.Called(contractID, version)
}

// MockConnectionManager is a mock implementation of ConnectionManager interface
type MockConnectionManager struct {
	mock.Mock
}

func (m *MockConnectionManager) Connect(ownerID, apiKey, serviceName string) error {
	args := m.Called(ownerID, apiKey, serviceName)
	return args.Error(0)
}

func (m *MockConnectionManager) Disconnect(ownerID string) error {
	args := m.Called(ownerID)
	return args.Error(0)
}

func (m *MockConnectionManager) GetClient(ownerID string) (*models.ConnectedClient, bool) {
	args := m.Called(ownerID)
	return args.Get(0).(*models.ConnectedClient), args.Bool(1)
}

func (m *MockConnectionManager) IsConnected(ownerID string) bool {
	args := m.Called(ownerID)
	return args.Bool(0)
}

// MockContractValidator is a mock implementation of ContractValidator interface
type MockContractValidator struct {
	mock.Mock
}

func (m *MockContractValidator) ValidateStepInContract(contractID string, version int, stepName string, context map[string]string) error {
	args := m.Called(contractID, version, stepName, context)
	return args.Error(0)
}

func (m *MockContractValidator) GetContractGraph(contractID string, version int) (*models.ContractGraph, error) {
	args := m.Called(contractID, version)
	return args.Get(0).(*models.ContractGraph), args.Error(1)
}

func (m *MockContractValidator) LoadContractGraphIntoCache(contractID string, version int) error {
	args := m.Called(contractID, version)
	return args.Error(0)
}

func TestThreadService_HandleRecordEvent_RequiredFields(t *testing.T) {
	// Setup
	mockValkeyRepo := &MockThreadRepository{}
	mockGraphRepo := &MockContractGraphRepository{}
	mockStepEvent := &MockStepEventService{}
	mockCache := &MockCacheManager{}
	mockConnection := &MockConnectionManager{}
	mockContractValidator := &MockContractValidator{}

	service := NewThreadService(mockValkeyRepo, mockGraphRepo, mockStepEvent, mockCache, mockConnection, mockContractValidator)

	// Setup mock for authentication - these tests don't check auth, so return true
	mockConnection.On("IsConnected", mock.Anything).Return(true)

	// Setup mock for contract validation - these tests don't check validation, so return nil
	mockContractValidator.On("ValidateStepInContract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	tests := []struct {
		name    string
		request *models.RecordEventRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "missing thread ID",
			request: &models.RecordEventRequest{
				ThreadID:   "",
				StepName:   "test_step",
				StartedAt:  "2025-01-15T10:00:00Z",
				FinishedAt: "2025-01-15T10:05:00Z",
				Status:     "completed",
				Context:    map[string]string{"key": "value"},
			},
			wantErr: true,
			errMsg:  "Thread ID is required",
		},
		{
			name: "missing step name",
			request: &models.RecordEventRequest{
				ThreadID:   "thread-123",
				StepName:   "",
				StartedAt:  "2025-01-15T10:00:00Z",
				FinishedAt: "2025-01-15T10:05:00Z",
				Status:     "completed",
				Context:    map[string]string{"key": "value"},
			},
			wantErr: true,
			errMsg:  "StepName is required",
		},
		{
			name: "missing status",
			request: &models.RecordEventRequest{
				ThreadID:   "thread-123",
				StepName:   "test_step",
				StartedAt:  "2025-01-15T10:00:00Z",
				FinishedAt: "2025-01-15T10:05:00Z",
				Status:     "",
				Context:    map[string]string{"key": "value"},
			},
			wantErr: true,
			errMsg:  "Status is required",
		},
		{
			name: "missing started at",
			request: &models.RecordEventRequest{
				ThreadID:   "thread-123",
				StepName:   "test_step",
				StartedAt:  "",
				FinishedAt: "2025-01-15T10:05:00Z",
				Status:     "completed",
				Context:    map[string]string{"key": "value"},
			},
			wantErr: true,
			errMsg:  "StartedAt is required",
		},
		{
			name: "missing finished at",
			request: &models.RecordEventRequest{
				ThreadID:   "thread-123",
				StepName:   "test_step",
				StartedAt:  "2025-01-15T10:00:00Z",
				FinishedAt: "",
				Status:     "completed",
				Context:    map[string]string{"key": "value"},
			},
			wantErr: true,
			errMsg:  "FinishedAt is required",
		},
		{
			name: "missing context",
			request: &models.RecordEventRequest{
				ThreadID:   "thread-123",
				StepName:   "test_step",
				StartedAt:  "2025-01-15T10:00:00Z",
				FinishedAt: "2025-01-15T10:05:00Z",
				Status:     "completed",
				Context:    nil,
			},
			wantErr: true,
			errMsg:  "Context is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add connected client for authentication
			ownerID := "owner-123"
			client := &models.ConnectedClient{
				OwnerID:     ownerID,
				ApiKey:      "test-key",
				ServiceName: "test-service",
			}
			mockConnection.On("GetClient", ownerID).Return(client, true)
			mockConnection.On("IsConnected", ownerID).Return(true)

			response := service.HandleRecordEvent(tt.request, ownerID)

			if tt.wantErr {
				assert.Equal(t, "error", response.Status)
				assert.Contains(t, response.Message, tt.errMsg)
			} else {
				assert.Equal(t, "success", response.Status)
			}
		})
	}
}

func TestThreadService_HandleRecordEvent_DuplicateStepPrevention(t *testing.T) {
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

	// Setup mock for contract validation
	mockContractValidator.On("ValidateStepInContract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Create a thread with a completed step
	threadID := "thread-123"
	stepName := "completed_step"

	thread := &models.Thread{
		ID:      threadID,
		OwnerID: "owner-123",
		Steps: map[string]*models.StepState{
			stepName: {
				ID:          stepName,
				Status:      "completed",
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				RetryCount:  0,
				IsCompleted: true,
			},
		},
	}

	// Add connected client
	ownerID := "owner-123"
	client := &models.ConnectedClient{
		OwnerID:     ownerID,
		ApiKey:      "test-key",
		ServiceName: "test-service",
	}
	mockConnection.On("GetClient", ownerID).Return(client, true)

	// Put thread in cache
	mockCache.On("GetThread", threadID).Return(thread, true)
	mockCache.On("SetThread", threadID, thread).Return()

	// Try to record the same step again
	request := &models.RecordEventRequest{
		ThreadID:   threadID,
		StepName:   stepName,
		StartedAt:  "2025-01-15T10:00:00Z",
		FinishedAt: "2025-01-15T10:05:00Z",
		Status:     "completed",
		Context:    map[string]string{"key": "value"},
	}

	response := service.HandleRecordEvent(request, ownerID)

	// Should fail with duplicate error
	assert.Equal(t, "error", response.Status)
	assert.Contains(t, response.Message, "already completed")
}

func TestThreadService_HandleRecordEvent_RetryTracking(t *testing.T) {
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

	// Setup mock for contract validation
	mockContractValidator.On("ValidateStepInContract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	threadID := "thread-123"
	stepName := "retry_step"

	// Create a thread with a failed step
	thread := &models.Thread{
		ID:      threadID,
		OwnerID: "owner-123",
		Steps: map[string]*models.StepState{
			stepName: {
				ID:          stepName,
				Status:      "failed",
				CreatedAt:   time.Now().Add(-time.Hour),
				UpdatedAt:   time.Now().Add(-time.Hour),
				RetryCount:  1,
				IsCompleted: false,
			},
		},
	}

	// Mock thread save
	mockValkeyRepo.On("Save", mock.Anything, mock.AnythingOfType("*models.Thread")).Return(nil)

	// Mock step event processing
	mockStepEvent.On("ProcessStepEvent", mock.AnythingOfType("models.StepEvent")).Return(nil)

	// Add connected client
	ownerID := "owner-123"
	client := &models.ConnectedClient{
		OwnerID:     ownerID,
		ApiKey:      "test-key",
		ServiceName: "test-service",
	}
	mockConnection.On("GetClient", ownerID).Return(client, true)

	// Put thread in cache
	mockCache.On("GetThread", threadID).Return(thread, true)
	mockCache.On("SetThread", threadID, thread).Return()

	// Retry the step
	request := &models.RecordEventRequest{
		ThreadID:   threadID,
		StepName:   stepName,
		StartedAt:  "2025-01-15T10:00:00Z",
		FinishedAt: "2025-01-15T10:05:00Z",
		Status:     "completed",
		Context:    map[string]string{"key": "value"},
	}

	response := service.HandleRecordEvent(request, ownerID)

	// Should succeed
	assert.Equal(t, "success", response.Status)

	// Check that retry count was incremented
	mockCache.On("GetThread", threadID).Return(thread, true)
	updatedThread := thread
	assert.Equal(t, 2, updatedThread.Steps[stepName].RetryCount)
	assert.Equal(t, "completed", updatedThread.Steps[stepName].Status)
	assert.True(t, updatedThread.Steps[stepName].IsCompleted)
}
