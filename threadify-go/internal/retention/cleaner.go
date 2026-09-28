package retention

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const batchSize = 100

// Cleaner removes terminal threads for this Engine's single company. A zero
// retention period disables deletion and preserves historical data indefinitely.
type Cleaner struct {
	pool      *pgxpool.Pool
	cache     *redis.Client
	companyID string
	logger    *zap.Logger
	stop      chan struct{}
	done      chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
}

func NewCleaner(pool *pgxpool.Pool, cache *redis.Client, companyID string, logger *zap.Logger) *Cleaner {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Cleaner{pool: pool, cache: cache, companyID: companyID, logger: logger, stop: make(chan struct{}), done: make(chan struct{}), ctx: ctx, cancel: cancel}
}

func (c *Cleaner) Start() {
	c.startOnce.Do(func() { go c.run() })
}

func (c *Cleaner) Stop(ctx context.Context) error {
	c.Start()
	c.stopOnce.Do(func() { c.cancel(); close(c.stop) })
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Cleaner) run() {
	defer close(c.done)
	run := func() {
		ctx, cancel := context.WithTimeout(c.ctx, 5*time.Minute)
		defer cancel()
		for i := 0; i < 10; i++ {
			deleted, err := c.Sweep(ctx, time.Now().UTC())
			if err != nil {
				c.logger.Error("thread retention sweep failed", zap.Error(err))
				return
			}
			if deleted == 0 {
				return
			}
			c.logger.Info("removed expired terminal threads", zap.Int("count", deleted))
			if deleted < batchSize {
				return
			}
		}
	}
	run()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			run()
		}
	}
}

// Sweep removes at most one batch. Valkey keys are removed before PostgreSQL
// commits; on a failure the database remains available as the source of truth.
func (c *Cleaner) Sweep(ctx context.Context, now time.Time) (int, error) {
	if c.pool == nil || c.cache == nil || c.companyID == "" {
		return 0, fmt.Errorf("invalid thread retention configuration")
	}
	var days int
	if err := c.pool.QueryRow(ctx, `SELECT thread_retention_days FROM companies WHERE id=$1`, c.companyID).Scan(&days); err != nil {
		return 0, fmt.Errorf("read company thread retention: %w", err)
	}
	if days == 0 {
		return 0, nil
	}
	if days < 0 || days > 36500 {
		return 0, fmt.Errorf("invalid company thread retention days: %d", days)
	}
	cutoff := now.UTC().AddDate(0, 0, -days)
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin thread retention sweep: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	rows, err := tx.Query(ctx, `SELECT id FROM threads
		WHERE company_id=$1 AND status IN ('completed','cancelled','closed','failed')
		AND COALESCE(terminal_archived_at,updated_at) < $2
		ORDER BY COALESCE(terminal_archived_at,updated_at),id LIMIT $3 FOR UPDATE SKIP LOCKED`, c.companyID, cutoff, batchSize)
	if err != nil {
		return 0, fmt.Errorf("select expired threads: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := c.purgeCache(ctx, ids); err != nil {
		return 0, fmt.Errorf("purge retained thread cache: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO thread_retention_tombstones(thread_id,company_id)
		SELECT unnest($1::text[]),$2 ON CONFLICT(thread_id) DO NOTHING`, ids, c.companyID)
	if err != nil {
		return 0, fmt.Errorf("record retained thread deletion: %w", err)
	}
	for _, table := range []string{"thread_validations", "thread_notifications", "thread_step_states", "step_substeps"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE thread_id=ANY($1::text[])", ids); err != nil {
			return 0, fmt.Errorf("delete %s: %w", table, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM threads WHERE id=ANY($1::text[]) AND company_id=$2`, ids, c.companyID); err != nil {
		return 0, fmt.Errorf("delete expired threads: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit thread retention sweep: %w", err)
	}
	return len(ids), nil
}

func (c *Cleaner) purgeCache(ctx context.Context, ids []string) error {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	var cursor uint64
	remove := make(map[string]bool)
	for {
		keys, next, err := c.cache.Scan(ctx, cursor, "thread:*", 1000).Result()
		if err != nil {
			return err
		}
		for _, key := range keys {
			id, _, ok := strings.Cut(strings.TrimPrefix(key, "thread:"), ":")
			if !ok {
				id = strings.TrimPrefix(key, "thread:")
			}
			if wanted[id] {
				remove[key] = true
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	keys := make([]string, 0, len(remove))
	for key := range remove {
		keys = append(keys, key)
		if len(keys) == 500 {
			if err := c.cache.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if len(keys) > 0 {
		return c.cache.Unlink(ctx, keys...).Err()
	}
	return nil
}
