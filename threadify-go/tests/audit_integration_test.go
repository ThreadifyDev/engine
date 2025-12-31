package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
)

func TestAuditEventService_Integration(t *testing.T) {
	// Skip if running in CI without Redis
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test ValkeyService
	valkeyService, err := database.NewValkeyService("localhost", 6379, "threadify_secure_password", 0)
	require.NoError(t, err)
	defer valkeyService.Close()

	// Clean up any existing test queue
	ctx := context.Background()
	queueKey := "queue:test_audit_events"
	valkeyService.Client.Del(ctx, queueKey)

	// Create audit config
	config := &AuditQueueConfig{
		Name:           "test_audit_events",
		RetentionHours: 12,
		BatchSize:      50,
		BatchTimeoutMs: 5000,
		MaxRetries:     3,
	}

	// Create audit service
	auditService := NewAuditEventService(valkeyService, config)

	// Test logging token created event
	err = auditService.LogTokenCreated(ctx, "thread-123", "contract-456", "user-789", "external_partner", "read,write")
	assert.NoError(t, err)

	// Test logging token used event
	err = auditService.LogTokenUsed(ctx, time.Now().Add(24*time.Hour), "thread-123", "user-789")
	assert.NoError(t, err)

	// Test logging thread joined event
	err = auditService.LogThreadJoined(ctx, "thread-123", "contract-456", "user-789", "merchant", "read,write")
	assert.NoError(t, err)

	// Verify events are in the queue
	events, err := valkeyService.Client.LRange(ctx, queueKey, 0, -1).Result()
	require.NoError(t, err)
	assert.Len(t, events, 3, "Should have 3 audit events in queue")

	// Clean up test queue
	valkeyService.Client.Del(ctx, queueKey)
}

func TestAuditEventService_ConfigurationValidation(t *testing.T) {
	valkeyService, err := database.NewValkeyService("localhost", 6379, "threadify_secure_password", 0)
	require.NoError(t, err)
	defer valkeyService.Close()

	// Test invalid config - empty name
	invalidConfig1 := &AuditQueueConfig{
		Name:           "",
		RetentionHours: 12,
		BatchSize:      50,
		BatchTimeoutMs: 5000,
		MaxRetries:     3,
	}

	assert.Panics(t, func() {
		NewAuditEventService(valkeyService, invalidConfig1)
	}, "Should panic with empty queue name")

	// Test invalid config - negative retention
	invalidConfig2 := &AuditQueueConfig{
		Name:           "test_queue",
		RetentionHours: -1,
		BatchSize:      50,
		BatchTimeoutMs: 5000,
		MaxRetries:     3,
	}

	assert.Panics(t, func() {
		NewAuditEventService(valkeyService, invalidConfig2)
	}, "Should panic with negative retention")
}
