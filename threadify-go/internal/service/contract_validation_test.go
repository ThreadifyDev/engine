package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/threadify/engine/internal/models"
)

func TestContractValidationService_ValidateStepInContract(t *testing.T) {
	// Setup mocks
	mockGraphRepo := &MockContractGraphRepository{}
	mockCache := &MockCacheManager{}

	validator := NewContractValidationService(mockGraphRepo, nil, mockCache)

	// Create a test contract graph
	testGraph := &models.ContractGraph{
		Graph: models.Graph{
			Nodes: map[string]models.GraphNode{
				"step-a": {
					ID:   "step-a",
					Type: "step",
					BusinessContext: map[string]string{
						"required_field": "string",
					},
				},
			},
			FinalStep: "step-a",
		},
	}

	// Setup mock expectations
	mockCache.On("GetContractGraph", "test-contract", 1).Return((*models.ContractGraph)(nil), false)
	mockGraphRepo.On("Get", mock.Anything, "test-contract", 1).Return(testGraph, nil)
	mockCache.On("SetContractGraph", "test-contract", 1, testGraph).Return()

	// Test valid step validation
	err := validator.ValidateStepInContract("test-contract", 1, "step-a", map[string]string{
		"required_field": "test_value",
	})
	assert.NoError(t, err)

	// Test step not found
	err = validator.ValidateStepInContract("test-contract", 1, "non-existent-step", map[string]string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in contract")

	// Test missing required context - should pass since field is optional
	err = validator.ValidateStepInContract("test-contract", 1, "step-a", map[string]string{})
	assert.NoError(t, err) // No error because context validation is optional

	// Test empty required context field
	err = validator.ValidateStepInContract("test-contract", 1, "step-a", map[string]string{
		"required_field": "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a non-empty string")

	// Verify all mocks were called
	mock.AssertExpectationsForObjects(t, mockGraphRepo, mockCache)
}

func TestContractValidationService_GetContractGraph(t *testing.T) {
	// Setup mocks
	mockGraphRepo := &MockContractGraphRepository{}
	mockCache := &MockCacheManager{}

	validator := NewContractValidationService(mockGraphRepo, nil, mockCache)

	testGraph := &models.ContractGraph{
		Graph: models.Graph{
			Nodes: map[string]models.GraphNode{
				"step-a": {ID: "step-a", Type: "step"},
			},
			FinalStep: "step-a",
		},
	}

	// Test cache hit
	mockCache.On("GetContractGraph", "test-contract", 1).Return(testGraph, true)

	graph, err := validator.GetContractGraph("test-contract", 1)
	assert.NoError(t, err)
	assert.Equal(t, "step-a", graph.Graph.FinalStep)

	// Test cache miss
	mockCache.On("GetContractGraph", "test-contract", 2).Return((*models.ContractGraph)(nil), false)
	mockGraphRepo.On("Get", mock.Anything, "test-contract", 2).Return(testGraph, nil)
	mockCache.On("SetContractGraph", "test-contract", 2, testGraph).Return()

	graph, err = validator.GetContractGraph("test-contract", 2)
	assert.NoError(t, err)
	assert.Equal(t, "step-a", graph.Graph.FinalStep)

	// Verify all mocks were called
	mock.AssertExpectationsForObjects(t, mockGraphRepo, mockCache)
}

func TestContractValidationService_LoadContractGraphIntoCache(t *testing.T) {
	// Setup mocks
	mockGraphRepo := &MockContractGraphRepository{}
	mockCache := &MockCacheManager{}

	validator := NewContractValidationService(mockGraphRepo, nil, mockCache)

	testGraph := &models.ContractGraph{
		Graph: models.Graph{
			Nodes: map[string]models.GraphNode{
				"step-a": {ID: "step-a", Type: "step"},
			},
			FinalStep: "step-a",
		},
	}

	// Test loading new contract
	mockCache.On("GetContractGraph", "test-contract", 1).Return((*models.ContractGraph)(nil), false)
	mockGraphRepo.On("Get", mock.Anything, "test-contract", 1).Return(testGraph, nil)
	mockCache.On("SetContractGraph", "test-contract", 1, testGraph).Return()

	err := validator.LoadContractGraphIntoCache("test-contract", 1)
	assert.NoError(t, err)

	// Test already cached (should not call repository)
	mockCache.On("GetContractGraph", "test-contract", 2).Return(testGraph, true)

	err = validator.LoadContractGraphIntoCache("test-contract", 2)
	assert.NoError(t, err)

	// Verify all mocks were called
	mock.AssertExpectationsForObjects(t, mockGraphRepo, mockCache)
}
