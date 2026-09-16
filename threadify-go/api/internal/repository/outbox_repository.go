package repository

import (
	"context"
	"fmt"
	"time"

	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

type outboxRepository struct {
	pool *pgxpool.Pool
}

func NewOutboxRepository(pool *pgxpool.Pool) domain.OutboxRepository {
	return &outboxRepository{pool: pool}
}

func (r *outboxRepository) Create(ctx context.Context, event *domain.OutboxEvent) error {
	return r.CreateTx(ctx, r.pool, event)
}

func (r *outboxRepository) CreateTx(ctx context.Context, tx domain.ExecContext, event *domain.OutboxEvent) error {
	return r.insert(ctx, tx, event)
}

func (r *outboxRepository) insert(ctx context.Context, tx domain.ExecContext, event *domain.OutboxEvent) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}
	const query = `
		INSERT INTO outbox_events
			(id, type, payload, status, retry_count, max_retries, next_run_at, reference_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := execer.Exec(ctx, query,
		event.ID, event.Type, event.Payload, event.Status,
		event.RetryCount, event.MaxRetries, event.NextRunAt,
		nullableString(event.ReferenceID),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func (r *outboxRepository) FetchPendingDue(ctx context.Context, limit int) ([]*domain.OutboxEvent, error) {
	const query = `
		UPDATE outbox_events
		SET status = $1, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE status IN ($2, $3)
			  AND next_run_at <= NOW()
			  AND retry_count < max_retries
			ORDER BY next_run_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $4
		)
		RETURNING id, type, payload, status, retry_count, max_retries,
		          next_run_at, error_log, reference_id, created_at, updated_at
	`
	rows, err := r.pool.Query(ctx, query,
		domain.OutboxStatusProcessing,
		domain.OutboxStatusPending,
		domain.OutboxStatusFailed,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []*domain.OutboxEvent
	for rows.Next() {
		e := &domain.OutboxEvent{}
		if err := rows.Scan(
			&e.ID, &e.Type, &e.Payload, &e.Status,
			&e.RetryCount, &e.MaxRetries, &e.NextRunAt,
			&e.LastError, &e.ReferenceID, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *outboxRepository) ExistsByReference(ctx context.Context, eventType, referenceID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE type = $1
			  AND reference_id = $2
			  AND status IN ('pending', 'processing', 'done')
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, eventType, referenceID).Scan(&exists)
	return exists, err
}

func (r *outboxRepository) ExistsPendingByReference(ctx context.Context, eventType, referenceID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE type = $1
			  AND reference_id = $2
			  AND status IN ('pending', 'processing')
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, eventType, referenceID).Scan(&exists)
	return exists, err
}

func (r *outboxRepository) MarkDone(ctx context.Context, id string) error {
	const query = `
		UPDATE outbox_events
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.pool.Exec(ctx, query, domain.OutboxStatusDone, id)
	if err != nil {
		return fmt.Errorf("mark outbox event done: %w", err)
	}
	return nil
}

func (r *outboxRepository) MarkFailedWithRetry(ctx context.Context, id, lastErr string, nextRunAt time.Time) error {
	const query = `
		UPDATE outbox_events
		SET status      = CASE WHEN retry_count + 1 >= max_retries
		                     THEN $1
		                     ELSE $2
		                 END,
		    retry_count = retry_count + 1,
		    error_log   = $3,
		    next_run_at = $4,
		    updated_at  = NOW()
		WHERE id = $5
	`
	_, err := r.pool.Exec(ctx, query,
		domain.OutboxStatusDead,
		domain.OutboxStatusPending,
		lastErr,
		nextRunAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event for retry: %w", err)
	}
	return nil
}

func (r *outboxRepository) PruneProcessed(ctx context.Context, before time.Time) (int64, error) {
	const query = `DELETE FROM outbox_events WHERE status = $1 AND created_at < $2`
	result, err := r.pool.Exec(ctx, query, domain.OutboxStatusDone, before)
	if err != nil {
		return 0, fmt.Errorf("prune outbox: %w", err)
	}
	return result.RowsAffected(), nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
