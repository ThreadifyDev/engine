package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

// StepStateRepository implements step state retrieval from PostgreSQL thread_activities
type StepStateRepository struct {
	pool *pgxpool.Pool
}

// NewStepStateRepository creates a new PostgreSQL step state repository
func NewStepStateRepository(pool *pgxpool.Pool) *StepStateRepository {
	return &StepStateRepository{
		pool: pool,
	}
}

// GetStepState has been removed - use GetStepsBatch() instead
// Step state should always come from thread_step_states table, not thread_activities
// For activity/event history, use GetStepHistory()

// GetStepHistory retrieves filtered step history from thread_activities table
func (r *StepStateRepository) GetStepHistory(ctx context.Context, threadID, stepIdentifier string, limit, offset int, startAt, endAt, activityType, actor *string) ([]models.StepHistory, error) {
	// Validate pagination parameters
	if limit <= 0 {
		limit = 100 // Default to 100 records
	}
	if offset < 0 {
		offset = 0
	}

	// Build dynamic WHERE clause
	whereConditions := []string{"thread_id = $1"}
	params := []interface{}{threadID}
	paramIndex := 2

	// Handle step identifier (stepName or stepName:idempKey)
	if strings.Contains(stepIdentifier, ":") {
		// Exact match for stepName:idempKey
		whereConditions = append(whereConditions, fmt.Sprintf("step_id = $%d", paramIndex))
		params = append(params, stepIdentifier)
		paramIndex++
	} else {
		// Wildcard match for stepName (all idempotency keys)
		whereConditions = append(whereConditions, fmt.Sprintf("step_id LIKE $%d", paramIndex))
		params = append(params, stepIdentifier+":%")
		paramIndex++
	}

	// Add activity type filter if provided
	if activityType != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("activity_type = $%d", paramIndex))
		params = append(params, *activityType)
		paramIndex++
	} else {
		// Default to step_recorded if no activity type specified
		whereConditions = append(whereConditions, "activity_type = 'step_recorded'")
	}

	// Add actor filter if provided
	if actor != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("actor = $%d", paramIndex))
		params = append(params, *actor)
		paramIndex++
	}

	// Add date range filters if provided
	if startAt != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("recorded_at >= $%d", paramIndex))
		params = append(params, *startAt)
		paramIndex++
	}
	if endAt != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("recorded_at <= $%d", paramIndex))
		params = append(params, *endAt)
		paramIndex++
	}

	// Build final query - ORDER BY recorded_at DESC for most recent first
	// Use ROW_NUMBER() to get absolute attempt numbers regardless of pagination
	query := fmt.Sprintf(`
		SELECT 
			payload,
			status,
			recorded_at,
			ROW_NUMBER() OVER (ORDER BY recorded_at ASC) as attempt_number
		FROM thread_activities 
		WHERE %s
		ORDER BY recorded_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(whereConditions, " AND "), paramIndex, paramIndex+1)

	params = append(params, limit, offset)

	rows, err := r.pool.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query step history: %w", err)
	}
	defer rows.Close()

	var history []models.StepHistory

	for rows.Next() {
		var payload sql.NullString
		var status sql.NullString
		var recordedAt time.Time
		var attemptNumber int

		err := rows.Scan(&payload, &status, &recordedAt, &attemptNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step history: %w", err)
		}

		// Parse payload JSON to extract context, duration, and status
		var contextStr string
		var duration int
		var errorMsg string
		var statusFromPayload string

		if payload.Valid {
			var payloadData map[string]interface{}
			if err := json.Unmarshal([]byte(payload.String), &payloadData); err == nil {
				// Extract context as JSON string
				if ctx, exists := payloadData["context"]; exists {
					if ctxBytes, err := json.Marshal(ctx); err == nil {
						contextStr = string(ctxBytes)
					}
				}

				// Extract duration if available
				if dur, exists := payloadData["duration"]; exists {
					if durFloat, ok := dur.(float64); ok {
						duration = int(durFloat)
					}
				}

				// Extract status from payload (primary source)
				if statusVal, exists := payloadData["status"]; exists {
					if statusStr, ok := statusVal.(string); ok {
						statusFromPayload = statusStr
					}
				}

				// Extract error if status is failed/error
				if statusFromPayload == "failed" || statusFromPayload == "error" {
					if err, exists := payloadData["error"]; exists {
						if errStr, ok := err.(string); ok {
							errorMsg = errStr
						}
					}
				}
			}
		}

		// Use status from payload if available, otherwise use status column
		statusValue := statusFromPayload
		if statusValue == "" && status.Valid {
			statusValue = status.String
		}

		stepHistory := models.StepHistory{
			Attempt:   attemptNumber,
			Timestamp: recordedAt.Format(time.RFC3339),
			Status:    statusValue,
			Context:   contextStr,
			Duration:  duration,
			Error:     errorMsg,
		}

		history = append(history, stepHistory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return history, nil
}

// GetStepsBatch retrieves steps for multiple threads from thread_step_states table in a single query
// Returns a map of threadID -> list of step states
func (r *StepStateRepository) GetStepsBatch(ctx context.Context, threadIDs []string) (map[string][]*models.StepStateInfo, error) {
	if len(threadIDs) == 0 {
		return make(map[string][]*models.StepStateInfo), nil
	}

	query := `
		SELECT 
			id,
			thread_id,
			step_name,
			idempotency_key,
			status,
			retry_count,
			first_seen_at,
			last_updated_at,
			previous_step,
			actor
		FROM thread_step_states
		WHERE thread_id = ANY($1)
		ORDER BY thread_id, first_seen_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query steps batch: %w", err)
	}
	defer rows.Close()

	stepsMap := make(map[string][]*models.StepStateInfo)
	for rows.Next() {
		step := &models.StepStateInfo{}
		var previousStep, actor sql.NullString

		err := rows.Scan(
			&step.LatestStepID,
			&step.ThreadID,
			&step.StepName,
			&step.IdempotencyKey,
			&step.Status,
			&step.RetryCount,
			&step.FirstSeenAt,
			&step.LastUpdatedAt,
			&previousStep,
			&actor,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step: %w", err)
		}

		if previousStep.Valid {
			step.PreviousStep = previousStep.String
		}
		if actor.Valid {
			step.Actor = actor.String
		}

		stepsMap[step.ThreadID] = append(stepsMap[step.ThreadID], step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return stepsMap, nil
}

// GetStepsWithPermissionCheck retrieves steps with SQL-level permission filtering
// Filters by thread_access permissions and actor for .own permissions
func (r *StepStateRepository) GetStepsWithPermissionCheck(
	ctx context.Context,
	threadID string,
	userID string,
	stepName *string,
	idempotencyKey *string,
	status *string,
) ([]*models.StepStateInfo, error) {
	// Build query with permission filtering via JOIN to thread_access
	// Permission logic:
	// - thread.read.* = can see all steps
	// - thread.read.own = can only see steps where actor = userID
	query := `
		SELECT 
			s.id,
			s.thread_id,
			s.step_name,
			s.idempotency_key,
			s.status,
			s.retry_count,
			s.first_seen_at,
			s.last_updated_at,
			s.previous_step,
			s.actor
		FROM thread_step_states s
		INNER JOIN thread_access ta ON s.thread_id = ta.thread_id
		WHERE s.thread_id = $1
		  AND ta.user_id = $2
		  AND ta.status = 'active'
		  AND (
			'thread.read.*' = ANY(ta.permissions)
			OR ('thread.read.own' = ANY(ta.permissions) AND s.actor = $2)
		  )
	`

	args := []interface{}{threadID, userID}
	argIdx := 3

	// Add optional filters
	if stepName != nil && *stepName != "" {
		query += fmt.Sprintf(" AND s.step_name = $%d", argIdx)
		args = append(args, *stepName)
		argIdx++
	}

	if idempotencyKey != nil && *idempotencyKey != "" {
		query += fmt.Sprintf(" AND s.idempotency_key = $%d", argIdx)
		args = append(args, *idempotencyKey)
		argIdx++
	}

	if status != nil && *status != "" {
		query += fmt.Sprintf(" AND s.status = $%d", argIdx)
		args = append(args, *status)
		argIdx++
	}

	query += " ORDER BY s.first_seen_at ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query steps with permission check: %w", err)
	}
	defer rows.Close()

	var steps []*models.StepStateInfo
	for rows.Next() {
		step := &models.StepStateInfo{}
		var previousStep, actor sql.NullString

		err := rows.Scan(
			&step.LatestStepID,
			&step.ThreadID,
			&step.StepName,
			&step.IdempotencyKey,
			&step.Status,
			&step.RetryCount,
			&step.FirstSeenAt,
			&step.LastUpdatedAt,
			&previousStep,
			&actor,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step: %w", err)
		}

		if previousStep.Valid {
			step.PreviousStep = previousStep.String
		}
		if actor.Valid {
			step.Actor = actor.String
		}

		steps = append(steps, step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return steps, nil
}

// GetStepHistoryWithPermissionCheck retrieves step history with SQL-level permission filtering
func (r *StepStateRepository) GetStepHistoryWithPermissionCheck(
	ctx context.Context,
	threadID string,
	userID string,
	stepIdentifier string,
	limit int,
	offset int,
	startAt *string,
	endAt *string,
	activityType *string,
	actorFilter *string,
) ([]models.StepHistory, error) {
	// Validate pagination parameters
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Build WHERE conditions with permission check
	whereConditions := []string{
		"ta_activity.thread_id = $1",
		"acc.user_id = $2",
		"acc.status = 'active'",
		`(
			'thread.read.*' = ANY(acc.permissions)
			OR ('thread.read.own' = ANY(acc.permissions) AND ta_activity.actor = $2)
		)`,
	}
	params := []interface{}{threadID, userID}
	paramIndex := 3

	// Handle step identifier (stepName or stepName:idempKey)
	if strings.Contains(stepIdentifier, ":") {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.step_id = $%d", paramIndex))
		params = append(params, stepIdentifier)
		paramIndex++
	} else {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.step_id LIKE $%d", paramIndex))
		params = append(params, stepIdentifier+":%")
		paramIndex++
	}

	// Add activity type filter
	if activityType != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.activity_type = $%d", paramIndex))
		params = append(params, *activityType)
		paramIndex++
	} else {
		whereConditions = append(whereConditions, "ta_activity.activity_type = 'step_recorded'")
	}

	// Add actor filter if provided
	if actorFilter != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.actor = $%d", paramIndex))
		params = append(params, *actorFilter)
		paramIndex++
	}

	// Add date range filters
	if startAt != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.recorded_at >= $%d", paramIndex))
		params = append(params, *startAt)
		paramIndex++
	}
	if endAt != nil {
		whereConditions = append(whereConditions, fmt.Sprintf("ta_activity.recorded_at <= $%d", paramIndex))
		params = append(params, *endAt)
		paramIndex++
	}

	query := fmt.Sprintf(`
		SELECT 
			ta_activity.payload,
			ta_activity.status,
			ta_activity.recorded_at,
			ROW_NUMBER() OVER (ORDER BY ta_activity.recorded_at ASC) as attempt_number
		FROM thread_activities ta_activity
		INNER JOIN thread_access acc ON ta_activity.thread_id = acc.thread_id
		WHERE %s
		ORDER BY ta_activity.recorded_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(whereConditions, " AND "), paramIndex, paramIndex+1)

	params = append(params, limit, offset)

	rows, err := r.pool.Query(ctx, query, params...)
	if err != nil {
		return nil, fmt.Errorf("failed to query step history with permission check: %w", err)
	}
	defer rows.Close()

	var history []models.StepHistory

	for rows.Next() {
		var payload sql.NullString
		var status sql.NullString
		var recordedAt time.Time
		var attemptNumber int

		err := rows.Scan(&payload, &status, &recordedAt, &attemptNumber)
		if err != nil {
			return nil, fmt.Errorf("failed to scan step history: %w", err)
		}

		// Parse payload JSON to extract context, duration, and status
		var contextStr string
		var duration int
		var errorMsg string
		var statusFromPayload string

		if payload.Valid {
			var payloadData map[string]interface{}
			if err := json.Unmarshal([]byte(payload.String), &payloadData); err == nil {
				if ctx, exists := payloadData["context"]; exists {
					if ctxBytes, err := json.Marshal(ctx); err == nil {
						contextStr = string(ctxBytes)
					}
				}
				if dur, exists := payloadData["duration"]; exists {
					if durFloat, ok := dur.(float64); ok {
						duration = int(durFloat)
					}
				}
				if statusVal, exists := payloadData["status"]; exists {
					if statusStr, ok := statusVal.(string); ok {
						statusFromPayload = statusStr
					}
				}
				if statusFromPayload == "failed" || statusFromPayload == "error" {
					if err, exists := payloadData["error"]; exists {
						if errStr, ok := err.(string); ok {
							errorMsg = errStr
						}
					}
				}
			}
		}

		statusValue := statusFromPayload
		if statusValue == "" && status.Valid {
			statusValue = status.String
		}

		stepHistory := models.StepHistory{
			Attempt:   attemptNumber,
			Timestamp: recordedAt.Format(time.RFC3339),
			Status:    statusValue,
			Context:   contextStr,
			Duration:  duration,
			Error:     errorMsg,
		}

		history = append(history, stepHistory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return history, nil
}
