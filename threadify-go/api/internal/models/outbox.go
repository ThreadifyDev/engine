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
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Payload    []byte    `json:"-"` // Encrypted JSON
	Status     string    `json:"status"`
	RetryCount int       `json:"retry_count"`
	MaxRetries int       `json:"max_retries"`
	NextRunAt  time.Time `json:"next_run_at"`
	ErrorLog   *string   `json:"error_log,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
