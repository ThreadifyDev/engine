package service

import (
	"encoding/json"
	"fmt"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/utils"

	"go.uber.org/zap"
)

// queueLegacyUserMigration queues a migration event for a legacy user
// source: "login" or "forgot_password" - determines which email to send after migration
func (s *AuthService) queueLegacyUserMigration(userID, email, password, source string) error {
	email = normalizeEmail(email)

	inflight, err := s.outboxRepo.ExistsPendingByReference(models.EventTypeMigrateLegacyUser, userID)
	if err != nil {
		return fmt.Errorf("check inflight migration: %w", err)
	}
	if inflight {
		s.logger.Debug("migration already queued for user", zap.String("user_id", userID))
		return nil
	}

	// Payload contains email, user_id, password, and source
	payload, err := json.Marshal(map[string]string{
		"email":    email,
		"user_id":  userID,
		"password": password,
		"source":   source, // "login" or "forgot_password"
	})
	if err != nil {
		return fmt.Errorf("marshal migration event payload: %w", err)
	}

	encrypted, err := utils.Encrypt(payload, s.encryptionKey)
	if err != nil {
		for i := range payload {
			payload[i] = 0
		}
		return fmt.Errorf("encrypt migration event payload: %w", err)
	}

	for i := range payload {
		payload[i] = 0
	}
	if err := s.outboxRepo.Create(&models.OutboxEvent{
		ID:          utils.GenerateID(),
		Type:        models.EventTypeMigrateLegacyUser,
		Payload:     encrypted,
		Status:      models.OutboxStatusPending,
		MaxRetries:  models.OutboxDefaultMaxRetries,
		NextRunAt:   time.Now(),
		ReferenceID: userID,
	}); err != nil {
		return fmt.Errorf("create migration event: %w", err)
	}

	if s.outboxWorker != nil {
		s.outboxWorker.Trigger()
	}

	s.logger.Info("legacy user migration queued via outbox")

	return nil
}
