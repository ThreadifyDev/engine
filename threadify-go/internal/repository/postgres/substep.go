package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
)

// SubStepRepository handles sub-step database operations
type SubStepRepository struct {
	pool *pgxpool.Pool
}

// NewSubStepRepository creates a new sub-step repository
func NewSubStepRepository(pool *pgxpool.Pool) *SubStepRepository {
	return &SubStepRepository{pool: pool}
}

// GetSubStepsByStepID retrieves all sub-steps for a given step
func (r *SubStepRepository) GetSubStepsByStepID(ctx context.Context, stepID string) ([]*domain.SubStep, error) {
	query := `
		SELECT 
			id, 
			thread_id, 
			step_id, 
			name, 
			status, 
			payload, 
			recorded_at, 
			created_at
		FROM step_substeps
		WHERE step_id = $1
		ORDER BY recorded_at ASC
	`

	rows, err := r.pool.Query(ctx, query, stepID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sub-steps: %w", err)
	}
	defer rows.Close()

	var subSteps []*domain.SubStep
	for rows.Next() {
		var subStep domain.SubStep
		var payloadJSON []byte

		err := rows.Scan(
			&subStep.ID,
			&subStep.ThreadID,
			&subStep.StepID,
			&subStep.Name,
			&subStep.Status,
			&payloadJSON,
			&subStep.RecordedAt,
			&subStep.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sub-step: %w", err)
		}

		// Parse payload JSON
		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &subStep.Payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
			}
		}

		subSteps = append(subSteps, &subStep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sub-steps: %w", err)
	}

	return subSteps, nil
}

// GetSubStepsByThreadID retrieves all sub-steps for a given thread
func (r *SubStepRepository) GetSubStepsByThreadID(ctx context.Context, threadID string, stepID string) ([]*domain.SubStep, error) {
	query := `
		SELECT 
			id, 
			thread_id, 
			step_id, 
			name, 
			status, 
			payload, 
			recorded_at, 
			created_at
		FROM step_substeps
		WHERE thread_id = $1 AND step_id = $2
		ORDER BY recorded_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID, stepID)
	if err != nil {
		return nil, fmt.Errorf("failed to query sub-steps: %w", err)
	}
	defer rows.Close()

	var subSteps []*domain.SubStep
	for rows.Next() {
		var subStep domain.SubStep
		var payloadJSON []byte

		err := rows.Scan(
			&subStep.ID,
			&subStep.ThreadID,
			&subStep.StepID,
			&subStep.Name,
			&subStep.Status,
			&payloadJSON,
			&subStep.RecordedAt,
			&subStep.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan sub-step: %w", err)
		}

		// Parse payload JSON
		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &subStep.Payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
			}
		}

		subSteps = append(subSteps, &subStep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sub-steps: %w", err)
	}

	return subSteps, nil
}
