package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
)

// ThreadNotificationRepository handles queries for thread notifications
// Now reads from thread_activities table where activity_type='validation_result'
type ThreadNotificationRepository struct {
	pool *pgxpool.Pool
}

// NewThreadNotificationRepository creates a new repository instance
func NewThreadNotificationRepository(pool *pgxpool.Pool) *ThreadNotificationRepository {
	return &ThreadNotificationRepository{pool: pool}
}

// GetThreadNotifications retrieves notifications for a thread with optional filters
// Reads from thread_activities table where activity_type='validation_result'
func (r *ThreadNotificationRepository) GetThreadNotifications(
	ctx context.Context,
	threadID string,
	options *domain.ThreadNotificationQueryOptions,
) ([]*domain.ThreadNotification, error) {
	query := `
		SELECT 
			payload->>'notification_id' as notification_id,
			thread_id,
			step_id,
			SPLIT_PART(step_id, ':', 1) as step_name,
			SPLIT_PART(step_id, ':', 2) as idempotency_key,
			payload->>'source' as source,
			payload->>'notification_type' as notification_type,
			payload->>'step_status' as step_status,
			status as validation_status,
			payload->>'violation_type' as violation_type,
			payload->>'severity' as severity,
			payload->>'message' as message,
			payload->>'details' as details,
			recorded_at as timestamp
		FROM thread_activities 
		WHERE thread_id = $1 AND activity_type = 'validation_result'
	`

	args := []interface{}{threadID}
	argIndex := 2

	// Add optional filters
	if options != nil {
		if options.StepID != "" {
			query += fmt.Sprintf(" AND step_id = $%d", argIndex)
			args = append(args, options.StepID)
			argIndex++
		}

		if options.StepName != "" {
			query += fmt.Sprintf(" AND SPLIT_PART(step_id, ':', 1) = $%d", argIndex)
			args = append(args, options.StepName)
			argIndex++
		}

		if options.Source != "" {
			query += fmt.Sprintf(" AND payload->>'source' = $%d", argIndex)
			args = append(args, options.Source)
			argIndex++
		}

		if options.NotificationType != "" {
			query += fmt.Sprintf(" AND payload->>'notification_type' = $%d", argIndex)
			args = append(args, options.NotificationType)
			argIndex++
		}

		if len(options.Severity) > 0 {
			placeholders := make([]string, len(options.Severity))
			for i, sev := range options.Severity {
				placeholders[i] = fmt.Sprintf("$%d", argIndex)
				args = append(args, sev)
				argIndex++
			}
			query += fmt.Sprintf(" AND payload->>'severity' IN (%s)", strings.Join(placeholders, ","))
		}
	}

	// Order by timestamp descending (newest first)
	query += " ORDER BY recorded_at DESC"

	// Add limit and offset
	if options != nil {
		if options.Limit > 0 {
			query += fmt.Sprintf(" LIMIT $%d", argIndex)
			args = append(args, options.Limit)
			argIndex++
		} else {
			// Default limit to prevent excessive results
			query += fmt.Sprintf(" LIMIT $%d", argIndex)
			args = append(args, 100)
			argIndex++
		}

		if options.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, options.Offset)
		}
	} else {
		// Default limit
		query += " LIMIT 100"
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.ThreadNotification
	for rows.Next() {
		var notif domain.ThreadNotification
		var detailsStr *string
		var idempotencyKey, stepStatus, validationStatus, violationType, severity *string

		err := rows.Scan(
			&notif.NotificationID,
			&notif.ThreadID,
			&notif.StepID,
			&notif.StepName,
			&idempotencyKey,
			&notif.Source,
			&notif.NotificationType,
			&stepStatus,
			&validationStatus,
			&violationType,
			&severity,
			&notif.Message,
			&detailsStr,
			&notif.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}

		// Handle nullable fields
		if idempotencyKey != nil {
			notif.IdempotencyKey = *idempotencyKey
		}
		if stepStatus != nil {
			notif.StepStatus = *stepStatus
		}
		if validationStatus != nil {
			notif.ValidationStatus = *validationStatus
		}
		if violationType != nil {
			notif.ViolationType = *violationType
		}
		if severity != nil {
			notif.Severity = *severity
		}

		// Parse details JSON string if present
		// Details is stored as a JSON string within the JSONB payload
		if detailsStr != nil && *detailsStr != "" {
			if err := json.Unmarshal([]byte(*detailsStr), &notif.Details); err != nil {
				return nil, fmt.Errorf("failed to parse details JSON: %w", err)
			}
		}

		notifications = append(notifications, &notif)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", err)
	}

	return notifications, nil
}

