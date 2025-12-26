package valkey

import (
	"context"
	"testing"
	"time"

	"github.com/threadify/engine/internal/models"
)

func TestThreadRepository_RoleHashOperations(t *testing.T) {
	// Setup test Redis client
	redis := NewMockValkeyService()

	ctx := context.Background()
	threadRepo := NewThreadRepository(redis, 24*3600) // 24 hours TTL
	threadID := "test-thread-roles"

	// Test assigning role to user
	err := threadRepo.AssignRole(ctx, threadID, "payment_processor", "api-key-123")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// Test getting user role
	userRole, err := threadRepo.GetUserRole(ctx, threadID, "api-key-123")
	if err != nil {
		t.Fatalf("Failed to get user role: %v", err)
	}

	if userRole != "payment_processor" {
		t.Errorf("Expected role 'payment_processor', got '%s'", userRole)
	}

	// Test getting all roles
	allRoles, err := threadRepo.GetAllRoles(ctx, threadID)
	if err != nil {
		t.Fatalf("Failed to get all roles: %v", err)
	}

	if allRoles["payment_processor"] != "api-key-123" {
		t.Errorf("Expected payment_processor role assigned to api-key-123, got '%s'", allRoles["payment_processor"])
	}

	// Test removing role
	err = threadRepo.RemoveRole(ctx, threadID, "payment_processor")
	if err != nil {
		t.Fatalf("Failed to remove role: %v", err)
	}

	// Verify role is removed (should be empty string)
	userRoleAfter, err := threadRepo.GetUserRole(ctx, threadID, "api-key-123")
	if err != nil {
		t.Fatalf("Failed to get user role after removal: %v", err)
	}

	if userRoleAfter != "" {
		t.Errorf("Expected empty role after removal, got '%s'", userRoleAfter)
	}
}

func TestThreadRepository_PermissionHashOperations(t *testing.T) {
	// Setup test Redis client
	redis := NewMockValkeyService()

	ctx := context.Background()
	threadRepo := NewThreadRepository(redis, 24*3600)
	threadID := "test-thread-perms"

	// Test setting user permissions
	permissions := []string{"read", "write", "approve"}
	err := threadRepo.SetUserPermissions(ctx, threadID, "api-key-456", permissions)
	if err != nil {
		t.Fatalf("Failed to set user permissions: %v", err)
	}

	// Test getting user permissions
	userPerms, err := threadRepo.GetUserPermissions(ctx, threadID, "api-key-456")
	if err != nil {
		t.Fatalf("Failed to get user permissions: %v", err)
	}

	if len(userPerms) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(userPerms))
	}

	expectedPerms := map[string]bool{"read": true, "write": true, "approve": true}
	for _, perm := range userPerms {
		if !expectedPerms[perm] {
			t.Errorf("Unexpected permission '%s'", perm)
		}
	}

	// Test updating permissions
	newPermissions := []string{"read", "review"}
	err = threadRepo.SetUserPermissions(ctx, threadID, "api-key-456", newPermissions)
	if err != nil {
		t.Fatalf("Failed to update user permissions: %v", err)
	}

	updatedPerms, err := threadRepo.GetUserPermissions(ctx, threadID, "api-key-456")
	if err != nil {
		t.Fatalf("Failed to get updated user permissions: %v", err)
	}

	if len(updatedPerms) != 2 {
		t.Errorf("Expected 2 updated permissions, got %d", len(updatedPerms))
	}
}

