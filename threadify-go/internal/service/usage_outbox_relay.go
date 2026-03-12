package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/database"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

const (
	usageOutboxGroupName = "usage-outbox-relay"
	usageOutboxBatchSize = int64(200)

	streamStartPending = "0"
	streamStartNew     = ">"
	noBlock            = time.Duration(0)
	blockDuration      = 2 * time.Second
	errorBackoff       = 500 * time.Millisecond
	emptyBackoff       = 250 * time.Millisecond
	stalePELThreshold  = 5 * time.Minute
	stopTimeout        = 20 * time.Second

	fieldEventID           = "event_id"
	fieldCompanyID         = "company_id"
	fieldMeter             = "meter"
	fieldAmount            = "amount"
	fieldBillingCycleStart = "billing_cycle_start"
	fieldTimestamp         = "timestamp"
)

type UsageOutboxRelay struct {
	valkey        *database.ValkeyService
	natsPublisher *natsrepo.ArchivalPublisher
	logger        *zap.Logger
	consumerName  string

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

func NewUsageOutboxRelay(
	valkey *database.ValkeyService,
	natsPublisher *natsrepo.ArchivalPublisher,
	logger *zap.Logger,
) *UsageOutboxRelay {
	return &UsageOutboxRelay{
		valkey:        valkey,
		natsPublisher: natsPublisher,
		logger:        logger,
		consumerName:  buildConsumerName(),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
}

func buildConsumerName() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return fmt.Sprintf("%s-%s", usageOutboxGroupName, host)
	}
	return fmt.Sprintf("%s-%d", usageOutboxGroupName, os.Getpid())
}

func (r *UsageOutboxRelay) Start() error {
	if r.valkey == nil || r.valkey.Client == nil {
		return fmt.Errorf("usage outbox relay: valkey client is required")
	}
	if r.natsPublisher == nil {
		return fmt.Errorf("usage outbox relay: nats publisher is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.valkey.Client.XGroupCreateMkStream(ctx, database.UsageOutboxStreamKey, usageOutboxGroupName, "0").Err(); err != nil {
		if !strings.Contains(err.Error(), "BUSYGROUP") {
			return fmt.Errorf("create usage outbox consumer group: %w", err)
		}
	}

	go r.run()
	r.logger.Info("usage outbox relay started",
		zap.String("stream", database.UsageOutboxStreamKey),
		zap.String("group", usageOutboxGroupName),
		zap.String("consumer", r.consumerName),
	)
	return nil
}

func (r *UsageOutboxRelay) Stop() error {
	r.stopOnce.Do(func() {
		close(r.stopCh)
	})

	select {
	case <-r.doneCh:
	case <-time.After(stopTimeout):
		r.logger.Warn("usage outbox relay stop timed out",
			zap.Duration("timeout", stopTimeout),
		)
	}
	return nil
}

func (r *UsageOutboxRelay) run() {
	defer close(r.doneCh)

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		processedPending, err := r.forwardBatch(streamStartPending, noBlock)
		if err != nil {
			r.logger.Error("usage outbox relay: failed to process pending messages", zap.Error(err))
			time.Sleep(errorBackoff)
			continue
		}
		if processedPending > 0 {
			continue
		}

		if _, err := r.forwardBatch(streamStartNew, blockDuration); err != nil {
			if err != redis.Nil {
				r.logger.Error("usage outbox relay: failed to process new messages", zap.Error(err))
			}
			time.Sleep(emptyBackoff)
		}
	}
}

type aggregateKey struct {
	companyID         string
	meter             string
	billingCycleStart string
}

