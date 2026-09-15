package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

type StepStateConsumer struct {
	js           JetStreamPublisher
	db           DBExecer
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

type stepStateEvent struct {
	SuccessOrder   string `json:"successOrder"`
	SuccessContext string `json:"successContext"`
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
	event stepStateEvent
	msg   jetstream.Msg
}

func NewStepStateConsumer(
	js JetStreamPublisher,
	db DBExecer,
	batchSize int,
	flushInterval time.Duration,
	consumerID string,
	logger *zap.Logger,
) (*StepStateConsumer, error) {
	return &StepStateConsumer{
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
	cons, err := c.js.CreateOrUpdateConsumer(ctx, natsrepo.StreamStepState, jetstream.ConsumerConfig{
		Durable:       "step-state-archivers",
		FilterSubject: natsrepo.SubjectStepState,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	c.logger.Info("started step state consumer", zap.String("consumer", c.consumerID))

	c.wg.Add(1)
	go c.run(ctx, cons)
	return nil
}

func (c *StepStateConsumer) run(ctx context.Context, cons jetstream.Consumer) {
	defer c.wg.Done()

	consumeCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var event stepStateEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			c.logger.Error("unmarshal error",
				zap.String("consumer", c.consumerID),
				zap.Error(err),
			)
			msg.Nak()
			return
		}

		c.mu.Lock()
		c.buffer = append(c.buffer, pendingStepState{event: event, msg: msg})
		if len(c.buffer) >= c.batchSize {
			c.flushLocked(ctx)
		}
		c.mu.Unlock()
	}, jetstream.ConsumeErrHandler(func(consumeCtx jetstream.ConsumeContext, err error) {
		c.logger.Error("consume error", zap.Error(err))
	}))
	if err != nil {
		c.logger.Error("failed to start consume", zap.Error(err))
		return
	}

	ticker := time.NewTicker(c.flushTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			consumeCtx.Stop()
			c.flushAll(context.Background())
			return

		case <-ctx.Done():
			consumeCtx.Stop()
			c.flushAll(context.Background())
			return

		case <-ticker.C:
			c.flushAll(ctx)

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

	events := make([]stepStateEvent, 0, len(pending))
	for _, p := range pending {
		events = append(events, p.event)
	}

	c.logger.Info("flushing step states",
		zap.Int("total", len(pending)),
		zap.Int("events", len(events)),
		zap.String("consumer", c.consumerID),
	)

	if err := c.writeBatch(ctx, events); err != nil {
		c.logger.Error("flush failed, nacking batch",
			zap.String("consumer", c.consumerID),
			zap.Error(err),
		)
		for _, p := range pending {
			p.msg.NakWithDelay(5 * time.Second)
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

// processMessages is used by Runtime, whose worker owns batching and acknowledgements.
func (c *StepStateConsumer) processMessages(ctx context.Context, msgs []jetstream.Msg) error {
	events := make([]stepStateEvent, 0, len(msgs))
	for _, msg := range msgs {
		var event stepStateEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return fmt.Errorf("decode step state: %w", err)
		}
		events = append(events, event)
	}

	return c.writeBatch(ctx, events)
}

func (c *StepStateConsumer) writeBatch(ctx context.Context, events []stepStateEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Persist successful attempts before mutable state deduplication can discard
	// a success followed by a failed retry in the same message batch.
	successes := make([]StreamEvent, 0, len(events))
	for _, e := range events {
		if e.SuccessOrder != "" {
			successes = append(successes, StreamEvent{Data: map[string]string{
				"threadId": e.ThreadID, "stepName": e.StepName, "stepId": e.StepID, "status": e.Status, "successOrder": e.SuccessOrder, "successContext": e.SuccessContext,
			}})
		}
	}
	if err := NewPostgresWriter(c.db, c.logger).writeSuccessfulContexts(ctx, successes); err != nil {
		return err
	}
	type key struct{ thread, step, idempotency string }
	seen := map[key]int{}
	deduped := make([]stepStateEvent, 0, len(events))
	for _, e := range events {
		k := key{e.ThreadID, e.StepName, e.IdempotencyKey}
		if i, ok := seen[k]; ok {
			deduped[i] = e
		} else {
			seen[k] = len(deduped)
			deduped = append(deduped, e)
		}
	}
	events = deduped

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

	if _, err := c.db.Exec(ctx, query, args...); err != nil {
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
