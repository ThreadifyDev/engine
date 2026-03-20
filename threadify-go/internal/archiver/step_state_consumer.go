package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

type StepStateConsumer struct {
	nc           *nats.Conn
	js           nats.JetStreamContext
	db           *database.PostgresDB
	batchSize    int
	flushTimeout time.Duration
	consumerID   string
	logger       *zap.Logger

	mu     sync.Mutex
	buffer []pendingStepState

	stopOnce sync.Once
	stopChan chan struct{}
	wg       sync.WaitGroup
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

type pendingStepState struct {
	event StepStateEvent
	msg   *nats.Msg
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
		nc:           nc,
		js:           js,
		db:           db,
		batchSize:    batchSize,
		flushTimeout: flushInterval,
		consumerID:   consumerID,
		logger:       logger,
		buffer:       make([]pendingStepState, 0, batchSize),
		stopChan:     make(chan struct{}),
	}, nil
}

func (c *StepStateConsumer) Start(ctx context.Context) error {
	sub, err := c.js.PullSubscribe("state.step", "step-state-archivers", nats.ManualAck())
	if err != nil {
		return fmt.Errorf("subscribe to state.step: %w", err)
	}

	c.logger.Info("started step state consumer", zap.String("consumer", c.consumerID))

	c.wg.Add(1)
	go c.run(ctx, sub)
	return nil
}

func (c *StepStateConsumer) run(ctx context.Context, sub *nats.Subscription) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.flushTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			c.flushAll(context.Background())
			return

		case <-ctx.Done():
			c.flushAll(context.Background())
			return

		case <-ticker.C:
			c.mu.Lock()
			if len(c.buffer) > 0 {
				c.flushLocked(ctx)
			}
			c.mu.Unlock()

		default:
			msgs, err := sub.Fetch(c.batchSize, nats.MaxWait(c.flushTimeout))
			if err != nil {
				if !errors.Is(err, nats.ErrTimeout) {
					c.logger.Error("fetch error",
						zap.String("consumer", c.consumerID),
						zap.Error(err),
					)
					select {
					case <-ctx.Done():
						c.flushAll(context.Background())
						return
					case <-c.stopChan:
						c.flushAll(context.Background())
						return
					case <-time.After(time.Second):
					}
				}
				continue
			}

			c.mu.Lock()
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
				c.buffer = append(c.buffer, pendingStepState{event: event, msg: msg})
			}

			if len(c.buffer) >= c.batchSize {
				c.flushLocked(ctx)
			}
			c.mu.Unlock()
		}
	}
}

func (c *StepStateConsumer) flushAll(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushLocked(ctx)
}

func (c *StepStateConsumer) flushLocked(ctx context.Context) {
	if len(c.buffer) == 0 {
		return
	}

	pending := c.buffer
	c.buffer = make([]pendingStepState, 0, c.batchSize)

	type dedupKey struct{ threadID, stepName, idempKey string }
	seen := make(map[dedupKey]int, len(pending))
	deduped := make([]StepStateEvent, 0, len(pending))
	for _, p := range pending {
		k := dedupKey{p.event.ThreadID, p.event.StepName, p.event.IdempotencyKey}
		if idx, ok := seen[k]; ok {
			deduped[idx] = p.event
		} else {
			seen[k] = len(deduped)
			deduped = append(deduped, p.event)
		}
	}

	c.logger.Info("flushing step states",
		zap.Int("total", len(pending)),
		zap.Int("unique", len(deduped)),
		zap.String("consumer", c.consumerID),
	)

	if err := c.writeBatch(ctx, deduped); err != nil {
		c.logger.Error("flush failed, nacking batch",
			zap.String("consumer", c.consumerID),
			zap.Error(err),
		)
		for _, p := range pending {
			p.msg.Nak()
		}
		return
	}

	for _, p := range pending {
		p.msg.Ack()
	}

	c.logger.Info("wrote step states",
		zap.Int("count", len(pending)),
		zap.String("consumer", c.consumerID),
	)
}

func (c *StepStateConsumer) writeBatch(ctx context.Context, events []StepStateEvent) error {
	if len(events) == 0 {
		return nil
	}

	const numCols = 14
	placeholderRows := make([]string, 0, len(events))
	args := make([]interface{}, 0, len(events)*numCols)

	for i, e := range events {
		base := i * numCols
		cols := make([]string, numCols)
		for j := range cols {
			cols[j] = fmt.Sprintf("$%d", base+j+1)
		}
		placeholderRows = append(placeholderRows, "("+strings.Join(cols, ", ")+", NOW())")

		args = append(args,
			e.StepID,
			e.ThreadID,
			e.StepName,
			e.IdempotencyKey,
			e.Status,
			e.RetryCount,
			nullIfEmpty(e.FirstSeenAt),
			nullIfEmpty(e.LastUpdatedAt),
			nullIfEmpty(e.StartedAt),
			nullIfEmpty(e.FinishedAt),
			e.PreviousStep,
			e.Actor,
			e.ActorService,
			nullIfEmpty(e.LatestContext),
		)
	}

	query := `INSERT INTO thread_step_states (
		id, thread_id, step_name, idempotency_key, status,
		retry_count, first_seen_at, last_updated_at, started_at, finished_at,
		previous_step, actor, actor_service, latest_context, created_at
	) VALUES ` + strings.Join(placeholderRows, ", ") + `
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

	if _, err := c.db.Pool.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert step states: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (c *StepStateConsumer) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
	c.wg.Wait()
	c.logger.Info("stopped step state consumer", zap.String("consumer", c.consumerID))
}