func (r *UsageOutboxRelay) forwardBatch(streamStart string, block time.Duration) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streams, err := r.valkey.Client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    usageOutboxGroupName,
		Consumer: r.consumerName,
		Streams:  []string{database.UsageOutboxStreamKey, streamStart},
		Count:    usageOutboxBatchSize,
		Block:    block,
		NoAck:    false,
	}).Result()
	if err != nil {
		return 0, err
	}

	type bucket struct {
		companyID         string
		meter             string
		billingCycleStart string
		amount            int64
		eventID           string
		timestamp         string
	}

	window := make(map[aggregateKey]*bucket)
	var allIDs []string
	var poisonIDs []string

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			r.warnIfStale(msg.ID)
			allIDs = append(allIDs, msg.ID)

			event, ok := mapUsageOutboxEvent(msg.Values)
			if !ok {
				poisonIDs = append(poisonIDs, msg.ID)
				r.logger.Warn("dropping malformed usage outbox message",
					zap.String("stream_id", msg.ID),
					zap.Any("values", msg.Values),
				)
				continue
			}

			companyID, _ := event[fieldCompanyID].(string)
			meter, _ := event[fieldMeter].(string)
			billingCycleStart, _ := event[fieldBillingCycleStart].(string)
			timestamp, _ := event[fieldTimestamp].(string)

			rawAmount := event[fieldAmount]
			amount, err := parseRelayAmount(rawAmount)
			if err != nil || amount <= 0 {
				poisonIDs = append(poisonIDs, msg.ID)
				r.logger.Warn("dropping usage outbox message with invalid amount",
					zap.String("stream_id", msg.ID),
					zap.Any("amount", rawAmount),
					zap.Error(err),
				)
				continue
			}

			key := aggregateKey{
				companyID:         companyID,
				meter:             meter,
				billingCycleStart: billingCycleStart,
			}

			if b, exists := window[key]; exists {
				b.amount += amount
				b.timestamp = timestamp
			} else {
				eventID, _ := event[fieldEventID].(string)
				window[key] = &bucket{
					companyID:         companyID,
					meter:             meter,
					billingCycleStart: billingCycleStart,
					amount:            amount,
					eventID:           eventID,
					timestamp:         timestamp,
				}
			}
		}
	}

	if len(poisonIDs) > 0 {
		poisonCtx, poisonCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = r.valkey.Client.XAck(poisonCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, poisonIDs...).Err()
		poisonCancel()
	}

	if len(window) == 0 {
		return 0, nil
	}

	aggregated := make([]map[string]interface{}, 0, len(window))
	for _, b := range window {
		aggregated = append(aggregated, map[string]interface{}{
			fieldEventID:           b.eventID,
			fieldCompanyID:         b.companyID,
			fieldMeter:             b.meter,
			fieldAmount:            b.amount,
			fieldBillingCycleStart: b.billingCycleStart,
			fieldTimestamp:         b.timestamp,
		})
	}

	r.logger.Debug("usage outbox relay: aggregated batch",
		zap.Int("raw_events", len(allIDs)-len(poisonIDs)),
		zap.Int("aggregated_buckets", len(aggregated)),
		zap.Int("poison_dropped", len(poisonIDs)),
	)

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 10*time.Second)
	publishErr := r.natsPublisher.PublishUsageSyncBatch(pubCtx, aggregated)
	pubCancel()

	if publishErr != nil {
		return 0, fmt.Errorf("publish usage batch to NATS: %w", publishErr)
	}

	validIDs := make([]string, 0, len(allIDs)-len(poisonIDs))
	poisonSet := make(map[string]struct{}, len(poisonIDs))
	for _, id := range poisonIDs {
		poisonSet[id] = struct{}{}
	}
	for _, id := range allIDs {
		if _, isPoison := poisonSet[id]; !isPoison {
			validIDs = append(validIDs, id)
		}
	}

	if len(validIDs) > 0 {
		ackCtx, ackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		ackErr := r.valkey.Client.XAck(ackCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, validIDs...).Err()
		ackCancel()

		if ackErr != nil {
			r.logger.Warn("failed to ACK processed outbox entries — may be redelivered",
				zap.Int("count", len(validIDs)),
				zap.Error(ackErr),
			)
		}
	}

	return len(aggregated), nil
}

func (r *UsageOutboxRelay) warnIfStale(msgID string) {
	var ms int64
	if _, err := fmt.Sscanf(msgID, "%d-", &ms); err != nil {
		return
	}
	age := time.Since(time.UnixMilli(ms))
	if age > stalePELThreshold {
		r.logger.Warn("usage outbox relay: stale PEL entry",
			zap.String("stream_id", msgID),
			zap.Duration("age", age),
			zap.Duration("threshold", stalePELThreshold),
		)
	}
}

func mapUsageOutboxEvent(values map[string]interface{}) (map[string]interface{}, bool) {
	required := []string{
		fieldEventID,
		fieldCompanyID,
		fieldMeter,
		fieldAmount,
		fieldBillingCycleStart,
		fieldTimestamp,
	}

	event := make(map[string]interface{}, len(required))
	for _, key := range required {
		value, ok := values[key]
		if !ok {
			return nil, false
		}
		event[key] = value
	}

	return event, true
}

func parseRelayAmount(v interface{}) (int64, error) {
	switch value := v.(type) {
	case string:
		var n int64
		if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
			return 0, fmt.Errorf("parse amount string %q: %w", value, err)
		}
		return n, nil
	case int64:
		return value, nil
	case float64:
		return int64(value), nil
	default:
		return 0, fmt.Errorf("unsupported amount type %T", v)
	}
}
