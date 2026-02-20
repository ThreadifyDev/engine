package tests

import (
	"context"
	"testing"
)

func TestLuaScriptManager_LoadScripts(t *testing.T) {
	mockClient := NewMockValkeyClient()
	manager := NewLuaScriptManager(mockClient)

	err := manager.LoadScripts(context.Background())
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Verify script hash is stored
	hash, exists := manager.GetScriptHash("update_thread_status")
	if !exists {
		t.Fatal("Expected script hash to exist")
	}
	if hash != "mock-sha-hash" {
		t.Fatalf("Expected hash 'mock-sha-hash', got '%s'", hash)
	}
}

func TestLuaScriptManager_UpdateThreadStatus(t *testing.T) {
	mockClient := NewMockValkeyClient()
	manager := NewLuaScriptManager(mockClient)

	// Load scripts first
	_ = manager.LoadScripts(context.Background())

	status, err := manager.UpdateThreadStatus(
		context.Background(),
		"thread-123",
		"step-456",
		"failed",
		"failed",
		`{"violation":"data"}`,
		"2025-12-30T12:00:00Z",
	)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if status != "completed" {
		t.Fatalf("Expected status 'completed', got '%s'", status)
	}
}
