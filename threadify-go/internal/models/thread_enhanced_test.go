package models

import (
	"testing"
)

func TestThreadWithRefs(t *testing.T) {
	// Test creating thread with refs
	thread := NewThread("thread123", "payment_flow", 1, "user123")

	// Add refs to thread
	thread.Refs = map[string]string{
		"stripeId":   "ch_abc123",
		"customerId": "12345",
		"nhsId":      "NHS-789",
	}

	// Test JSON serialization
	data, err := thread.ToJSON()
	if err != nil {
		t.Fatalf("Failed to marshal thread: %v", err)
	}

	// Test JSON deserialization
	unmarshaled, err := FromJSON(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal thread: %v", err)
	}

	// Verify refs are preserved
	if unmarshaled.Refs == nil {
		t.Error("Expected refs to be preserved, got nil")
	}

	if unmarshaled.Refs["stripeId"] != "ch_abc123" {
		t.Errorf("Expected stripeId 'ch_abc123', got '%s'", unmarshaled.Refs["stripeId"])
	}

	if unmarshaled.Refs["customerId"] != "12345" {
		t.Errorf("Expected customerId '12345', got '%s'", unmarshaled.Refs["customerId"])
	}
}

func TestThreadWithEmptyRefs(t *testing.T) {
	// Test thread with no refs (backward compatibility)
	thread := NewThread("thread456", "", 0, "user456")

	// Should work without refs
	data, err := thread.ToJSON()
	if err != nil {
		t.Fatalf("Failed to marshal thread without refs: %v", err)
	}

	unmarshaled, err := FromJSON(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal thread without refs: %v", err)
	}

	// Refs should be nil when not set
	if unmarshaled.Refs != nil {
		t.Errorf("Expected refs to be nil, got %v", unmarshaled.Refs)
	}
}

func TestThreadWithContractName(t *testing.T) {
	// Test thread with contract name (new field)
	thread := NewThread("thread789", "payment_flow", 1, "user789")

	// Set contract name
	thread.ContractName = "payment_flow_v2"
	thread.Refs = map[string]string{
		"externalId": "ext_123",
	}

	// Test serialization
	data, err := thread.ToJSON()
	if err != nil {
		t.Fatalf("Failed to marshal thread with contract name: %v", err)
	}

	unmarshaled, err := FromJSON(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal thread with contract name: %v", err)
	}

	// Verify contract name
	if unmarshaled.ContractName != "payment_flow_v2" {
		t.Errorf("Expected contractName 'payment_flow_v2', got '%s'", unmarshaled.ContractName)
	}

	// Verify refs still work
	if unmarshaled.Refs["externalId"] != "ext_123" {
		t.Errorf("Expected externalId 'ext_123', got '%s'", unmarshaled.Refs["externalId"])
	}
}

func TestThreadCompleteWithRefs(t *testing.T) {
	// Test completing thread with refs
	thread := NewThread("thread999", "test_contract", 1, "user999")
	thread.Refs = map[string]string{"testRef": "testValue"}

	// Complete thread
	thread.Complete()

	// Verify completion preserves refs
	if !thread.IsCompleted() {
		t.Error("Expected thread to be completed")
	}

	if thread.Refs["testRef"] != "testValue" {
		t.Errorf("Expected refs preserved after completion, got %v", thread.Refs)
	}

	if thread.Status != ThreadStatusCompleted {
		t.Errorf("Expected status 'completed', got '%s'", thread.Status)
	}
}