// GetNotificationSummary retrieves aggregated counts for a thread (lightweight query)
// Reads from thread_activities table where activity_type='validation_result'
// Note: All archived notifications are validation source (execution is ephemeral)
func (r *ThreadNotificationRepository) GetNotificationSummary(
	ctx context.Context,
	threadID string,
) (*domain.NotificationSummary, error) {
	query := `
		SELECT 
			COUNT(*) as total_notifications,
			COUNT(*) FILTER (WHERE payload->>'severity' = 'critical') as critical_count,
			COUNT(*) FILTER (WHERE payload->>'severity' = 'warning') as warning_count,
			COUNT(*) FILTER (WHERE payload->>'severity' = 'major') as major_count,
			COUNT(*) FILTER (WHERE payload->>'severity' = 'minor') as minor_count,
			COUNT(*) FILTER (WHERE payload->>'severity' = 'info') as info_count
		FROM thread_activities 
		WHERE thread_id = $1 AND activity_type = 'validation_result'
	`

	var summary domain.NotificationSummary
	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&summary.TotalNotifications,
		&summary.CriticalCount,
		&summary.WarningCount,
		&summary.MajorCount,
		&summary.MinorCount,
		&summary.InfoCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query notification summary: %w", err)
	}

	// Set boolean flags
	summary.HasCritical = summary.CriticalCount > 0
	summary.HasWarnings = summary.WarningCount > 0

	// All archived notifications are validation source (execution is ephemeral)
	summary.ValidationCount = summary.TotalNotifications
	summary.ExecutionCount = 0

	return &summary, nil
}

// GetGlobalNotifications retrieves cross-thread notifications with optional filters.
func (r *ThreadNotificationRepository) GetGlobalNotifications(
	ctx context.Context,
	companyID string,
	contractName *string,
	refKey *string,
	refValue *string,
	severity []string,
	startedAfter *string,
	startedBefore *string,
	limit int,
	offset int,
) ([]*domain.ThreadNotification, error) {
	query := `
		SELECT 
			a.notification_id,
			a.thread_id,
			a.step_id,
			a.step_name,
			a.idempotency_key,
			a.source,
			a.notification_type,
			a.step_status,
			a.validation_status,
			a.violation_type,
			a.severity,
			a.message,
			a.details::text as details,
			a.timestamp
		FROM thread_notifications a
		JOIN threads t ON a.thread_id = t.id
	`

	// Add joins if we need to filter by refs
	if (refKey != nil && *refKey != "") || (refValue != nil && *refValue != "") {
		query += ` JOIN thread_refs r ON a.thread_id = r.thread_id`
	}

	query += ` WHERE t.company_id = $1`

	args := []interface{}{companyID}
	argIndex := 2

	if contractName != nil && *contractName != "" {
		query += fmt.Sprintf(" AND t.contract_name = $%d", argIndex)
		args = append(args, *contractName)
		argIndex++
	}

	if refKey != nil && *refKey != "" {
		query += fmt.Sprintf(" AND r.ref_key = $%d", argIndex)
		args = append(args, *refKey)
		argIndex++
	}

	if refValue != nil && *refValue != "" {
		query += fmt.Sprintf(" AND r.ref_value = $%d", argIndex)
		args = append(args, *refValue)
		argIndex++
	}

	if len(severity) > 0 {
		placeholders := make([]string, len(severity))
		for i, sev := range severity {
			placeholders[i] = fmt.Sprintf("$%d", argIndex)
			args = append(args, sev)
			argIndex++
		}
		query += fmt.Sprintf(" AND a.severity IN (%s)", strings.Join(placeholders, ","))
	}

	if startedAfter != nil && *startedAfter != "" {
		query += fmt.Sprintf(" AND a.timestamp >= $%d", argIndex)
		args = append(args, *startedAfter)
		argIndex++
	}

	if startedBefore != nil && *startedBefore != "" {
		query += fmt.Sprintf(" AND a.timestamp <= $%d", argIndex)
		args = append(args, *startedBefore)
		argIndex++
	}

	query += " ORDER BY a.timestamp DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
		argIndex++
	} else {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, 100)
		argIndex++
	}

	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, offset)
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query global notifications: %w", err)
	}
	defer rows.Close()

	var notifications []*domain.ThreadNotification
	for rows.Next() {
		var notif domain.ThreadNotification
		var detailsStr *string
		var idempotencyKey, stepStatus, validationStatus, violationType, sev *string

		err := rows.Scan(
			&notif.NotificationID,
			&notif.ThreadID,
			&notif.StepID,
			&notif.StepName,
			&idempotencyKey,
			&notif.Source,
			&notif.NotificationType,
			&stepStatus,
			&validationStatus,
			&violationType,
			&sev,
			&notif.Message,
			&detailsStr,
			&notif.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan global notification: %w", err)
		}

		if idempotencyKey != nil {
			notif.IdempotencyKey = *idempotencyKey
		}
		if stepStatus != nil {
			notif.StepStatus = *stepStatus
		}
		if validationStatus != nil {
			notif.ValidationStatus = *validationStatus
		}
		if violationType != nil {
			notif.ViolationType = *violationType
		}
		if sev != nil {
			notif.Severity = *sev
		}

		if detailsStr != nil && *detailsStr != "" {
			if err := json.Unmarshal([]byte(*detailsStr), &notif.Details); err != nil {
				return nil, fmt.Errorf("failed to parse details JSON: %w", err)
			}
		}

		notifications = append(notifications, &notif)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating global notifications: %w", err)
	}

	return notifications, nil
}