func TestThreadRepository_EventQueueOperations(t *testing.T) {
	// Setup test Redis client
	redis := NewMockValkeyService()

	ctx := context.Background()
	threadRepo := NewThreadRepository(redis, 24*3600)
	threadID := "test-thread-events"

	// Test adding thread event
	event := models.ThreadEvent{
		ThreadID:  threadID,
		StepName:  "payment",
		UserID:    "api-key-789",
		Role:      "payment_processor",
		Refs:      map[string]string{"stripe_payment_id": "pi_12345"},
		Context:   map[string]string{"amount": "$49.99"},
		Status:    "success",
		Timestamp: time.Now().UTC(),
	}

	err := threadRepo.AddThreadEvent(ctx, threadID, event)
	if err != nil {
		t.Fatalf("Failed to add thread event: %v", err)
	}

	// Test getting thread events
	events, err := threadRepo.GetThreadEvents(ctx, threadID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to get thread events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	retrievedEvent := events[0]
	if retrievedEvent.StepName != "payment" {
		t.Errorf("Expected step name 'payment', got '%s'", retrievedEvent.StepName)
	}

	if retrievedEvent.UserID != "api-key-789" {
		t.Errorf("Expected user ID 'api-key-789', got '%s'", retrievedEvent.UserID)
	}

	if retrievedEvent.Refs["stripe_payment_id"] != "pi_12345" {
		t.Errorf("Expected stripe_payment_id 'pi_12345', got '%s'", retrievedEvent.Refs["stripe_payment_id"])
	}

	// Test adding multiple events
	event2 := models.ThreadEvent{
		ThreadID:  threadID,
		StepName:  "fraud_check",
		UserID:    "api-key-789",
		Role:      "payment_processor",
		Status:    "in_progress",
		Timestamp: time.Now().UTC(),
	}

	err = threadRepo.AddThreadEvent(ctx, threadID, event2)
	if err != nil {
		t.Fatalf("Failed to add second thread event: %v", err)
	}

	// Test getting multiple events
	allEvents, err := threadRepo.GetThreadEvents(ctx, threadID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to get all thread events: %v", err)
	}

	if len(allEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(allEvents))
	}
}

func TestThreadRepository_PipelineOperations(t *testing.T) {
	// Setup test Redis client
	redis := NewMockValkeyService()

	ctx := context.Background()
	threadRepo := NewThreadRepository(redis, 24*3600)
	threadID := "test-thread-pipeline"

	// Test creating thread with setup using pipeline
	thread := models.NewThread(threadID, "payment_flow", 1, "user123")
	thread.ContractName = "payment_flow"
	thread.Refs = map[string]string{
		"stripeId":   "ch_abc123",
		"customerId": "12345",
	}

	creatorID := "api-key-123"
	creatorRole := "payment_processor"
	creatorPerms := []string{"read", "write", "approve"}

	err := threadRepo.CreateThreadWithSetup(ctx, thread, creatorID, creatorRole, creatorPerms)
	if err != nil {
		t.Fatalf("Failed to create thread with setup: %v", err)
	}

	// Verify thread was created
	retrievedThread, err := threadRepo.Get(ctx, threadID)
	if err != nil {
		t.Fatalf("Failed to get created thread: %v", err)
	}

	if retrievedThread.ContractName != "payment_flow" {
		t.Errorf("Expected contract name 'payment_flow', got '%s'", retrievedThread.ContractName)
	}

	if retrievedThread.Refs["stripeId"] != "ch_abc123" {
		t.Errorf("Expected stripeId 'ch_abc123', got '%s'", retrievedThread.Refs["stripeId"])
	}

	// Verify role was assigned
	userRole, err := threadRepo.GetUserRole(ctx, threadID, creatorID)
	if err != nil {
		t.Fatalf("Failed to get user role: %v", err)
	}

	if userRole != creatorRole {
		t.Errorf("Expected role '%s', got '%s'", creatorRole, userRole)
	}

	// Verify permissions were set
	userPerms, err := threadRepo.GetUserPermissions(ctx, threadID, creatorID)
	if err != nil {
		t.Fatalf("Failed to get user permissions: %v", err)
	}

	if len(userPerms) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(userPerms))
	}

	// Verify event was added
	events, err := threadRepo.GetThreadEvents(ctx, threadID, 0, 10)
	if err != nil {
		t.Fatalf("Failed to get thread events: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 creation event, got %d", len(events))
	}

	if events[0].Action != "thread_created" {
		t.Errorf("Expected action 'thread_created', got '%s'", events[0].Action)
	}
}
