package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/models"
)

func TestCacheService_BasicOperations(t *testing.T) {
	cache := NewCacheService()

	// Test thread caching
	contractID := "contract-1"
	contractVersion := 1
	thread := &models.Thread{
		ID:              "thread-123",
		ContractID:      &contractID,
		ContractVersion: &contractVersion,
		OwnerID:         "owner-123",
		CompanyID:       "company-123",
		Status:          models.ThreadStatusActive,
		CurrentSteps:    []string{"step-a"},
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
			TerminalSteps: []string{"step-a"},
		},
	}

	// Test SetContractGraph and GetContractGraph
	cache.SetContractGraph("contract-1", 1, graph)
	retrievedGraph, exists := cache.GetContractGraph("contract-1", 1)
	assert.True(t, exists)
	assert.Equal(t, []string{"step-a"}, retrievedGraph.Graph.TerminalSteps)

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
