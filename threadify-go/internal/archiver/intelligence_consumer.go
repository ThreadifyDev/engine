package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

const (
	defaultQueryTimeout    = 15 * time.Second
	maxConsecutiveFailures = 5
	initialBackoff         = 1 * time.Second
	maxBackoff             = 30 * time.Second
	shutdownFlushTimeout   = 30 * time.Second
)

type IntelligenceConsumer struct {
	nc           *nats.Conn
	js           nats.JetStreamContext
	db           *database.PostgresDB
	batchSize    int
	flushTimeout time.Duration
	consumerID   string
	logger       *zap.Logger

	mu     sync.Mutex
	buffer []pendingIntelEvent

	consecutiveFailures atomic.Int32

	stopOnce    sync.Once
	stopChan    chan struct{}
	shutdownCtx context.Context
	shutdownFn  context.CancelFunc
	wg          sync.WaitGroup
}

type pendingIntelEvent struct {
	threadID  string
	eventType string
	msg       *nats.Msg
}

func NewIntelligenceConsumer(
	nc *nats.Conn,
	db *database.PostgresDB,
	batchSize int,
	flushInterval time.Duration,
	consumerID string,
	logger *zap.Logger,
) (*IntelligenceConsumer, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}

	shutdownCtx, shutdownFn := context.WithCancel(context.Background())

	return &IntelligenceConsumer{
		nc:           nc,
		js:           js,
		db:           db,
		batchSize:    batchSize,
		flushTimeout: flushInterval,
		consumerID:   consumerID,
		logger:       logger,
		buffer:       make([]pendingIntelEvent, 0, batchSize),
		stopChan:     make(chan struct{}),
		shutdownCtx:  shutdownCtx,
		shutdownFn:   shutdownFn,
	}, nil
}

func (c *IntelligenceConsumer) Start(ctx context.Context) error {
	sub, err := c.js.PullSubscribe(
		"metadata.thread",
		"intelligence-metrics",
		nats.ManualAck(),
		nats.AckExplicit(),
	)
	if err != nil {
		return fmt.Errorf("subscribe to metadata.thread for intelligence: %w", err)
	}

	c.logger.Info("started intelligence consumer", zap.String("consumer", c.consumerID))

	c.wg.Add(1)
	go c.run(ctx, sub)
	return nil
}

func (c *IntelligenceConsumer) run(ctx context.Context, sub *nats.Subscription) {
	defer c.wg.Done()

	lastFlush := time.Now()

	for {
		select {
		case <-c.stopChan:
			c.flushAll()
			return
		case <-ctx.Done():
			c.flushAll()
			return
		default:
		}

		if failures := c.consecutiveFailures.Load(); failures >= maxConsecutiveFailures {
			shift := int(failures - maxConsecutiveFailures)
			if shift > 4 {
				shift = 4
			}
			backoff := initialBackoff * time.Duration(1<<shift)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			c.logger.Warn("intelligence consumer circuit breaker active, backing off",
				zap.Int32("consecutive_failures", failures),
				zap.Duration("backoff", backoff),
			)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			case <-c.stopChan:
				return
			}
			continue
		}

		msgs, err := sub.Fetch(c.batchSize, nats.MaxWait(c.flushTimeout))
		if err != nil {
			if !errors.Is(err, nats.ErrTimeout) {
				c.logger.Error("intelligence consumer fetch error",
					zap.String("consumer", c.consumerID),
					zap.Error(err),
				)
				select {
				case <-ctx.Done():
					c.flushAll()
					return
				case <-c.stopChan:
					c.flushAll()
					return
				case <-time.After(time.Second):
				}
			}
			// On timeout or transient error, fall through to flush check below.
		}

		c.mu.Lock()
		for _, msg := range msgs {
			var data map[string]interface{}
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				c.logger.Error("intelligence consumer unmarshal error", zap.Error(err))
				msg.Nak()
				continue
			}

			threadID, _ := data["threadId"].(string)
			if threadID == "" {
				msg.Ack()
				continue
			}

			action, _ := data["action"].(string)
			status, _ := data["status"].(string)

			var eventType string
			switch {
			case action == "ref_added":
				eventType = "ref_added"
			case status == "completed":
				eventType = "completed"
			case status == "cancelled":
				eventType = "cancelled"
			default:
				msg.Ack()
				continue
			}

			c.buffer = append(c.buffer, pendingIntelEvent{
				threadID:  threadID,
				eventType: eventType,
				msg:       msg,
			})
		}

		shouldFlush := len(c.buffer) >= c.batchSize || time.Since(lastFlush) >= c.flushTimeout
		if shouldFlush && len(c.buffer) > 0 {
			c.flushLocked(ctx)
			lastFlush = time.Now()
		}
		c.mu.Unlock()
	}
}

