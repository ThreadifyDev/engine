package models

import "time"

const (
	OutboxStatusPending    = "pending"
	OutboxStatusProcessing = "processing"
	OutboxStatusDone       = "done"
	OutboxStatusFailed     = "failed"
	OutboxStatusDead       = "dead"

	OutboxDefaultMaxRetries = 5

	EventTypeRegisterAuthUser       = "register_auth_user"
	EventTypeSendVerificationEmail  = "send_verification_email"
	EventTypeSendPasswordResetEmail = "send_password_reset_email"
	EventTypeMigrateLegacyUser      = "migrate_legacy_user"
	EventTypeSendTeamInvitation     = "send_team_invitation"

	MigrationSourceLogin          = "login"
	MigrationSourceForgotPassword = "forgot_password"
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
