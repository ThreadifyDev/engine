package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
)

// AuditQueueConfig represents configuration for audit queue
type AuditQueueConfig struct {
	Name           string `mapstructure:"name"`
	RetentionHours int    `mapstructure:"retention_hours"`
	BatchSize      int    `mapstructure:"batch_size"`
	BatchTimeoutMs int    `mapstructure:"batch_timeout_ms"`
	MaxRetries     int    `mapstructure:"max_retries"`
}

// IsValid validates the audit configuration
func (c *AuditQueueConfig) IsValid() bool {
	return c.Name != "" &&
		c.RetentionHours > 0 &&
		c.BatchSize > 0 &&
		c.BatchTimeoutMs > 0 &&
		c.MaxRetries >= 0
}

// GetRetentionDuration returns retention as time.Duration
func (c *AuditQueueConfig) GetRetentionDuration() time.Duration {
	return time.Duration(c.RetentionHours) * time.Hour
}

// GetBatchTimeout returns batch timeout as time.Duration
func (c *AuditQueueConfig) GetBatchTimeout() time.Duration {
	return time.Duration(c.BatchTimeoutMs) * time.Millisecond
}

// AuditEventType represents different types of audit events
type AuditEventType string

const (
	TokenCreated AuditEventType = "token_created"
	TokenUsed    AuditEventType = "token_used"
	ThreadJoined AuditEventType = "thread_joined"
)

// AuditEvent represents an audit event
type AuditEvent struct {
	ID         string                 `json:"id"`
	Type       AuditEventType         `json:"type"`
	ThreadID   string                 `json:"threadId"`
	ContractID string                 `json:"contractId"`
	UserID     string                 `json:"userId"`
	Data       map[string]interface{} `json:"data"`
	Timestamp  time.Time              `json:"timestamp"`
	Metadata   map[string]string      `json:"metadata"`
}

// AuditEventService handles audit event logging
type AuditEventService struct {
	valkeyService *database.ValkeyService
	config        *AuditQueueConfig
	queue         string
}

// NewAuditEventService creates a new audit event service
func NewAuditEventService(valkeyService *database.ValkeyService, config *AuditQueueConfig) *AuditEventService {
	if !config.IsValid() {
		panic("invalid audit queue configuration")
	}

	return &AuditEventService{
		valkeyService: valkeyService,
		config:        config,
		queue:         config.Name,
	}
}

// LogTokenCreated logs when an invitation token is created
func (s *AuditEventService) LogTokenCreated(ctx context.Context, threadID, contractID, userID, role, permissions string) error {
	event := &AuditEvent{
		ID:         uuid.New().String(),
		Type:       TokenCreated,
		ThreadID:   threadID,
		ContractID: contractID,
		UserID:     userID,
		Data: map[string]interface{}{
			"role":        role,
			"permissions": permissions,
		},
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"source": "invite_party",
		},
	}

	return s.enqueueEvent(ctx, event)
}

// LogTokenUsed logs when an invitation token is used
func (s *AuditEventService) LogTokenUsed(ctx context.Context, tokenExpiry time.Time, threadID, userID string) error {
	event := &AuditEvent{
		ID:       uuid.New().String(),
		Type:     TokenUsed,
		ThreadID: threadID,
		UserID:   userID,
		Data: map[string]interface{}{
			"tokenExpiry": tokenExpiry.Unix(),
		},
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"source": "join_thread",
		},
	}

	return s.enqueueEvent(ctx, event)
}

// LogThreadJoined logs when a user joins a thread
func (s *AuditEventService) LogThreadJoined(ctx context.Context, threadID, contractID, userID string) error {
	event := &AuditEvent{
		ID:         uuid.New().String(),
		Type:       ThreadJoined,
		ThreadID:   threadID,
		ContractID: contractID,
		UserID:     userID,
		Data:       map[string]interface{}{},
		Timestamp:  time.Now(),
		Metadata: map[string]string{
			"source": "thread_join",
		},
	}

	return s.enqueueEvent(ctx, event)
}

// enqueueEvent adds an audit event to the queue
func (s *AuditEventService) enqueueEvent(ctx context.Context, event *AuditEvent) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	// Add to Redis queue with TTL for retention
	key := fmt.Sprintf("queue:%s", s.queue)
	err = s.valkeyService.Enqueue(ctx, key, string(eventJSON), s.config.GetRetentionDuration())
	if err != nil {
		return fmt.Errorf("failed to enqueue audit event: %w", err)
	}

	return nil
}