func (c *IntelligenceConsumer) flushAll() {
	ctx, cancel := context.WithTimeout(c.shutdownCtx, shutdownFlushTimeout)
	defer cancel()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.flushLocked(ctx)
}

func (c *IntelligenceConsumer) flushLocked(ctx context.Context) {
	if len(c.buffer) == 0 {
		return
	}

	pending := c.buffer
	c.buffer = make([]pendingIntelEvent, 0, c.batchSize)

	seen := make(map[string]bool, len(pending))
	threadIDs := make([]string, 0, len(pending))
	for _, p := range pending {
		if !seen[p.threadID] {
			seen[p.threadID] = true
			threadIDs = append(threadIDs, p.threadID)
		}
	}

	var startSeq, endSeq uint64
	if len(pending) > 0 {
		if meta, err := pending[0].msg.Metadata(); err == nil {
			startSeq = meta.Sequence.Stream
		}
		if meta, err := pending[len(pending)-1].msg.Metadata(); err == nil {
			endSeq = meta.Sequence.Stream
		}
	}

	c.logger.Info("recalculating entity profile metrics",
		zap.Int("events", len(pending)),
		zap.Int("unique_threads", len(threadIDs)),
		zap.Uint64("start_seq", startSeq),
		zap.Uint64("end_seq", endSeq),
		zap.String("consumer", c.consumerID),
	)

	if err := c.recalculateMetrics(ctx, threadIDs); err != nil {
		c.consecutiveFailures.Add(1)
		c.logger.Error("failed to recalculate entity profile metrics",
			zap.String("consumer", c.consumerID),
			zap.Int32("consecutive_failures", c.consecutiveFailures.Load()),
			zap.Error(err),
		)
		for _, p := range pending {
			p.msg.NakWithDelay(2 * time.Second)
		}
		return
	}

	c.consecutiveFailures.Store(0)

	var ackErrors int
	for _, p := range pending {
		if err := p.msg.AckSync(); err != nil {
			ackErrors++
			c.logger.Error("failed to sync ACK message",
				zap.Uint64("seq", startSeq),
				zap.Error(err),
			)
		}
	}

	if ackErrors > 0 {
		c.logger.Warn("batch processed but some ACKs failed",
			zap.Int("failed_acks", ackErrors),
			zap.Int("total_acks", len(pending)),
		)
	}

	c.logger.Info("entity profile metrics updated",
		zap.Int("threads_processed", len(threadIDs)),
		zap.Int("acks_sent", len(pending)-ackErrors),
		zap.String("consumer", c.consumerID),
	)
}

func (c *IntelligenceConsumer) recalculateMetrics(ctx context.Context, threadIDs []string) error {
	if len(threadIDs) == 0 {
		return nil
	}

	if err := c.recalculateProfileMetrics(ctx, threadIDs); err != nil {
		return err
	}

	if err := c.recalculatePartnerCompatibility(ctx, threadIDs); err != nil {
		c.logger.Warn("failed to update partner compatibility (non-fatal)", zap.Error(err))
	}

	return nil
}

