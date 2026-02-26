package models

import "time"

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusFailed     = "failed"
	OutboxStatusDead       = "dead"

	EventTypeRegisterAuthUser      = "register_auth_user"
	EventTypeSendVerificationEmail = "send_verification_email"
)

type OutboxEvent struct {
	ID          string
	Type        string
	Payload     []byte
	Status      string
	RetryCount  int
	MaxRetries  int
	LastError   *string
	NextRunAt   time.Time
	ReferenceID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
