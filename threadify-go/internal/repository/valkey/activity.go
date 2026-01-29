package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
)

// ActivityRepository handles stream and event operations in Valkey
type ActivityRepository struct {
	valkey        interfaces.ValkeyClient
	natsPublisher *natsrepo.ArchivalPublisher
	postgresRepo  PostgresActivityRepository // For hot/cold fallback
}

// PostgresActivityRepository defines the interface for PostgreSQL activity operations
type PostgresActivityRepository interface {
	GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error)
}

// NewActivityRepository creates a new activity repository
func NewActivityRepository(valkey interfaces.ValkeyClient, natsPublisher *natsrepo.ArchivalPublisher) *ActivityRepository {
	return &ActivityRepository{
		valkey:        valkey,
		natsPublisher: natsPublisher,
	}
}

// NewActivityRepositoryWithPostgres creates a new activity repository with PostgreSQL fallback
func NewActivityRepositoryWithPostgres(valkey interfaces.ValkeyClient, natsPublisher *natsrepo.ArchivalPublisher, postgresRepo PostgresActivityRepository) *ActivityRepository {
	return &ActivityRepository{
		valkey:        valkey,
		natsPublisher: natsPublisher,
		postgresRepo:  postgresRepo,
	}
}

// RecordAccessGranted records an access granted event to streams
func (r *ActivityRepository) RecordAccessGranted(ctx context.Context, threadID, userID string, access *interfaces.UserAccess, invitedBy, serviceName, runtimeRole string) error {
	// Determine event type based on context
	eventType := "access_granted"
	if invitedBy != "self" && len(access.Roles) > 1 {
		eventType = "role_added"
	}

	// Publish to NATS for archival
	rolesJSON, _ := json.Marshal(access.Roles)
	permissionsJSON, _ := json.Marshal(access.Permissions)
	// SYNCHRONOUS - Critical for access control persistence
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishThreadAccess(pubCtx, map[string]interface{}{
			"threadId":     threadID,
			"userId":       userID,
			"roles":        string(rolesJSON),
			"runtime_role": runtimeRole,
			"permissions":  string(permissionsJSON),
			"grantedBy":    invitedBy,
			"grantedAt":    access.GrantedAt,
			"status":       access.Status,
			"event_type":   eventType,
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread access to NATS: %v\n", err)
		}
	}

	// Publish activity log to NATS (SYNCHRONOUS - critical for audit trail)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":          "access_granted",
			"thread_id":     threadID,
			"user_id":       userID,
			"actor":         userID,      // user-123 (person getting access)
			"actor_service": serviceName, // merchant-service
			"role":          strings.Join(access.Roles, ","),
			"runtime_role":  runtimeRole, // Use parameter, not access object (access may not have it populated yet)
			"granted_by":    invitedBy,
			"granted_at":    access.GrantedAt,
			"method":        "direct", // Service layer should determine method
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish access granted activity to NATS: %v\n", err)
		}
	}

	return nil
}

// RecordInvitationUsed records an invitation used event to streams
func (r *ActivityRepository) RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error {
	// Publish to NATS for archival (SYNCHRONOUS - critical for audit trail)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":          "invitation_used",
			"thread_id":     threadID,
			"user_id":       userID,
			"actor":         userID,      // user-123 (person using invitation)
			"actor_service": serviceName, // merchant-service
			"role":          role,
			"invited_by":    invitedBy,
			"timestamp":     time.Now().Format(time.RFC3339),
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish invitation used to NATS: %v\n", err)
		}
	}

	return nil
}

// RecordThreadCreated records a thread created event to streams
func (r *ActivityRepository) RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error {
	// Publish thread metadata to NATS (SYNCHRONOUS - critical for PostgreSQL persistence)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishThreadMetadata(pubCtx, map[string]interface{}{
			"threadId":    threadID,
			"creatorId":   creatorID,
			"creatorRole": creatorRole,
			"createdAt":   time.Now().Format(time.RFC3339),
			"status":      "active",
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread metadata to NATS: %v\n", err)
		}
	}

	// Publish activity log to NATS (SYNCHRONOUS - critical for audit trail)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":          "thread_created",
			"thread_id":     threadID,
			"user_id":       creatorID,
			"actor":         creatorID,   // user-123
			"actor_service": serviceName, // merchant-service
			"role":          creatorRole,
			"timestamp":     time.Now().Format(time.RFC3339),
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread created activity to NATS: %v\n", err)
		}
	}

	return nil
}

