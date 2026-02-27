package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

type StepStateConsumer struct {
	nc         *nats.Conn
	js         nats.JetStreamContext
	db         *database.PostgresDB
	batchSize  int
	ticker     *time.Ticker
	consumerID string
	logger     *zap.Logger
}

type StepStateEvent struct {
	StepID         string `json:"stepId"`
	ThreadID       string `json:"threadId"`
	StepName       string `json:"stepName"`
	IdempotencyKey string `json:"idempotencyKey"`
	Status         string `json:"status"`
	RetryCount     int    `json:"retryCount"`
	FirstSeenAt    string `json:"firstSeenAt"`
	LastUpdatedAt  string `json:"lastUpdatedAt"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	PreviousStep   string `json:"previousStep"`
	Actor          string `json:"actor"`
	ActorService   string `json:"actorService"`
	LatestContext  string `json:"latestContext"`
}

// parseTS parses an RFC3339 string into a *time.Time.
// Empty string → nil → NULL in DB. Invalid format → error surfaced to caller.
func parseTS(s string) (interface{}, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		// Fall back to RFC3339 without nanoseconds
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", s, err)
		}
	}
	return t, nil
}

func NewStepStateConsumer(
	nc *nats.Conn,
	db *database.PostgresDB,
	batchSize int,
	flushInterval time.Duration,
	consumerID string,
	logger *zap.Logger,
) (*StepStateConsumer, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}
	return &StepStateConsumer{
		nc:         nc,
		js:         js,
		db:         db,
		batchSize:  batchSize,
		ticker:     time.NewTicker(flushInterval),
		consumerID: consumerID,
		logger:     logger,
	}, nil
}

func (c *StepStateConsumer) Start(ctx context.Context) error {
	sub, err := c.js.PullSubscribe("state.step", "step-state-archivers", nats.ManualAck())
	if err != nil {
		return fmt.Errorf("subscribe to state.step: %w", err)
	}

	c.logger.Info("started step state consumer", zap.String("consumer", c.consumerID))

	var buffer []StepStateEvent
	var pending []*nats.Msg // held until flush succeeds

	flush := func(fCtx context.Context) {
		if len(buffer) == 0 {
			return
		}
		if err := c.writeBatch(fCtx, buffer); err != nil {
			c.logger.Error("flush failed",
				zap.String("consumer", c.consumerID),
				zap.Error(err),
			)
			for _, m := range pending {
				m.Nak()
			}
		} else {
			for _, m := range pending {
				m.Ack()
			}
		}
		buffer = buffer[:0]
		pending = pending[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush(context.Background())
			return nil
		case <-c.ticker.C:
			flush(context.Background())
		default:
			msgs, err := sub.Fetch(c.batchSize, nats.MaxWait(5*time.Second))
			if err != nil {
				if !errors.Is(err, nats.ErrTimeout) {
					c.logger.Error("fetch error",
						zap.String("consumer", c.consumerID),
						zap.Error(err),
					)
					time.Sleep(time.Second)
				}
				continue
			}

			for _, msg := range msgs {
				var event StepStateEvent
				if err := json.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Error("unmarshal error",
						zap.String("consumer", c.consumerID),
						zap.Error(err),
					)
					msg.Nak()
					continue
				}
				buffer = append(buffer, event)
				pending = append(pending, msg)
			}

			if len(buffer) >= c.batchSize {
				flush(ctx)
			}
		}
	}
}

func (c *StepStateConsumer) Stop() {
	c.ticker.Stop()
	c.logger.Info("stopped step state consumer", zap.String("consumer", c.consumerID))
}

func (c *StepStateConsumer) writeBatch(ctx context.Context, events []StepStateEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Last write for a given (thread, step, idempotency_key) wins.
	unique := make(map[string]*StepStateEvent, len(events))
	for i := range events {
		e := &events[i]
		unique[e.ThreadID+":"+e.StepName+":"+e.IdempotencyKey] = e
	}
	deduped := make([]*StepStateEvent, 0, len(unique))
	for _, e := range unique {
		deduped = append(deduped, e)
	}

	const cols = 14
	values := make([]interface{}, 0, len(deduped)*cols)
	for _, e := range deduped {
		firstSeenAt, err := parseTS(e.FirstSeenAt)
		if err != nil {
			return fmt.Errorf("step %s: %w", e.StepID, err)
		}

		if firstSeenAt == nil {
			c.logger.Warn("first_seen_at is nil", zap.Any("event", e))
			firstSeenAt = time.Now()
		}
		lastUpdatedAt, err := parseTS(e.LastUpdatedAt)
		if err != nil {
			return fmt.Errorf("step %s: %w", e.StepID, err)
		}
		startedAt, err := parseTS(e.StartedAt)
		if err != nil {
			return fmt.Errorf("step %s: %w", e.StepID, err)
		}
		finishedAt, err := parseTS(e.FinishedAt)
		if err != nil {
			return fmt.Errorf("step %s: %w", e.StepID, err)
		}

		values = append(values,
			e.StepID,
			e.ThreadID,
			e.StepName,
			e.IdempotencyKey,
			e.Status,
			e.RetryCount,
			firstSeenAt,
			lastUpdatedAt,
			startedAt,
			finishedAt,
			nullIfEmpty(e.PreviousStep),
			nullIfEmpty(e.Actor),
			nullIfEmpty(e.ActorService),
			nullIfEmpty(e.LatestContext),
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context
	) VALUES ` + buildPlaceholders(len(deduped), cols) + `
	ON CONFLICT (thread_id, step_name, idempotency_key) DO UPDATE SET
		status          = EXCLUDED.status,
		retry_count     = EXCLUDED.retry_count,
		last_updated_at = EXCLUDED.last_updated_at,
		started_at      = EXCLUDED.started_at,
		finished_at     = EXCLUDED.finished_at,
		previous_step   = EXCLUDED.previous_step,
		actor           = EXCLUDED.actor,
		actor_service   = EXCLUDED.actor_service,
		latest_context  = EXCLUDED.latest_context`
	// created_at omitted: DEFAULT NOW() on first insert, never overwritten on conflict.

	if _, err := c.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("insert step states: %w", err)
	}

	c.logger.Info("wrote step states",
		zap.Int("total", len(events)),
		zap.Int("unique", len(deduped)),
		zap.String("consumer", c.consumerID),
	)
	return nil
}
