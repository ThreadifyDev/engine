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

	var batchEvents []map[string]interface{}
	var batchIDs []string
	var poisonIDs []string

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			r.warnIfStale(msg.ID)

			event, ok := mapUsageOutboxEvent(msg.Values)
			if !ok {
				poisonIDs = append(poisonIDs, msg.ID)
				r.logger.Warn("dropping malformed usage outbox message",
					zap.String("stream_id", msg.ID),
					zap.Any("values", msg.Values),
				)
				continue
			}

			batchEvents = append(batchEvents, event)
			batchIDs = append(batchIDs, msg.ID)
		}
	}

	if len(poisonIDs) > 0 {
		poisonCtx, poisonCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = r.valkey.Client.XAck(poisonCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, poisonIDs...).Err()
		poisonCancel()
	}

	if len(batchEvents) == 0 {
		return 0, nil
	}

	pubCtx, pubCancel := context.WithTimeout(context.Background(), 10*time.Second)
	publishErr := r.natsPublisher.PublishUsageSyncBatch(pubCtx, batchEvents)
	pubCancel()

	if publishErr != nil {
		return 0, fmt.Errorf("publish usage batch to NATS: %w", publishErr)
	}

	ackCtx, ackCancel := context.WithTimeout(context.Background(), 5*time.Second)
	ackErr := r.valkey.Client.XAck(ackCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, batchIDs...).Err()
	ackCancel()

	if ackErr != nil {
		return 0, fmt.Errorf("ack usage outbox messages: %w", ackErr)
	}

	return len(batchEvents), nil
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
