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
// For activity/event history, use GetStepHistoryWithPermissionCheck()

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
			started_at,
			finished_at,
			previous_step,
			actor,
			actor_service,
			latest_context
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
		var previousStep, actor, actorService, latestContext, startedAt, finishedAt sql.NullString

		err := rows.Scan(
			&step.LatestStepID,
			&step.ThreadID,
			&step.StepName,
			&step.IdempotencyKey,
			&step.Status,
			&step.RetryCount,
			&step.FirstSeenAt,
			&step.LastUpdatedAt,
			&startedAt,
			&finishedAt,
			&previousStep,
			&actor,
			&actorService,
			&latestContext,
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
		if actorService.Valid {
			step.ActorService = actorService.String
		}
		if latestContext.Valid {
			step.LatestContext = latestContext.String
		}
		if startedAt.Valid {
			step.StartedAt = &startedAt.String
		}
		if finishedAt.Valid {
			step.FinishedAt = &finishedAt.String
		}

		stepsMap[step.ThreadID] = append(stepsMap[step.ThreadID], step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return stepsMap, nil
}

// GetStepsWithPermissionCheck retrieves steps with SQL-level permission filtering
// MVP: Company-wide access with cross-company sharing support
func (r *StepStateRepository) GetStepsWithPermissionCheck(
	ctx context.Context,
	threadID string,
	companyID string,
	stepName *string,
	idempotencyKey *string,
	status *string,
) ([]*models.StepStateInfo, error) {

	// Build query with company-level permission filtering
	// Users can view steps if they have access to the thread (company-wide or cross-company sharing)
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
			s.started_at,
			s.finished_at,
			s.previous_step,
			s.actor,
			s.actor_service,
			s.latest_context
		FROM thread_step_states s
		INNER JOIN threads t ON s.thread_id = t.id
		WHERE s.thread_id = $1
		  AND t.company_id = $2
	`

	args := []interface{}{threadID, companyID}
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
		var previousStep, actor, actorService, latestContext, startedAt, finishedAt sql.NullString

		err := rows.Scan(
			&step.LatestStepID,
			&step.ThreadID,
			&step.StepName,
			&step.IdempotencyKey,
			&step.Status,
			&step.RetryCount,
			&step.FirstSeenAt,
			&step.LastUpdatedAt,
			&startedAt,
			&finishedAt,
			&previousStep,
			&actor,
			&actorService,
			&latestContext,
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
		if actorService.Valid {
			step.ActorService = actorService.String
		}
		if latestContext.Valid {
			step.LatestContext = latestContext.String
		}
		if startedAt.Valid {
			step.StartedAt = &startedAt.String
		}
		if finishedAt.Valid {
			step.FinishedAt = &finishedAt.String
		}

		steps = append(steps, step)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return steps, nil
}

// GetStepHistoryWithPermissionCheck retrieves step history with SQL-level permission filtering
// MVP: Company-wide access with cross-company sharing support
func (r *StepStateRepository) GetStepHistoryWithPermissionCheck(
	ctx context.Context,
	threadID string,
	companyID string,
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

	// Build WHERE conditions with company-level permission check
	whereConditions := []string{
		"ta_activity.thread_id = $1",
	}
	params := []interface{}{threadID}
	paramIndex := 2

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

	// Add company permission check
	whereConditions = append(whereConditions, fmt.Sprintf("t.company_id = $%d", paramIndex))
	params = append(params, companyID)
	paramIndex++

	query := fmt.Sprintf(`
		SELECT 
			ta_activity.payload,
			ta_activity.status,
			ta_activity.recorded_at,
			ta_activity.actor,
			ta_activity.actor_service,
			ta_activity.started_at,
			ta_activity.finished_at,
			t.company_id,
			c.name as company_name,
			ROW_NUMBER() OVER (ORDER BY ta_activity.recorded_at ASC) as attempt_number
		FROM thread_activities ta_activity
		INNER JOIN threads t ON ta_activity.thread_id = t.id
		LEFT JOIN companies c ON t.company_id = c.id
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
		var actorVal sql.NullString
		var actorServiceVal sql.NullString
		var startedAtVal sql.NullTime
		var finishedAtVal sql.NullTime
		var companyIdVal sql.NullString
		var companyNameVal sql.NullString
		var attemptNumber int

		err := rows.Scan(&payload, &status, &recordedAt, &actorVal, &actorServiceVal, &startedAtVal, &finishedAtVal, &companyIdVal, &companyNameVal, &attemptNumber)
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

		// Extract actor and actor_service
		actorStr := ""
		if actorVal.Valid {
			actorStr = actorVal.String
		}
		actorServiceStr := ""
		if actorServiceVal.Valid {
			actorServiceStr = actorServiceVal.String
		}

		// Extract company info
		companyIdStr := ""
		if companyIdVal.Valid {
			companyIdStr = companyIdVal.String
		}
		companyNameStr := ""
		if companyNameVal.Valid {
			companyNameStr = companyNameVal.String
		}

		// Format timestamps
		startedAtStr := ""
		if startedAtVal.Valid {
			startedAtStr = startedAtVal.Time.Format(time.RFC3339Nano)
		}
		finishedAtStr := ""
		if finishedAtVal.Valid {
			finishedAtStr = finishedAtVal.Time.Format(time.RFC3339Nano)
		}

		stepHistory := models.StepHistory{
			Attempt:      attemptNumber,
			Timestamp:    recordedAt.Format(time.RFC3339Nano),
			Status:       statusValue,
			Context:      contextStr,
			Duration:     duration,
			StartedAt:    startedAtStr,
			FinishedAt:   finishedAtStr,
			Error:        errorMsg,
			Actor:        actorStr,
			ActorService: actorServiceStr,
			CompanyId:    companyIdStr,
			CompanyName:  companyNameStr,
		}

		history = append(history, stepHistory)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return history, nil
}
