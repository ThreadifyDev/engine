package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

const (
	defaultQueryTimeout    = 15 * time.Second
	maxConsecutiveFailures = 5
	initialBackoff         = 1 * time.Second
	maxBackoff             = 30 * time.Second
	shutdownFlushTimeout   = 30 * time.Second
	consumerName           = "intelligence-metrics"
)

type IntelligenceConsumer struct {
	js           JetStreamPublisher
	db           DBExecer
	batchSize    int
	flushTimeout time.Duration
	consumerID   string
	logger       *zap.Logger

	mu     sync.Mutex
	buffer []pendingIntelEvent

	consecutiveFailures   atomic.Int32
	partnerCompatFailures atomic.Int32

	stopOnce    sync.Once
	stopChan    chan struct{}
	shutdownCtx context.Context
	shutdownFn  context.CancelFunc
	wg          sync.WaitGroup
}

type pendingIntelEvent struct {
	profileIDs []string
	msg        jetstream.Msg
}

func NewIntelligenceConsumer(
	js JetStreamPublisher,
	db DBExecer,
	batchSize int,
	flushInterval time.Duration,
	consumerID string,
	logger *zap.Logger,
) (*IntelligenceConsumer, error) {
	shutdownCtx, shutdownFn := context.WithCancel(context.Background())

	return &IntelligenceConsumer{
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
	cons, err := c.js.CreateOrUpdateConsumer(ctx, natsrepo.StreamProfileRecalculate, jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: natsrepo.SubjectProfileRecalculate,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	c.logger.Info("started intelligence consumer", zap.String("consumer", c.consumerID))

	c.wg.Add(1)
	go c.run(ctx, cons)
	return nil
}

func (c *IntelligenceConsumer) run(ctx context.Context, cons jetstream.Consumer) {
	defer c.wg.Done()

	consumeCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var profileIDs []string
		if err := json.Unmarshal(msg.Data(), &profileIDs); err != nil {
			c.logger.Error("unmarshal error", zap.Error(err))
			msg.Nak()
			return
		}

		if len(profileIDs) == 0 {
			msg.Ack()
			return
		}

		c.mu.Lock()
		c.buffer = append(c.buffer, pendingIntelEvent{
			profileIDs: profileIDs,
			msg:        msg,
		})

		if len(c.buffer) >= c.batchSize {
			pending := c.buffer
			c.buffer = make([]pendingIntelEvent, 0, c.batchSize)
			c.mu.Unlock()
			c.flush(ctx, pending)
			return
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
			c.flushAll()
			return
		case <-ctx.Done():
			consumeCtx.Stop()
			c.flushAll()
			return
		case <-ticker.C:
			c.mu.Lock()
			if len(c.buffer) > 0 {
				pending := c.buffer
				c.buffer = make([]pendingIntelEvent, 0, c.batchSize)
				c.mu.Unlock()
				c.flush(ctx, pending)
			} else {
				c.mu.Unlock()
			}
		}
	}
}

func (c *IntelligenceConsumer) flushAll() {
	ctx, cancel := context.WithTimeout(c.shutdownCtx, shutdownFlushTimeout)
	defer cancel()

	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	pending := c.buffer
	c.buffer = make([]pendingIntelEvent, 0, c.batchSize)
	c.mu.Unlock()

	c.flush(ctx, pending)
}

func (c *IntelligenceConsumer) flush(ctx context.Context, pending []pendingIntelEvent) {
	if len(pending) == 0 {
		return
	}

	seen := make(map[string]struct{}, len(pending)*2)
	profileIDs := make([]string, 0, len(pending)*2)
	for _, p := range pending {
		for _, id := range p.profileIDs {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				profileIDs = append(profileIDs, id)
			}
		}
	}

	c.logger.Info("recalculating entity profile metrics",
		zap.Int("messages", len(pending)),
		zap.Int("unique_profiles", len(profileIDs)),
		zap.String("consumer", c.consumerID),
	)

	if err := c.recalculateMetrics(ctx, profileIDs); err != nil {
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
		if err := p.msg.Ack(); err != nil {
			ackErrors++
			c.logger.Error("failed to ack intelligence message", zap.Error(err))
		}
	}

	if ackErrors > 0 {
		c.logger.Warn("batch processed but some ACKs failed",
			zap.Int("failed_acks", ackErrors),
			zap.Int("total_acks", len(pending)),
		)
	}

	c.logger.Info("entity profile metrics updated",
		zap.Int("profiles_processed", len(profileIDs)),
		zap.Int("acks_sent", len(pending)-ackErrors),
		zap.String("consumer", c.consumerID),
	)
}

func (c *IntelligenceConsumer) recalculateMetrics(ctx context.Context, profileIDs []string) error {
	if len(profileIDs) == 0 {
		return nil
	}

	if err := c.recalculateProfileMetrics(ctx, profileIDs); err != nil {
		return err
	}

	if err := c.recalculatePartnerCompatibility(ctx, profileIDs); err != nil {
		c.logger.Warn("failed to update partner compatibility (non-fatal)", zap.Error(err))
		c.partnerCompatFailures.Add(1)
	}

	return nil
}

func (c *IntelligenceConsumer) recalculateProfileMetrics(ctx context.Context, profileIDs []string) error {
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
			SUM(COALESCE(v.violation_count, 0)),
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
				CASE WHEN t.completed_at IS NOT NULL AND t.completed_at > t.created_at THEN
					EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000
				END
			)::bigint,
			NOW()
		FROM entity_profile ep
		JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id AND ept.archived_at IS NULL
		JOIN thread_refs all_refs ON all_refs.ref_key = ept.type AND all_refs.ref_value = ep.ref_key
		JOIN threads t ON t.id = all_refs.thread_id AND t.company_id = ep.company_id
		LEFT JOIN LATERAL (
			SELECT COUNT(DISTINCT tv.validation_id) AS violation_count
			FROM thread_validations tv
			WHERE tv.thread_id = t.id AND tv.overall_status = 'violated'
		) v ON TRUE
		WHERE ep.id = ANY($1)
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

	if _, err := c.db.Exec(queryCtx, query, profileIDs); err != nil {
		return fmt.Errorf("recalculate entity profile metrics: %w", err)
	}
	return nil
}

func (c *IntelligenceConsumer) recalculatePartnerCompatibility(ctx context.Context, profileIDs []string) error {
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
		WHERE ep.id = ANY($1)
		GROUP BY ep.id, partner_refs.ref_value
		ON CONFLICT (entity_profile_id, partner_ref) DO UPDATE SET
			total_interactions      = EXCLUDED.total_interactions,
			successful_interactions = EXCLUDED.successful_interactions,
			compatibility_score     = EXCLUDED.compatibility_score,
			last_calculated_at      = NOW()`

	if _, err := c.db.Exec(queryCtx, query, profileIDs); err != nil {
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
	c.logger.Info("stopped intelligence consumer")
}