// ArchiveValidationResults writes validation results to archival stream
func (r *ActivityRepository) ArchiveValidationResults(
	ctx context.Context,
	threadID string,
	stepID string,
	stepName string,
	idempotencyKey string,
	notifications []models.ValidationNotification,
	finalStatus string,
	hasCriticalViolation bool,
) error {
	// Build validation summary
	validationSummary := make([]map[string]interface{}, 0, len(notifications))
	criticalCount := 0
	warningCount := 0
	minorCount := 0
	infoCount := 0

	for _, notif := range notifications {
		validation := map[string]interface{}{
			"type":       notif.ViolationType,
			"severity":   notif.Severity,
			"message":    notif.Message,
			"passed":     notif.Status == "passed",
			"stepStatus": notif.StepStatus,
			"status":     notif.Status,
		}

		// Add details if present
		if notif.Details != nil && len(notif.Details) > 0 {
			validation["details"] = notif.Details
		}

		validationSummary = append(validationSummary, validation)

		// Count by severity
		switch notif.Severity {
		case string(models.SeverityCritical):
			criticalCount++
		case string(models.SeverityWarning):
			warningCount++
		case string(models.SeverityMinor):
			minorCount++
		case string(models.SeverityInfo):
			infoCount++
		}
	}

	// Marshal validations to JSON
	validationsJSON, err := json.Marshal(validationSummary)
	if err != nil {
		return fmt.Errorf("failed to marshal validations: %w", err)
	}

	// Build stream values for archival
	streamValues := map[string]interface{}{
		"validationID":         uuid.New().String(),
		"threadID":             threadID,
		"stepID":               stepID,
		"stepName":             stepName,
		"idempotencyKey":       idempotencyKey,
		"timestamp":            time.Now().Format(time.RFC3339),
		"validations":          string(validationsJSON),
		"overallStatus":        finalStatus,
		"hasCriticalViolation": hasCriticalViolation,
		"criticalCount":        criticalCount,
		"warningCount":         warningCount,
		"minorCount":           minorCount,
		"infoCount":            infoCount,
		"totalValidations":     len(notifications),
		"maxlen":               "~",
		"limit":                100000,
	}

	// Publish to NATS for archival (SYNCHRONOUS - critical for validation persistence)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishThreadValidation(pubCtx, streamValues); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread validations to NATS: %v\n", err)
		}
	}

	return nil
}

// ArchiveThreadMetadata writes thread metadata to archival stream
func (r *ActivityRepository) ArchiveThreadMetadata(ctx context.Context, thread *models.Thread, status string) error {
	// Convert contract version to string (handle nil pointer)
	contractVersion := "0"
	if thread.ContractVersion != nil {
		contractVersion = fmt.Sprintf("%d", *thread.ContractVersion)
	}

	// Handle nil contract ID
	contractID := ""
	if thread.ContractID != nil {
		contractID = *thread.ContractID
	}

	// Format completed timestamp
	completedAt := ""
	if thread.CompletedAt != nil {
		completedAt = thread.CompletedAt.Format(time.RFC3339)
	} else if status == "completed" {
		completedAt = time.Now().Format(time.RFC3339)
	}

	// Write to thread_metadata stream for normalized table
	streamValues := map[string]interface{}{
		"id":              thread.ID,
		"ownerId":         thread.OwnerID,
		"companyId":       thread.CompanyID,
		"contractId":      contractID,
		"contractVersion": contractVersion,
		"contractName":    thread.ContractName,
		"status":          status,
		"lastHash":        thread.LastHash,
		"startedAt":       thread.StartedAt.Format(time.RFC3339),
		"completedAt":     completedAt,
		"maxlen":          "~",
		"limit":           100000,
	}

	// Publish to NATS for archival (SYNCHRONOUS - critical for PostgreSQL persistence)
	if r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishThreadMetadata(pubCtx, streamValues); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread metadata to NATS: %v\n", err)
		}
	}

	// Publish thread_completed event to NATS for audit trail (SYNCHRONOUS - critical for audit trail)
	if status == "completed" && r.natsPublisher != nil {
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
			"type":          "thread_completed",
			"thread_id":     thread.ID,
			"actor":         thread.OwnerID, // user-123 (thread owner)
			"actor_service": "system",       // system-generated event
			"final_status":  status,
			"last_hash":     thread.LastHash,
			"completed_at":  completedAt,
			"timestamp":     time.Now().Format(time.RFC3339),
		}); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish thread completed activity to NATS: %v\n", err)
		}
	}

	return nil
}

// ArchiveStepState publishes step state snapshot to NATS for archival to Postgres
// SYNCHRONOUS - Critical for PostgreSQL persistence (DB-first architecture)
func (r *ActivityRepository) ArchiveStepState(ctx context.Context, stepState *interfaces.StepStateSnapshot) error {
	if r.natsPublisher == nil {
		return fmt.Errorf("NATS publisher not available")
	}

	stepStateData := map[string]interface{}{
		"step_id":         stepState.ID,
		"thread_id":       stepState.ThreadID,
		"step_name":       stepState.StepName,
		"idempotency_key": stepState.IdempotencyKey,
		"status":          stepState.Status,
		"retry_count":     stepState.RetryCount,
		"first_seen_at":   stepState.FirstSeenAt,
		"last_updated_at": stepState.LastUpdatedAt,
		"previous_step":   stepState.PreviousStep,
	}

	// Synchronous publish - fail request if NATS is down (PostgreSQL persistence is critical)
	if err := r.natsPublisher.PublishStepState(ctx, stepStateData); err != nil {
		return fmt.Errorf("failed to archive step state to NATS: %w", err)
	}

	fmt.Printf("✅ [ARCHIVE-SUCCESS] Step state archived for step=%s\n", stepState.StepName)
	return nil
}

// GetActivityLog retrieves activity log from PostgreSQL
// Activities are NEVER cached in Valkey - they go directly: NATS → PostgreSQL
func (r *ActivityRepository) GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error) {
	if r.postgresRepo == nil {
		return []map[string]interface{}{}, nil
	}

	return r.postgresRepo.GetActivityLog(ctx, threadID)
}
