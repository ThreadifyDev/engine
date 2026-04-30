package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/threadify/engine/internal/domain"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

// Repository-level sentinel errors.
var errNATSPublisherNotAvailable = errors.New("NATS publisher not available")

// ActivityRepository handles stream and event operations in Valkey
type ActivityRepository struct {
	natsPublisher *natsrepo.ArchivalPublisher
	postgresRepo  PostgresActivityRepository // For hot/cold fallback
	logger        *zap.Logger
}

// PostgresActivityRepository defines the interface for PostgreSQL activity operations
type PostgresActivityRepository interface {
	GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error)
}

// NewActivityRepository creates a new activity repository
func NewActivityRepository(natsPublisher *natsrepo.ArchivalPublisher, logger *zap.Logger) *ActivityRepository {
	return &ActivityRepository{
		natsPublisher: natsPublisher,
		logger:        logger,
	}
}

// NewActivityRepositoryWithPostgres creates a new activity repository with PostgreSQL fallback
func NewActivityRepositoryWithPostgres(natsPublisher *natsrepo.ArchivalPublisher, postgresRepo PostgresActivityRepository, logger *zap.Logger) *ActivityRepository {
	return &ActivityRepository{
		natsPublisher: natsPublisher,
		postgresRepo:  postgresRepo,
		logger:        logger,
	}
}

// publishWithTimeout wraps a NATS publish call with a 5s timeout context.
// The caller must not defer cancel — this helper manages it internally.
func (r *ActivityRepository) publishWithTimeout(ctx context.Context, publish func(context.Context) error) error {
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return publish(pubCtx)
}

// RecordAccessGranted records an access granted event to streams
func (r *ActivityRepository) RecordAccessGranted(ctx context.Context, threadID, userID string, access *domain.UserAccess, invitedBy, serviceName, runtimeRole string) error {
	if r.natsPublisher == nil {
		return nil
	}

	// Determine event type based on context
	eventType := "access_granted"
	if invitedBy != "self" && len(access.Roles) > 1 {
		eventType = "role_added"
	}

	rolesJSON, err := json.Marshal(access.Roles)
	if err != nil {
		return fmt.Errorf("marshal roles: %w", err)
	}
	permissionsJSON, err := json.Marshal(access.Permissions)
	if err != nil {
		return fmt.Errorf("marshal permissions: %w", err)
	}

	// SYNCHRONOUS - Critical for access control persistence
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishThreadAccess(pubCtx, map[string]interface{}{
			"threadId":     threadID,
			"userId":       userID,
			"roles":        string(rolesJSON),
			"runtime_role": runtimeRole,
			"permissions":  string(permissionsJSON),
			"grantedBy":    invitedBy,
			"grantedAt":    access.GrantedAt,
			"status":       access.Status,
			"eventType":    eventType,
		})
	}); err != nil {
		r.logger.Error("failed to publish thread access to NATS",
			zap.String("thread_id", threadID),
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}

	// SYNCHRONOUS - Critical for audit trail
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":         "access_granted",
			"actor":        userID,
			"actorService": serviceName,
			"role":         strings.Join(access.Roles, ","),
			"runtimeRole":  runtimeRole,
			"method":       "direct",
		})
	}); err != nil {
		r.logger.Error("failed to publish access granted activity to NATS",
			zap.String("thread_id", threadID),
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}

	return nil
}

// RecordInvitationUsed records an invitation used event to streams
func (r *ActivityRepository) RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error {
	if r.natsPublisher == nil {
		return nil
	}

	// SYNCHRONOUS - Critical for audit trail
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":      "invitation_used",
			"role":      role,
			"invitedBy": invitedBy,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}); err != nil {
		r.logger.Error("failed to publish invitation used to NATS",
			zap.String("thread_id", threadID),
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}

	return nil
}

// RecordThreadCreated records a thread created event to streams
func (r *ActivityRepository) RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error {
	if r.natsPublisher == nil {
		return nil
	}

	now := time.Now().Format(time.RFC3339)

	// SYNCHRONOUS - Critical for PostgreSQL persistence
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishThreadMetadata(pubCtx, map[string]interface{}{
			"threadId":    threadID,
			"creatorId":   creatorID,
			"creatorRole": creatorRole,
			"createdAt":   now,
			"status":      "active",
		})
	}); err != nil {
		r.logger.Error("failed to publish thread metadata to NATS",
			zap.String("thread_id", threadID),
			zap.String("creator_id", creatorID),
			zap.Error(err),
		)
	}

	// SYNCHRONOUS - Critical for audit trail
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":      "thread_created",
			"userId":    creatorID,
			"actor":     creatorID,
			"role":      creatorRole,
			"timestamp": now,
		})
	}); err != nil {
		r.logger.Error("failed to publish thread created activity to NATS",
			zap.String("thread_id", threadID),
			zap.String("creator_id", creatorID),
			zap.Error(err),
		)
	}

	return nil
}

