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
	nc          *nats.Conn
	js          nats.JetStreamContext
	db          *database.PostgresDB
	batchSize   int
	flushTicker *time.Ticker
	buffer      []StepStateEvent
	consumerID  string
	logger      *zap.Logger
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

// NewStepStateConsumer creates a new step state consumer.
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
		nc:          nc,
		js:          js,
		db:          db,
		batchSize:   batchSize,
		flushTicker: time.NewTicker(flushInterval),
		buffer:      make([]StepStateEvent, 0, batchSize),
		consumerID:  consumerID,
		logger:      logger,
	}, nil
}

// Start begins consuming step state events in a background goroutine.
func (c *StepStateConsumer) Start(ctx context.Context) error {
	sub, err := c.js.PullSubscribe("state.step", "step-state-archivers", nats.ManualAck())
	if err != nil {
		return fmt.Errorf("subscribe to state.step: %w", err)
	}

	c.logger.Info("started step state consumer", zap.String("consumer", c.consumerID))

	go func() {
		for {
			select {
			case <-ctx.Done():
				c.logger.Info("context cancelled, stopping", zap.String("consumer", c.consumerID))
				c.flush(context.Background())
				return

			case <-c.flushTicker.C:
				if len(c.buffer) > 0 {
					if err := c.flush(ctx); err != nil {
						c.logger.Error("flush failed",
							zap.String("consumer", c.consumerID),
							zap.Error(err),
						)
					}
				}

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

					c.buffer = append(c.buffer, event)

					// Flush if buffer is full.
					if len(c.buffer) >= c.batchSize {
						if err := c.flush(ctx); err != nil {
							c.logger.Error("flush failed",
								zap.String("consumer", c.consumerID),
								zap.Error(err),
							)
							msg.Nak()
						} else {
							msg.Ack()
						}
					} else {
						msg.Ack()
					}
				}
			}
		}
	}()

	return nil
}

// flush writes buffered events to Postgres and clears the buffer.
func (c *StepStateConsumer) flush(ctx context.Context) error {
	if len(c.buffer) == 0 {
		return nil
	}

	// Deduplicate — last write for a given (thread_id, step_name, idempotency_key) wins.
	uniqueEvents := make(map[string]*StepStateEvent)
	for i := range c.buffer {
		event := &c.buffer[i]
		key := fmt.Sprintf("%s:%s:%s", event.ThreadID, event.StepName, event.IdempotencyKey)
		uniqueEvents[key] = event
	}

	deduplicatedEvents := make([]*StepStateEvent, 0, len(uniqueEvents))
	for _, event := range uniqueEvents {
		deduplicatedEvents = append(deduplicatedEvents, event)
	}

	c.logger.Info("flushing step states",
		zap.Int("total", len(c.buffer)),
		zap.Int("unique", len(deduplicatedEvents)),
		zap.String("consumer", c.consumerID),
	)

	// Build batch upsert query.
	// created_at uses NOW() inline — set on first insert, never overwritten on conflict.
	const cols = 14
	values := make([]interface{}, 0, len(deduplicatedEvents)*cols)
	placeholders := make([]string, 0, len(deduplicatedEvents))

	for i, event := range deduplicatedEvents {
		offset := i * cols
		placeholders = append(placeholders, fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW())",
			offset+1, offset+2, offset+3, offset+4, offset+5,
			offset+6, offset+7, offset+8, offset+9, offset+10,
			offset+11, offset+12, offset+13, offset+14,
		))

		var startedAtVal interface{}
		if event.StartedAt != "" {
			startedAtVal = event.StartedAt
		}
		var finishedAtVal interface{}
		if event.FinishedAt != "" {
			finishedAtVal = event.FinishedAt
		}

		values = append(values,
			event.StepID,
			event.ThreadID,
			event.StepName,
			event.IdempotencyKey,
			event.Status,
			event.RetryCount,
			event.FirstSeenAt,
			event.LastUpdatedAt,
			startedAtVal,
			finishedAtVal,
			event.PreviousStep,
			event.Actor,
			event.ActorService,
			event.LatestContext,
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context, created_at
	) VALUES ` + joinStrings(placeholders) + `
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

	if _, err := c.db.Pool.Exec(ctx, query, values...); err != nil {
		return fmt.Errorf("insert step states: %w", err)
	}

	c.logger.Info("wrote step states",
		zap.Int("count", len(c.buffer)),
		zap.String("consumer", c.consumerID),
	)

	// Clear buffer.
	c.buffer = c.buffer[:0]
	return nil
}

// Stop stops the consumer, flushing any remaining buffered events.
func (c *StepStateConsumer) Stop() {
	c.flushTicker.Stop()
	c.flush(context.Background())
	c.logger.Info("stopped step state consumer", zap.String("consumer", c.consumerID))
}

// joinStrings joins a slice of strings with ", ".
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
