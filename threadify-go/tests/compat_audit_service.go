package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/database"
)

type AuditQueueConfig struct {
	Name           string `mapstructure:"name" yaml:"name"`
	RetentionHours int    `mapstructure:"retention_hours" yaml:"retention_hours"`
	BatchSize      int    `mapstructure:"batch_size" yaml:"batch_size"`
	BatchTimeoutMs int    `mapstructure:"batch_timeout_ms" yaml:"batch_timeout_ms"`
	MaxRetries     int    `mapstructure:"max_retries" yaml:"max_retries"`
}

func (c AuditQueueConfig) IsValid() bool {
	return c.Name != "" &&
		c.RetentionHours > 0 &&
		c.BatchSize > 0 &&
		c.BatchTimeoutMs > 0 &&
		c.MaxRetries >= 0
}

type AuditEventService struct {
	valkey  *database.ValkeyService
	queue   string
	expires time.Duration
}

func NewAuditEventService(valkey *database.ValkeyService, cfg *AuditQueueConfig) *AuditEventService {
	if cfg == nil || !cfg.IsValid() {
		panic("invalid audit queue configuration")
	}

	return &AuditEventService{
		valkey:  valkey,
		queue:   "queue:" + cfg.Name,
		expires: time.Duration(cfg.RetentionHours) * time.Hour,
	}
}

func (s *AuditEventService) LogTokenCreated(ctx context.Context, threadID, contractID, userID, role, accessLevel string) error {
	return s.enqueue(ctx, map[string]interface{}{
		"type":        "token_created",
		"threadId":    threadID,
		"contractId":  contractID,
		"userId":      userID,
		"role":        role,
		"accessLevel": accessLevel,
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *AuditEventService) LogTokenUsed(ctx context.Context, expiresAt time.Time, threadID, userID string) error {
	return s.enqueue(ctx, map[string]interface{}{
		"type":      "token_used",
		"threadId":  threadID,
		"userId":    userID,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339Nano),
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *AuditEventService) LogThreadJoined(ctx context.Context, threadID, contractID, userID, role, accessLevel string) error {
	return s.enqueue(ctx, map[string]interface{}{
		"type":        "thread_joined",
		"threadId":    threadID,
		"contractId":  contractID,
		"userId":      userID,
		"role":        role,
		"accessLevel": accessLevel,
		"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *AuditEventService) enqueue(ctx context.Context, payload map[string]interface{}) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	if err := s.valkey.Client.LPush(ctx, s.queue, string(encoded)).Err(); err != nil {
		return fmt.Errorf("enqueue audit event: %w", err)
	}
	if s.expires > 0 {
		_ = s.valkey.Client.Expire(ctx, s.queue, s.expires).Err()
	}
	return nil
}