// ArchiveValidationResults writes validation notifications to archival stream
func (r *ActivityRepository) ArchiveValidationResults(
	ctx context.Context,
	threadID string,
	stepID string,
	stepName string,
	idempotencyKey string,
	notifications []domain.ValidationNotification,
	finalStatus string,
	hasCriticalViolation bool,
) error {
	if r.natsPublisher == nil || len(notifications) == 0 {
		return nil
	}

	mappedNotifs := make([]map[string]interface{}, len(notifications))
	for i, n := range notifications {
		mappedNotifs[i] = map[string]interface{}{
			"notificationId":   n.NotificationID,
			"threadId":         n.ThreadID,
			"stepId":           n.StepID,
			"stepName":         n.StepName,
			"ownerId":          n.OwnerID,
			"contractName":     n.ContractName,
			"source":           string(n.Source),
			"notificationType": string(n.NotificationType),
			"stepStatus":       n.StepStatus,
			"status":           n.Status,
			"violationType":    n.ViolationType,
			"severity":         n.Severity,
			"message":          n.Message,
			"details":          n.Details,
			"timestamp":        n.Timestamp,
		}
	}

	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishThreadNotifications(pubCtx, map[string]interface{}{
			"threadId":      threadID,
			"stepId":        stepID,
			"stepName":      stepName,
			"notifications": mappedNotifs,
		})
	}); err != nil {
		r.logger.Error("failed to publish thread notifications to NATS",
			zap.String("thread_id", threadID),
			zap.String("step_id", stepID),
			zap.Error(err),
		)
		return fmt.Errorf("failed to archive notifications: %w", err)
	}

	return nil
}

// ArchiveThreadMetadata writes thread metadata to archival stream
func (r *ActivityRepository) ArchiveThreadMetadata(ctx context.Context, thread *domain.Thread, status string) error {
	contractVersion := "0"
	if thread.ContractVersion != nil {
		contractVersion = fmt.Sprintf("%d", *thread.ContractVersion)
	}

	contractID := ""
	if thread.ContractID != nil {
		contractID = *thread.ContractID
	}

	completedAt := ""
	if thread.CompletedAt != nil {
		completedAt = thread.CompletedAt.Format(time.RFC3339)
	} else if status == "completed" {
		completedAt = time.Now().Format(time.RFC3339)
	}

	if r.natsPublisher == nil {
		return nil
	}

	// SYNCHRONOUS - Critical for PostgreSQL persistence
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishThreadMetadata(pubCtx, map[string]interface{}{
			"threadId":        thread.ID,
			"ownerId":         thread.OwnerID,
			"companyId":       thread.CompanyID,
			"contractId":      contractID,
			"contractVersion": contractVersion,
			"contractName":    thread.ContractName,
			"status":          status,
			"lastHash":        thread.LastHash,
			"startedAt":       thread.StartedAt.Format(time.RFC3339),
			"completedAt":     completedAt,
		})
	}); err != nil {
		r.logger.Error("failed to publish thread metadata to NATS",
			zap.String("thread_id", thread.ID),
			zap.String("status", status),
			zap.Error(err),
		)
	}

	if status != "completed" {
		return nil
	}

	// SYNCHRONOUS - Critical for audit trail
	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":         "thread_completed",
			"threadId":     thread.ID,
			"actor":        thread.OwnerID,
			"actorService": "system",
			"finalStatus":  status,
			"lastHash":     thread.LastHash,
			"completedAt":  completedAt,
			"timestamp":    time.Now().Format(time.RFC3339),
		})
	}); err != nil {
		r.logger.Error("failed to publish thread completed activity to NATS",
			zap.String("thread_id", thread.ID),
			zap.Error(err),
		)
	}

	return nil
}

// ArchiveStepState publishes step state snapshot to NATS for archival to Postgres.
// SYNCHRONOUS - Critical for PostgreSQL persistence (DB-first architecture).
func (r *ActivityRepository) ArchiveStepState(ctx context.Context, stepState *domain.StepStateSnapshot) error {
	if r.natsPublisher == nil {
		return errNATSPublisherNotAvailable
	}

	if err := r.publishWithTimeout(ctx, func(pubCtx context.Context) error {
		return r.natsPublisher.PublishStepState(pubCtx, map[string]interface{}{
			"stepId":         stepState.ID,
			"threadId":       stepState.ThreadID,
			"stepName":       stepState.StepName,
			"idempotencyKey": stepState.IdempotencyKey,
			"status":         stepState.Status,
			"retryCount":     stepState.RetryCount,
			"firstSeenAt":    stepState.FirstSeenAt,
			"lastUpdatedAt":  stepState.LastUpdatedAt,
			"startedAt":      stepState.StartedAt,
			"finishedAt":     stepState.FinishedAt,
			"previousStep":   stepState.PreviousStep,
			"actor":          stepState.Actor,
			"actorService":   stepState.ActorService,
			"latestContext":  stepState.LatestContext,
		})
	}); err != nil {
		return fmt.Errorf("failed to archive step state to NATS: %w", err)
	}

	r.logger.Info("step state archived",
		zap.String("step_name", stepState.StepName),
		zap.String("thread_id", stepState.ThreadID),
	)
	return nil
}

// GetActivityLog retrieves activity log from PostgreSQL.
// Activities are NEVER cached in Valkey — they go directly: NATS → PostgreSQL.
func (r *ActivityRepository) GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error) {
	if r.postgresRepo == nil {
		return []map[string]interface{}{}, nil
	}
	return r.postgresRepo.GetActivityLog(ctx, threadID)
}
