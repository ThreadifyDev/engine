package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ThreadRefsRepository handles thread references persistence in PostgreSQL
type ThreadRefsRepository struct {
	pool *pgxpool.Pool
}

// NewThreadRefsRepository creates a new thread refs repository
func NewThreadRefsRepository(pool *pgxpool.Pool) *ThreadRefsRepository {
	return &ThreadRefsRepository{
		pool: pool,
	}
}

// AddRefs inserts or updates refs for a thread (upsert)
func (r *ThreadRefsRepository) AddRefs(ctx context.Context, threadID string, refs map[string]string) error {
	if len(refs) == 0 {
		return nil
	}

	query := `
		INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (thread_id, ref_key) DO UPDATE SET
			ref_value = EXCLUDED.ref_value,
			updated_at = EXCLUDED.updated_at
	`

	now := time.Now()

	// Use a transaction for atomic batch insert
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for key, value := range refs {
		_, err := tx.Exec(ctx, query, threadID, key, value, now, now)
		if err != nil {
			return fmt.Errorf("failed to insert ref %s: %w", key, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRefs retrieves all refs for a thread
func (r *ThreadRefsRepository) GetRefs(ctx context.Context, threadID string) (map[string]string, error) {
	query := `
		SELECT ref_key, ref_value
		FROM thread_refs
		WHERE thread_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query refs: %w", err)
	}
	defer rows.Close()

	refs := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("failed to scan ref: %w", err)
		}
		refs[key] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return refs, nil
}

// GetThreadsByRef finds thread IDs that have a specific ref key-value pair
func (r *ThreadRefsRepository) GetThreadsByRef(ctx context.Context, refKey, refValue string) ([]string, error) {
	query := `
		SELECT DISTINCT thread_id
		FROM thread_refs
		WHERE ref_key = $1 AND ref_value = $2
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, refKey, refValue)
	if err != nil {
		return nil, fmt.Errorf("failed to query threads by ref: %w", err)
	}
	defer rows.Close()

	var threadIDs []string
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return nil, fmt.Errorf("failed to scan thread ID: %w", err)
		}
		threadIDs = append(threadIDs, threadID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return threadIDs, nil
}

// DeleteRef removes a specific ref from a thread
func (r *ThreadRefsRepository) DeleteRef(ctx context.Context, threadID, refKey string) error {
	query := `DELETE FROM thread_refs WHERE thread_id = $1 AND ref_key = $2`

	_, err := r.pool.Exec(ctx, query, threadID, refKey)
	if err != nil {
		return fmt.Errorf("failed to delete ref: %w", err)
	}

	return nil
}

// DeleteAllRefs removes all refs for a thread
func (r *ThreadRefsRepository) DeleteAllRefs(ctx context.Context, threadID string) error {
	query := `DELETE FROM thread_refs WHERE thread_id = $1`

	_, err := r.pool.Exec(ctx, query, threadID)
	if err != nil {
		return fmt.Errorf("failed to delete all refs: %w", err)
	}

	return nil
}
