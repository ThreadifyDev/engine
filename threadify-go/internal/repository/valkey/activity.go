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
	"github.com/threadify/engine/internal/utils"
)

// ActivityRepository handles stream and event operations in Valkey
type ActivityRepository struct {
	valkey interfaces.ValkeyClient
}

// NewActivityRepository creates a new activity repository
func NewActivityRepository(valkey interfaces.ValkeyClient) *ActivityRepository {
	return &ActivityRepository{
		valkey: valkey,
	}
}

// RecordAccessGranted records an access granted event to streams
func (r *ActivityRepository) RecordAccessGranted(ctx context.Context, threadID, userID string, access *interfaces.UserAccess, invitedBy, serviceName, scope string) error {
	// Determine event type based on context
	eventType := "access_granted"
	if invitedBy != "self" && len(access.Roles) > 1 {
		eventType = "role_added"
	}

	// Write to streams:thread_access for archival
	rolesJSON, _ := json.Marshal(access.Roles)
	_, err := r.valkey.XAdd(ctx, "streams:thread_access", map[string]interface{}{
		"threadId":    threadID,
		"userId":      userID,
		"roles":       string(rolesJSON),
		"permissions": strings.Join(access.Permissions, ","),
		"grantedBy":   invitedBy,
		"grantedAt":   access.GrantedAt,
		"status":      access.Status,
		"event_type":  eventType,
		"scope":       scope,
	})
	if err != nil {
		return fmt.Errorf("failed to write access granted to stream: %w", err)
	}

	// Write to thread activity stream
	// Note: thread:{id}:activity is now a hash, not a stream
	// Activity events are handled by partitioned streams only

	// Write to partitioned stream
	partition := utils.GetPartitionForThread(threadID)
	partitionedStream := fmt.Sprintf("streams:activity_log:%d", partition)
	accessValues := map[string]interface{}{
		"type":          "access_granted",
		"thread_id":     threadID,
		"user_id":       userID,
		"actor":         userID,      // user-123 (person getting access)
		"actor_service": serviceName, // merchant-service
		"role":          strings.Join(access.Roles, ","),
		"permissions":   strings.Join(access.Permissions, ","),
		"granted_by":    invitedBy,
		"granted_at":    access.GrantedAt,
		"method":        "direct", // Service layer should determine method
	}
	_, err = r.valkey.XAdd(ctx, partitionedStream, accessValues)
	if err != nil {
		return fmt.Errorf("failed to write access granted to partitioned stream: %w", err)
	}

	return nil
}

// RecordInvitationUsed records an invitation used event to streams
func (r *ActivityRepository) RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error {
	// Write to thread activity stream
	// Note: thread:{id}:activity is now a hash, not a stream
	// Activity events are handled by partitioned streams only
	partition := utils.GetPartitionForThread(threadID)
	partitionedStream := fmt.Sprintf("streams:activity_log:%d", partition)
	invitationValues := map[string]interface{}{
		"type":          "invitation_used",
		"thread_id":     threadID,
		"user_id":       userID,
		"actor":         userID,      // user-123 (person using invitation)
		"actor_service": serviceName, // merchant-service
		"role":          role,
		"invited_by":    invitedBy,
		"timestamp":     time.Now().Format(time.RFC3339),
	}

	_, err := r.valkey.XAdd(ctx, partitionedStream, invitationValues)
	if err != nil {
		return fmt.Errorf("failed to write invitation used to partitioned stream: %w", err)
	}

	return nil
}

// RecordThreadCreated records a thread created event to streams
func (r *ActivityRepository) RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error {
	// Write to streams:thread_metadata for archival
	_, err := r.valkey.XAdd(ctx, "streams:thread_metadata", map[string]interface{}{
		"threadId":    threadID,
		"creatorId":   creatorID,
		"creatorRole": creatorRole,
		"createdAt":   time.Now().Format(time.RFC3339),
		"status":      "active",
	})
	if err != nil {
		return fmt.Errorf("failed to write thread created to metadata stream: %w", err)
	}

	// Write to thread activity stream
	// Note: thread:{id}:activity is now a hash, not a stream
	// Activity events are handled by partitioned streams only

	// Write to partitioned stream
	partition := utils.GetPartitionForThread(threadID)
	partitionedStream := fmt.Sprintf("streams:activity_log:%d", partition)
	activityValues := map[string]interface{}{
		"type":          "thread_created",
		"thread_id":     threadID,
		"user_id":       creatorID,
		"actor":         creatorID,   // user-123
		"actor_service": serviceName, // merchant-service
		"role":          creatorRole,
		"timestamp":     time.Now().Format(time.RFC3339),
	}
	_, err = r.valkey.XAdd(ctx, partitionedStream, activityValues)
	if err != nil {
		return fmt.Errorf("failed to write thread created to partitioned stream: %w", err)
	}

	return nil
}