func (c *IntelligenceConsumer) recalculateProfileMetrics(ctx context.Context, threadIDs []string) error {
	queryCtx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	query := `
		INSERT INTO entity_profile_metrics (
			entity_profile_id, total_deliveries, completed_successfully,
			validation_violations, delivery_health_score,
			prev_delivery_health_score, health_trend_slope,
			average_delivery_time_ms, last_calculated_at
		)
		SELECT
			ep.id,
			COUNT(DISTINCT all_refs.thread_id),
			COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.id END),
			COUNT(DISTINCT CASE WHEN tv.overall_status = 'violated' THEN tv.validation_id END),
			CASE
				WHEN COUNT(DISTINCT all_refs.thread_id) > 0 THEN
					ROUND(
						COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.id END)::decimal
						/ COUNT(DISTINCT all_refs.thread_id) * 100, 2
					)
				ELSE NULL
			END,
			NULL,
			NULL,
			AVG(
				CASE WHEN t.completed_at IS NOT NULL THEN
					EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000
				END
			)::bigint,
			NOW()
		FROM entity_profile ep
		JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id AND ept.archived_at IS NULL
		JOIN thread_refs all_refs ON all_refs.ref_key = ept.type AND all_refs.ref_value = ep.ref_key
		JOIN threads t ON t.id = all_refs.thread_id AND t.company_id = ep.company_id
		LEFT JOIN thread_validations tv ON tv.thread_id = all_refs.thread_id
		WHERE ep.id IN (
			SELECT DISTINCT ep2.id
			FROM entity_profile ep2
			JOIN entity_profile_type ept2 ON ept2.id = ep2.entity_profile_type_id
			JOIN thread_refs tr2 ON tr2.ref_key = ept2.type AND tr2.ref_value = ep2.ref_key
			WHERE tr2.thread_id = ANY($1)
		)
		GROUP BY ep.id
		ON CONFLICT (entity_profile_id) DO UPDATE SET
			total_deliveries           = EXCLUDED.total_deliveries,
			completed_successfully     = EXCLUDED.completed_successfully,
			validation_violations      = EXCLUDED.validation_violations,
			prev_delivery_health_score = entity_profile_metrics.delivery_health_score,
			delivery_health_score      = EXCLUDED.delivery_health_score,
			health_trend_slope         = EXCLUDED.delivery_health_score
			                             - COALESCE(entity_profile_metrics.delivery_health_score, EXCLUDED.delivery_health_score),
			average_delivery_time_ms   = EXCLUDED.average_delivery_time_ms,
			last_calculated_at         = NOW()`

	_, err := c.db.Pool.Exec(queryCtx, query, threadIDs)
	if err != nil {
		return fmt.Errorf("recalculate entity profile metrics: %w", err)
	}
	return nil
}

func (c *IntelligenceConsumer) recalculatePartnerCompatibility(ctx context.Context, threadIDs []string) error {
	queryCtx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	query := `
		INSERT INTO entity_partner_compatibility (
			id, entity_profile_id, partner_ref,
			total_interactions, successful_interactions,
			compatibility_score, last_calculated_at
		)
		SELECT
			gen_random_uuid(),
			ep.id,
			partner_refs.ref_value,
			COUNT(DISTINCT t.id),
			COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.id END),
			CASE
				WHEN COUNT(DISTINCT t.id) > 0 THEN
					ROUND(
						COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.id END)::decimal
						/ COUNT(DISTINCT t.id) * 100, 2
					)
				ELSE NULL
			END,
			NOW()
		FROM entity_profile ep
		JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id AND ept.archived_at IS NULL
		JOIN thread_refs entity_refs ON entity_refs.ref_key = ept.type AND entity_refs.ref_value = ep.ref_key
		JOIN threads t ON t.id = entity_refs.thread_id AND t.company_id = ep.company_id
		JOIN thread_refs partner_refs ON partner_refs.thread_id = t.id AND partner_refs.ref_key = 'partner_ref'
		WHERE entity_refs.thread_id = ANY($1)
		GROUP BY ep.id, partner_refs.ref_value
		ON CONFLICT (entity_profile_id, partner_ref) DO UPDATE SET
			total_interactions      = EXCLUDED.total_interactions,
			successful_interactions = EXCLUDED.successful_interactions,
			compatibility_score     = EXCLUDED.compatibility_score,
			last_calculated_at      = NOW()`

	_, err := c.db.Pool.Exec(queryCtx, query, threadIDs)
	if err != nil {
		return fmt.Errorf("recalculate partner compatibility: %w", err)
	}
	return nil
}

func (c *IntelligenceConsumer) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopChan)
	})
	c.wg.Wait()
	c.shutdownFn()
	c.logger.Info("stopped intelligence consumer", zap.String("consumer", c.consumerID))
}
