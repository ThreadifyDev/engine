package service

import (
	"testing"

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

type MockContractGraphRepository struct {
	mock.Mock
}

func (m *MockContractGraphRepository) Save(ctx interface{}, graph *models.ContractGraph) error {
	args := m.Called(ctx, graph)
	return args.Error(0)
}

func (m *MockContractGraphRepository) Get(ctx interface{}, contractID string, version int) (*models.ContractGraph, error) {
	args := m.Called(ctx, contractID, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ContractGraph), args.Error(1)
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