// Helper methods for key generation

func (r *ActivityRepository) getActivityStreamKey(threadID string) string {
	return fmt.Sprintf("thread:%s:activity", threadID)
}

// StoreValidationNotification stores validation notification in stream
func (r *ActivityRepository) StoreValidationNotification(ctx context.Context, notif models.ValidationNotification) error {
	// Build stream values
	streamValues := map[string]interface{}{
		"notificationId": notif.NotificationID,
		"threadId":       notif.ThreadID,
		"stepId":         notif.StepID,
		"stepName":       notif.StepName,
		"ownerId":        notif.OwnerID,
		"status":         string(notif.Status),
		"message":        notif.Message,
		"timestamp":      notif.Timestamp.Format(time.RFC3339),
		"maxlen":         "~",
		"limit":          100000,
	}

	// Add optional fields
	if notif.ViolationType != "" {
		streamValues["violationType"] = string(notif.ViolationType)
	}
	if notif.Severity != "" {
		streamValues["severity"] = string(notif.Severity)
	}
	if notif.FromStep != "" {
		streamValues["fromStep"] = notif.FromStep
		streamValues["toStep"] = notif.ToStep
	}
	if notif.Duration != "" {
		streamValues["duration"] = notif.Duration
		streamValues["limit"] = notif.Limit
	}
	if len(notif.MissingFields) > 0 {
		streamValues["missingFields"] = strings.Join(notif.MissingFields, ",")
	}
	if len(notif.ExtraFields) > 0 {
		streamValues["extraFields"] = strings.Join(notif.ExtraFields, ",")
	}
	if notif.Details != nil {
		detailsJSON, _ := json.Marshal(notif.Details)
		streamValues["details"] = string(detailsJSON)
	}

	_, err := r.valkey.XAdd(ctx, "streams:validation_notifications", streamValues)
	return err
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
			"type":     string(notif.ViolationType),
			"severity": string(notif.Severity),
			"message":  notif.Message,
			"passed":   notif.Status == models.NotificationStatusCompleted,
		}

		// Add optional fields if present
		if notif.Duration != "" {
			validation["duration"] = notif.Duration
		}
		if notif.Limit != "" {
			validation["limit"] = notif.Limit
		}
		if len(notif.MissingFields) > 0 {
			validation["missingFields"] = notif.MissingFields
		}
		if notif.Details != nil {
			validation["details"] = notif.Details
		}

		validationSummary = append(validationSummary, validation)

		// Count by severity
		switch notif.Severity {
		case models.SeverityCritical:
			criticalCount++
		case models.SeverityWarning:
			warningCount++
		case models.SeverityMinor:
			minorCount++
		case models.SeverityInfo:
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

	// Write to thread_validations stream for archival
	_, err = r.valkey.XAdd(ctx, "streams:thread_validations", streamValues)
	if err != nil {
		return fmt.Errorf("failed to write to thread_validations stream: %w", err)
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

	_, err := r.valkey.XAdd(ctx, "streams:thread_metadata", streamValues)
	if err != nil {
		return fmt.Errorf("failed to write thread metadata to stream: %w", err)
	}

	// Write thread_completed event to partitioned stream for audit trail
	if status == "completed" {
		partition := utils.GetPartitionForThread(thread.ID)
		partitionedStream := fmt.Sprintf("streams:activity_log:%d", partition)
		activityValues := map[string]interface{}{
			"type":          "thread_completed",
			"thread_id":     thread.ID,
			"actor":         thread.OwnerID, // user-123 (thread owner)
			"actor_service": "system",       // system-generated event
			"final_status":  status,
			"last_hash":     thread.LastHash,
			"completed_at":  completedAt,
			"timestamp":     time.Now().Format(time.RFC3339),
		}
		_, err = r.valkey.XAdd(ctx, partitionedStream, activityValues)
		if err != nil {
			return fmt.Errorf("failed to write thread completed to partitioned stream: %w", err)
		}
	}

	return nil
}
