package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

// ThreadRepository handles thread persistence in PostgreSQL
type ThreadRepository struct {
	pool *pgxpool.Pool
}

// NewThreadRepository creates a new Postgres thread repository
func NewThreadRepository(pool *pgxpool.Pool) *ThreadRepository {
	return &ThreadRepository{
		pool: pool,
	}
}

// Save persists a thread to PostgreSQL
func (r *ThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	// Serialize context and steps to JSON
	contextJSON, err := json.Marshal(thread.Context)
	if err != nil {
		return fmt.Errorf("failed to marshal context: %w", err)
	}

	stepsJSON, err := json.Marshal(thread.Steps)
	if err != nil {
		return fmt.Errorf("failed to marshal steps: %w", err)
	}

	query := `
		INSERT INTO threads (
			id, contract_id, contract_version, owner_id, status, current_step,
			context, steps, started_at, completed_at, error, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			current_step = EXCLUDED.current_step,
			context = EXCLUDED.context,
			steps = EXCLUDED.steps,
			completed_at = EXCLUDED.completed_at,
			error = EXCLUDED.error,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()
	_, err = r.pool.Exec(ctx, query,
		thread.ID,
		thread.ContractID,
		thread.ContractVersion,
		thread.OwnerID,
		string(thread.Status),
		thread.CurrentStep,
		contextJSON,
		stepsJSON,
		thread.StartedAt,
		thread.CompletedAt,
		thread.Error,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	return nil
}

// Get retrieves a thread from PostgreSQL
func (r *ThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, status, current_step,
			   context, steps, started_at, completed_at, error
		FROM threads
		WHERE id = $1
	`

	var thread models.Thread
	var contextJSON, stepsJSON []byte
	var status string

	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&thread.ID,
		&thread.ContractID,
		&thread.ContractVersion,
		&thread.OwnerID,
		&status,
		&thread.CurrentStep,
		&contextJSON,
		&stepsJSON,
		&thread.StartedAt,
		&thread.CompletedAt,
		&thread.Error,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	thread.Status = models.ThreadStatus(status)

	// Deserialize context and steps
	if err := json.Unmarshal(contextJSON, &thread.Context); err != nil {
		return nil, fmt.Errorf("failed to unmarshal context: %w", err)
	}

	if err := json.Unmarshal(stepsJSON, &thread.Steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
	}

	return &thread, nil
}

// GetByOwner retrieves all threads for a given owner
func (r *ThreadRepository) GetByOwner(ctx context.Context, ownerID string, limit, offset int) ([]*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, status, current_step,
			   context, steps, started_at, completed_at, error
		FROM threads
		WHERE owner_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, ownerID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var contextJSON, stepsJSON []byte
		var status string

		err := rows.Scan(
			&thread.ID,
			&thread.ContractID,
			&thread.ContractVersion,
			&thread.OwnerID,
			&status,
			&thread.CurrentStep,
			&contextJSON,
			&stepsJSON,
			&thread.StartedAt,
			&thread.CompletedAt,
			&thread.Error,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		thread.Status = models.ThreadStatus(status)

		// Deserialize context and steps
		if err := json.Unmarshal(contextJSON, &thread.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}

		if err := json.Unmarshal(stepsJSON, &thread.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
		}

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return threads, nil
}

// GetByContract retrieves all threads for a given contract
func (r *ThreadRepository) GetByContract(ctx context.Context, contractID string, limit, offset int) ([]*models.Thread, error) {
	query := `
		SELECT id, contract_id, contract_version, owner_id, status, current_step,
			   context, steps, started_at, completed_at, error
		FROM threads
		WHERE contract_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, query, contractID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads: %w", err)
	}
	defer rows.Close()

	var threads []*models.Thread
	for rows.Next() {
		var thread models.Thread
		var contextJSON, stepsJSON []byte
		var status string

		err := rows.Scan(
			&thread.ID,
			&thread.ContractID,
			&thread.ContractVersion,
			&thread.OwnerID,
			&status,
			&thread.CurrentStep,
			&contextJSON,
			&stepsJSON,
			&thread.StartedAt,
			&thread.CompletedAt,
			&thread.Error,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan thread: %w", err)
		}

		thread.Status = models.ThreadStatus(status)

		// Deserialize context and steps
		if err := json.Unmarshal(contextJSON, &thread.Context); err != nil {
			return nil, fmt.Errorf("failed to unmarshal context: %w", err)
		}

		if err := json.Unmarshal(stepsJSON, &thread.Steps); err != nil {
			return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
		}

		threads = append(threads, &thread)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return threads, nil
}

// Delete removes a thread from PostgreSQL
func (r *ThreadRepository) Delete(ctx context.Context, threadID string) error {
	query := `DELETE FROM threads WHERE id = $1`

	_, err := r.pool.Exec(ctx, query, threadID)
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	return nil
}

// Count returns the total number of threads for an owner
func (r *ThreadRepository) Count(ctx context.Context, ownerID string) (int, error) {
	query := `SELECT COUNT(*) FROM threads WHERE owner_id = $1`

	var count int
	err := r.pool.QueryRow(ctx, query, ownerID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count threads: %w", err)
	}

	return count, nil
}
