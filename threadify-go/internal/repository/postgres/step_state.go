package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
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

// GetStepState reconstructs step state from thread_activities table
// Returns nil if no activities found for the step
func (r *StepStateRepository) GetStepState(ctx context.Context, threadID, stepName, idempotencyKey string) (*models.StepStateInfo, error) {
	stepID := fmt.Sprintf("%s:%s", stepName, idempotencyKey)

	query := `
		SELECT 
			step_id,
			activity_type,
			actor_service,
			payload,
			recorded_at
		FROM thread_activities 
		WHERE thread_id = $1 AND step_id = $2
		ORDER BY recorded_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID, stepID)
	if err != nil {
		return nil, fmt.Errorf("failed to query step activities: %w", err)
	}
	defer rows.Close()

	var activities []struct {
		StepID       string    `json:"step_id"`
		ActivityType string    `json:"activity_type"`
		ActorService string    `json:"actor_service"`
		Payload      string    `json:"payload"`
		RecordedAt   time.Time `json:"recorded_at"`
	}

	// Collect all activities
	for rows.Next() {
		var activity struct {
			StepID       string    `json:"step_id"`
			ActivityType string    `json:"activity_type"`
			ActorService string    `json:"actor_service"`
			Payload      string    `json:"payload"`
			RecordedAt   time.Time `json:"recorded_at"`
		}

		var payload sql.NullString
		err := rows.Scan(
			&activity.StepID,
			&activity.ActivityType,
			&activity.ActorService,
			&payload,
			&activity.RecordedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan activity row: %w", err)
		}

		if payload.Valid {
			activity.Payload = payload.String
		}

		activities = append(activities, activity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// No activities found
	if len(activities) == 0 {
		log.Printf("❌ No step activities found in PostgreSQL: thread=%s, step=%s", threadID, stepID)
		return nil, nil
	}

	log.Printf("📥 Found %d step activities in PostgreSQL: thread=%s, step=%s", len(activities), threadID, stepID)

	// Reconstruct step state from activities
	stepState := &models.StepStateInfo{
		ThreadID:       threadID,
		StepName:       stepName,
		IdempotencyKey: idempotencyKey,
		RetryCount:     len(activities) - 1, // First attempt is retry 0
		FirstSeenAt:    activities[0].RecordedAt,
		LastUpdatedAt:  activities[len(activities)-1].RecordedAt,
	}

	// Determine final status from the latest activity payload
	latestActivity := activities[len(activities)-1]
	stepState.Status = "pending" // Default status

	// Parse payload JSON to extract status
	if latestActivity.Payload != "" {
		// Simple JSON parsing to extract status field
		if strings.Contains(latestActivity.Payload, `"status":`) {
			parts := strings.Split(latestActivity.Payload, `"status":"`)
			if len(parts) > 1 {
				statusPart := strings.Split(parts[1], `"`)[0]
				switch statusPart {
				case "completed":
					stepState.Status = "success"
				case "failed", "error":
					stepState.Status = "failed"
				case "violated":
					stepState.Status = "violated"
				case "pending", "in_progress":
					stepState.Status = "pending"
				default:
					stepState.Status = statusPart // Use the actual status
				}
			}
		}
	}

	// Extract step ID from payload if available
	if latestActivity.Payload != "" {
		// Try to extract step ID from JSON payload
		if strings.Contains(latestActivity.Payload, "step_id") {
			// Simple extraction - in production, use proper JSON parsing
			parts := strings.Split(latestActivity.Payload, "\"step_id\":\"")
			if len(parts) > 1 {
				stepIDPart := strings.Split(parts[1], "\"")[0]
				stepState.LatestStepID = stepIDPart
			}
		}
	}

	// Determine previous step from the second-to-last activity if available
	if len(activities) > 1 {
		previousActivity := activities[len(activities)-2]
		stepState.PreviousStep = previousActivity.StepID
	}

	return stepState, nil
}

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
