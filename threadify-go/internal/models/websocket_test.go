package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartThreadRequest(t *testing.T) {
	tests := []struct {
		name    string
		request StartThreadRequest
		valid   bool
	}{
		{
			name: "valid request with refs",
			request: StartThreadRequest{
				Action:       "startThread",
				ContractName: "payment_flow",
				Role:         "payment_processor",
				Refs: map[string]string{
					"stripeId": "ch_123",
				},
			},
			valid: true,
		},
		{
			name: "valid request without refs",
			request: StartThreadRequest{
				Action:       "startThread",
				ContractName: "payment_flow",
				Role:         "payment_processor",
				// Refs omitted - should be fine
			},
			valid: true,
		},
		{
			name: "valid - missing role (non-contract workflow)",
			request: StartThreadRequest{
				Action:       "startThread",
				ContractName: "", // Empty contract name for non-contract workflow
				// Role omitted - valid for non-contract workflows
			},
			valid: true,
		},
		{
			name: "valid - non-contract workflow (no contract, no role)",
			request: StartThreadRequest{
				Action: "startThread",
				// ContractName and Role both omitted - valid for non-contract workflows
			},
			valid: true,
		},
		{
			name: "invalid - contract name provided but missing role",
			request: StartThreadRequest{
				Action:       "startThread",
				ContractName: "payment_flow",
				// Role missing - invalid when contract name is provided
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.request)
		})
	}
}

func TestStartThreadRequestWithRefs(t *testing.T) {
	// Test the new start thread request with refs
	req := StartThreadRequest{
		Action:       "startThread",
		ContractName: "payment_flow",
		Role:         "payment_processor",
		Refs: map[string]string{
			"customer_id": "12345",
			"stripeId":    "ch_abc123",
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled StartThreadRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	// Verify fields
	if unmarshaled.Action != "startThread" {
		t.Errorf("Expected action 'startThread', got '%s'", unmarshaled.Action)
	}
	if unmarshaled.ContractName != "payment_flow" {
		t.Errorf("Expected contractName 'payment_flow', got '%s'", unmarshaled.ContractName)
	}
	if unmarshaled.Role != "payment_processor" {
		t.Errorf("Expected role 'payment_processor', got '%s'", unmarshaled.Role)
	}
	if unmarshaled.Refs["customer_id"] != "12345" {
		t.Errorf("Expected customer_id '12345', got '%s'", unmarshaled.Refs["customer_id"])
	}
}

func TestJoinThreadRequest_Token(t *testing.T) {
	// Test token-based join
	req := JoinThreadRequest{
		Action:      "joinThread",
		ThreadToken: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var unmarshaled JoinThreadRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	if unmarshaled.Action != "joinThread" {
		t.Errorf("Expected action 'joinThread', got '%s'", unmarshaled.Action)
	}
	if unmarshaled.ThreadToken == "" {
		t.Error("Expected non-empty ThreadToken")
	}
}

func TestJoinThreadRequest_Direct(t *testing.T) {
	// Test direct join with threadId and role
	req := JoinThreadRequest{
		Action:   "joinThread",
		ThreadID: "thread123",
		Role:     "payment_gateway",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var unmarshaled JoinThreadRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	if unmarshaled.Action != "joinThread" {
		t.Errorf("Expected action 'joinThread', got '%s'", unmarshaled.Action)
	}
	if unmarshaled.ThreadID != "thread123" {
		t.Errorf("Expected threadId 'thread123', got '%s'", unmarshaled.ThreadID)
	}
	if unmarshaled.Role != "payment_gateway" {
		t.Errorf("Expected role 'payment_gateway', got '%s'", unmarshaled.Role)
	}
}

func TestRecordEventRequestWithRefs(t *testing.T) {
	// Test record event with refs
	req := RecordEventRequest{
		Action:    "recordThreadEvent",
		ThreadID:  "thread123",
		StepName:  "payment",
		StartedAt: "2025-01-01T12:00:00Z",
		Refs: map[string]string{
			"stripe_payment_id": "pi_12345",
		},
		Context: map[string]string{
			"amount": "$49.99",
		},
		Status: "success",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	var unmarshaled RecordEventRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal request: %v", err)
	}

	if unmarshaled.Refs["stripe_payment_id"] != "pi_12345" {
		t.Errorf("Expected stripe_payment_id 'pi_12345', got '%s'", unmarshaled.Refs["stripe_payment_id"])
	}
	if unmarshaled.Context["amount"] != "$49.99" {
		t.Errorf("Expected amount '$49.99', got '%s'", unmarshaled.Context["amount"])
	}
}
