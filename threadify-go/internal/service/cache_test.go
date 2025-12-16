package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/models"
)

func TestCacheService_BasicOperations(t *testing.T) {
	cache := NewCacheService()

	// Test thread caching
	thread := &models.Thread{
		ID:              "thread-123",
		ContractID:      stringPtr("contract-1"),
		ContractVersion: intPtr(1),
		OwnerID:         "owner-123",
		Status:          models.ThreadStatusActive,
		CurrentStep:     "step-a",
		Context:         map[string]interface{}{"key": "value"},
		Steps:           make(map[string]*models.StepState),
		StartedAt:       time.Now(),
	}

	// Test SetThread and GetThread
	cache.SetThread("thread-123", thread)
	retrieved, exists := cache.GetThread("thread-123")
	assert.True(t, exists)
	assert.Equal(t, thread.ID, retrieved.ID)
	assert.Equal(t, thread.OwnerID, retrieved.OwnerID)

	// Test non-existent thread
	_, exists = cache.GetThread("non-existent")
	assert.False(t, exists)

	// Test contract graph caching
	graph := &models.ContractGraph{
		Graph: models.Graph{
			Nodes: map[string]models.GraphNode{
				"step-a": {ID: "step-a", Type: "step"},
			},
			FinalStep: "step-a",
		},
	}

	// Test SetContractGraph and GetContractGraph
	cache.SetContractGraph("contract-1", 1, graph)
	retrievedGraph, exists := cache.GetContractGraph("contract-1", 1)
	assert.True(t, exists)
	assert.Equal(t, "step-a", retrievedGraph.Graph.FinalStep)

	// Test non-existent graph
	_, exists = cache.GetContractGraph("non-existent", 1)
	assert.False(t, exists)

	// Test ClearThreadCache
	cache.ClearThreadCache("thread-123")
	_, exists = cache.GetThread("thread-123")
	assert.False(t, exists)

	// Test ClearContractCache
	cache.SetContractGraph("contract-1", 1, graph) // Put it back
	cache.ClearContractCache("contract-1", 1)
	_, exists = cache.GetContractGraph("contract-1", 1)
	assert.False(t, exists)
}

// Helper functions for creating pointers
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
