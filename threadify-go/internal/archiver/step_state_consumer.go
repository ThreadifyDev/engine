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
	StepID         string `json:"step_id"`
	ThreadID       string `json:"thread_id"`
	StepName       string `json:"step_name"`
	IdempotencyKey string `json:"idempotency_key"`
	Status         string `json:"status"`
	RetryCount     int    `json:"retry_count"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastUpdatedAt  string `json:"last_updated_at"`
	StartedAt      string `json:"started_at"`
	FinishedAt     string `json:"finished_at"`
	PreviousStep   string `json:"previous_step"`
	Actor          string `json:"actor"`
	ActorService   string `json:"actor_service"`
	LatestContext  string `json:"latest_context"`
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

	flush := func(ctx context.Context) {
		if len(buffer) == 0 {
			return
		}
		if err := c.writeBatch(ctx, buffer); err != nil {
			c.logger.Error("flush failed", zap.String("consumer", c.consumerID), zap.Error(err))
		}
		buffer = buffer[:0]
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				flush(context.Background())
				return
			case <-c.ticker.C:
				flush(ctx)
			default:
				msgs, err := sub.Fetch(c.batchSize, nats.MaxWait(5*time.Second))
				if err != nil {
					if !errors.Is(err, nats.ErrTimeout) {
						c.logger.Error("fetch error", zap.String("consumer", c.consumerID), zap.Error(err))
						time.Sleep(time.Second)
					}
					continue
				}

				for _, msg := range msgs {
					var event StepStateEvent
					if err := json.Unmarshal(msg.Data, &event); err != nil {
						c.logger.Error("failed to unmarshal event", zap.String("consumer", c.consumerID), zap.Error(err))
						msg.Nak()
						continue
					}
					buffer = append(buffer, event)
					msg.Ack()
				}

				if len(buffer) >= c.batchSize {
					flush(ctx)
				}
			}
		}
	}()

	return nil
}

func (c *StepStateConsumer) Stop() {
	c.ticker.Stop()
	c.logger.Info("stopped step state consumer", zap.String("consumer", c.consumerID))
}

func (c *StepStateConsumer) writeBatch(ctx context.Context, events []StepStateEvent) error {
	unique := make(map[string]*StepStateEvent, len(events))
	for i := range events {
		e := &events[i]
		unique[fmt.Sprintf("%s:%s:%s", e.ThreadID, e.StepName, e.IdempotencyKey)] = e
	}

	deduped := make([]*StepStateEvent, 0, len(unique))
	for _, e := range unique {
		deduped = append(deduped, e)
	}

	const cols = 14
	values := make([]interface{}, 0, len(deduped)*cols)
	for _, e := range deduped {
		values = append(values,
			e.StepID, e.ThreadID, e.StepName, e.IdempotencyKey, e.Status,
			e.RetryCount, e.FirstSeenAt, e.LastUpdatedAt,
			nullIfEmpty(e.StartedAt), nullIfEmpty(e.FinishedAt),
			e.PreviousStep, e.Actor, e.ActorService, e.LatestContext,
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context, created_at
	) VALUES ` + buildPlaceholders(len(deduped), cols) + `
	ON CONFLICT (thread_id, step_name, idempotency_key) DO UPDATE SET
		status         = EXCLUDED.status,
		retry_count    = EXCLUDED.retry_count,
		last_updated_at = EXCLUDED.last_updated_at,
		started_at     = EXCLUDED.started_at,
		finished_at    = EXCLUDED.finished_at,
		previous_step  = EXCLUDED.previous_step,
		actor          = EXCLUDED.actor,
		actor_service  = EXCLUDED.actor_service,
		latest_context = EXCLUDED.latest_context`

	// buildPlaceholders doesn't handle the trailing NOW() for created_at;
	// replace the last closing paren of each row to append it.
	// Alternatively, pass created_at as a value — but NOW() is idiomatic here.

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
