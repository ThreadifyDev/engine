package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/database"
)

// StepStateConsumer consumes step state events from NATS and writes to Postgres
type StepStateConsumer struct {
	nc          *nats.Conn
	js          nats.JetStreamContext
	db          *database.PostgresDB
	batchSize   int
	flushTicker *time.Ticker
	buffer      []StepStateEvent
	consumerID  string
}

// StepStateEvent represents a step state event from NATS
type StepStateEvent struct {
	StepID         string `json:"step_id"`
	ThreadID       string `json:"thread_id"`
	StepName       string `json:"step_name"`
	IdempotencyKey string `json:"idempotency_key"`
	Status         string `json:"status"`
	RetryCount     int    `json:"retry_count"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastUpdatedAt  string `json:"last_updated_at"`
	PreviousStep   string `json:"previous_step"`
}

// NewStepStateConsumer creates a new step state consumer
func NewStepStateConsumer(
	nc *nats.Conn,
	db *database.PostgresDB,
	batchSize int,
	flushInterval time.Duration,
	consumerID string,
) (*StepStateConsumer, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	consumer := &StepStateConsumer{
		nc:          nc,
		js:          js,
		db:          db,
		batchSize:   batchSize,
		flushTicker: time.NewTicker(flushInterval),
		buffer:      make([]StepStateEvent, 0, batchSize),
		consumerID:  consumerID,
	}

	return consumer, nil
}

// Start begins consuming step state events
func (c *StepStateConsumer) Start(ctx context.Context) error {
	// Subscribe to step state subject
	sub, err := c.js.PullSubscribe("state.step", "step-state-archivers", nats.ManualAck())
	if err != nil {
		return fmt.Errorf("failed to subscribe to state.step: %w", err)
	}

	fmt.Printf("✅ [STEP-STATE-CONSUMER] Started consuming from state.step\n")

	// Process messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Printf("[STEP-STATE-CONSUMER] Context cancelled, stopping...\n")
				c.flush(context.Background())
				return
			case <-c.flushTicker.C:
				if len(c.buffer) > 0 {
					if err := c.flush(ctx); err != nil {
						fmt.Printf("❌ [STEP-STATE-CONSUMER] Flush failed: %v\n", err)
					}
				}
			default:
				// Fetch messages
				msgs, err := sub.Fetch(c.batchSize, nats.MaxWait(5*time.Second))
				if err != nil {
					if err == nats.ErrTimeout {
						// Timeout is normal when no messages available
						continue
					}
					fmt.Printf("❌ [STEP-STATE-CONSUMER] Error fetching messages: %v\n", err)
					time.Sleep(1 * time.Second)
					continue
				}

				for _, msg := range msgs {
					var event StepStateEvent
					if err := json.Unmarshal(msg.Data, &event); err != nil {
						fmt.Printf("❌ [STEP-STATE-CONSUMER] Failed to unmarshal event: %v\n", err)
						msg.Nak()
						continue
					}

					c.buffer = append(c.buffer, event)

					// Flush if buffer is full
					if len(c.buffer) >= c.batchSize {
						if err := c.flush(ctx); err != nil {
							fmt.Printf("❌ [STEP-STATE-CONSUMER] Flush failed: %v\n", err)
							// Nak all messages in buffer
							msg.Nak()
						} else {
							// Ack all messages
							msg.Ack()
						}
					} else {
						// Ack individual message
						msg.Ack()
					}
				}
			}
		}
	}()

	return nil
}

// flush writes buffered events to Postgres
func (c *StepStateConsumer) flush(ctx context.Context) error {
	if len(c.buffer) == 0 {
		return nil
	}

	// Flushing step state events to Postgres

	// Build batch upsert query
	query := `
		INSERT INTO thread_step_states (
			id, thread_id, step_name, idempotency_key, status,
			retry_count, first_seen_at, last_updated_at, previous_step, created_at
		) VALUES `

	values := make([]interface{}, 0, len(c.buffer)*9)
	placeholders := ""

	for i, event := range c.buffer {
		if i > 0 {
			placeholders += ", "
		}

		offset := i * 9
		placeholders += fmt.Sprintf(
			"($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, NOW())",
			offset+1, offset+2, offset+3, offset+4, offset+5,
			offset+6, offset+7, offset+8, offset+9,
		)

		values = append(values,
			event.StepID,
			event.ThreadID,
			event.StepName,
			event.IdempotencyKey,
			event.Status,
			event.RetryCount,
			event.FirstSeenAt,
			event.LastUpdatedAt,
			event.PreviousStep,
		)
	}

	query += placeholders + `
		ON CONFLICT (thread_id, step_name, idempotency_key) DO UPDATE SET
			status = EXCLUDED.status,
			retry_count = EXCLUDED.retry_count,
			last_updated_at = EXCLUDED.last_updated_at,
			previous_step = EXCLUDED.previous_step`

	// Execute batch insert
	_, err := c.db.Pool.Exec(ctx, query, values...)
	if err != nil {
		return fmt.Errorf("failed to insert step states: %w", err)
	}

	fmt.Printf("✅ [STEP-STATE-CONSUMER] Successfully wrote %d step states to Postgres\n", len(c.buffer))

	// Clear buffer
	c.buffer = c.buffer[:0]

	return nil
}

// Stop stops the consumer
func (c *StepStateConsumer) Stop() {
	c.flushTicker.Stop()
	c.flush(context.Background())
	fmt.Printf("[STEP-STATE-CONSUMER] Stopped\n")
}
