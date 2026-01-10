package postgres

import (
	"context"
	"database/sql"
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
