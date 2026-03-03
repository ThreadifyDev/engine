package repository

import (
	"database/sql"
	"fmt"
	"time"

	"threadify-go/api/internal/models"
)

type OutboxRepository struct {
	db *sql.DB
}

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

type outboxExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (r *OutboxRepository) Create(event *models.OutboxEvent) error {
	return r.CreateTx(nil, event)
}

func (r *OutboxRepository) CreateTx(tx *sql.Tx, event *models.OutboxEvent) error {
	if tx == nil {
		return r.insert(r.db, event)
	}
	return r.insert(tx, event)
}

func (r *OutboxRepository) insert(execer outboxExecer, event *models.OutboxEvent) error {
	const query = `
		INSERT INTO outbox_events
			(id, type, payload, status, retry_count, max_retries, next_run_at, reference_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
	`
	_, err := execer.Exec(query,
		event.ID, event.Type, event.Payload, event.Status,
		event.RetryCount, event.MaxRetries, event.NextRunAt,
		nullableString(event.ReferenceID),
	)
	if err != nil {
		return fmt.Errorf("insert outbox event: %w", err)
	}
	return nil
}

func (r *OutboxRepository) FetchPendingDue(limit int) ([]*models.OutboxEvent, error) {
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
	rows, err := r.db.Query(query,
		models.OutboxStatusProcessing,
		models.OutboxStatusPending,
		models.OutboxStatusFailed,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []*models.OutboxEvent
	for rows.Next() {
		e := &models.OutboxEvent{}
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

func (r *OutboxRepository) ExistsByReference(eventType, referenceID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE type = $1
			  AND reference_id = $2
			  AND status IN ('pending', 'processing', 'done')
		)
	`
	var exists bool
	err := r.db.QueryRow(query, eventType, referenceID).Scan(&exists)
	return exists, err
}

func (r *OutboxRepository) ExistsPendingByReference(eventType, referenceID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM outbox_events
			WHERE type = $1
			  AND reference_id = $2
			  AND status IN ('pending', 'processing')
		)
	`
	var exists bool
	err := r.db.QueryRow(query, eventType, referenceID).Scan(&exists)
	return exists, err
}

func (r *OutboxRepository) MarkDone(id string) error {
	const query = `
		UPDATE outbox_events
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`
	_, err := r.db.Exec(query, models.OutboxStatusDone, id)
	if err != nil {
		return fmt.Errorf("mark outbox event done: %w", err)
	}
	return nil
}

func (r *OutboxRepository) MarkFailedWithRetry(id, lastErr string, nextRunAt time.Time) error {
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
	_, err := r.db.Exec(query,
		models.OutboxStatusDead,
		models.OutboxStatusPending,
		lastErr,
		nextRunAt,
		id,
	)
	if err != nil {
		return fmt.Errorf("mark outbox event for retry: %w", err)
	}
	return nil
}

func (r *OutboxRepository) PruneProcessed(before time.Time) (int64, error) {
	const query = `DELETE FROM outbox_events WHERE status = $1 AND created_at < $2`
	result, err := r.db.Exec(query, models.OutboxStatusDone, before)
	if err != nil {
		return 0, fmt.Errorf("prune outbox: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
