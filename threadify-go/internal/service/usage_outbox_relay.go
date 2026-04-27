package service

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"threadify-go/shared/database"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	stopTimeout = 20 * time.Second

	usageOutboxGroupName = "usage-outbox-relay"

	usageOutboxBatchSize = int64(200)
	blockDuration        = 2 * time.Second
	noBlock              = time.Duration(0)
	stalePELThreshold    = 5 * time.Minute

	errorBackoff = 500 * time.Millisecond
	emptyBackoff = 250 * time.Millisecond

	streamStartPending = "0"
	streamStartNew     = ">"
)

type UsageOutboxRelay struct {
	valkey        UsageOutboxValkey
	natsPublisher UsageOutboxPublisher
	logger        *zap.Logger
	consumerName  string
	forwarder     UsageOutboxBatchForwarder
	sleeper       UsageOutboxSleeper

	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

//go:generate mockgen -package=usageoutboxmocks -destination=mocks/usageoutbox/usage_outbox_mocks.go -source=usage_outbox_relay.go
type UsageOutboxPublisher interface {
	PublishCreditTopupTrigger(ctx context.Context, event map[string]interface{}) error
	PublishUsageSyncBatch(ctx context.Context, events []map[string]interface{}) error
}

type UsageOutboxValkey interface {
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error)
	XAck(ctx context.Context, stream, group string, ids ...string) error
}

type UsageOutboxBatchForwarder interface {
	ForwardBatch(ctx context.Context, streamStart string, block time.Duration) (int, error)
}

type UsageOutboxSleeper interface {
	Sleep(ctx context.Context, d time.Duration)
}

type usageOutboxDefaultForwarder struct {
	relay *UsageOutboxRelay
}

func (f usageOutboxDefaultForwarder) ForwardBatch(ctx context.Context, streamStart string, block time.Duration) (int, error) {
	return f.relay.forwardBatch(ctx, streamStart, block)
}

type usageOutboxDefaultSleeper struct{}

func (usageOutboxDefaultSleeper) Sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func NewUsageOutboxRelay(
	valkey UsageOutboxValkey,
	natsPublisher UsageOutboxPublisher,
	logger *zap.Logger,
) *UsageOutboxRelay {
	relay := &UsageOutboxRelay{
		valkey:        valkey,
		natsPublisher: natsPublisher,
		logger:        logger,
		consumerName:  buildConsumerName(),
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
	}
	relay.forwarder = usageOutboxDefaultForwarder{relay: relay}
	relay.sleeper = usageOutboxDefaultSleeper{}
	return relay
}

// SetForwarder overrides the batch forwarder implementation (primarily for tests).
// If f is nil, the default forwarder is restored.
func (r *UsageOutboxRelay) SetForwarder(f UsageOutboxBatchForwarder) {
	if f == nil {
		r.forwarder = usageOutboxDefaultForwarder{relay: r}
		return
	}
	r.forwarder = f
}

// SetSleeper overrides the sleeper implementation (primarily for tests).
// If s is nil, the default sleeper is restored.
func (r *UsageOutboxRelay) SetSleeper(s UsageOutboxSleeper) {
	if s == nil {
		r.sleeper = usageOutboxDefaultSleeper{}
		return
	}
	r.sleeper = s
}

func buildConsumerName() string {
	if host, err := os.Hostname(); err == nil && host != "" {
		return fmt.Sprintf("%s-%s", usageOutboxGroupName, host)
	}
	return fmt.Sprintf("%s-%d", usageOutboxGroupName, os.Getpid())
}

func (r *UsageOutboxRelay) Start() error {
	if r.valkey == nil {
		return fmt.Errorf("usage outbox relay: valkey client is required")
	}
	if r.natsPublisher == nil {
		return fmt.Errorf("usage outbox relay: nats publisher is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.valkey.XGroupCreateMkStream(ctx, database.UsageOutboxStreamKey, usageOutboxGroupName, "0"); err != nil {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-r.stopCh
		cancel()
	}()

	for {
		select {
		case <-r.stopCh:
			return
		default:
		}

		attempted, err := r.forwarder.ForwardBatch(ctx, streamStartPending, noBlock)
		if err != nil {
			r.logger.Error("usage outbox relay: failed to process pending messages", zap.Error(err))
			r.sleeper.Sleep(ctx, errorBackoff)
			continue
		}
		if attempted > 0 {
			continue
		}

		_, err = r.forwarder.ForwardBatch(ctx, streamStartNew, blockDuration)
		if err != nil {
			if err == redis.Nil || err == context.Canceled {
				r.sleeper.Sleep(ctx, emptyBackoff)
				continue
			}
			r.logger.Error("usage outbox relay: failed to process new messages", zap.Error(err))
			r.sleeper.Sleep(ctx, errorBackoff)
		}
	}
}

type aggregateKey struct {
	companyID         string
	meter             string
	billingCycleStart string
	eventID           string
}

type bucket struct {
	companyID         string
	meter             string
	billingCycleStart string
	amount            int64
	eventID           string
	timestamp         string
}

func isCreditMeter(meter string) bool {
	return meter == MeterCreditSpend || meter == MeterCreditTopup || meter == MeterCreditTopupRequest
}

func (r *UsageOutboxRelay) forwardBatch(ctx context.Context, streamStart string, block time.Duration) (int, error) {
	readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
	defer readCancel()

	streams, err := r.valkey.XReadGroup(readCtx, &redis.XReadGroupArgs{
		Group:    usageOutboxGroupName,
		Consumer: r.consumerName,
		Streams:  []string{database.UsageOutboxStreamKey, streamStart},
		Count:    usageOutboxBatchSize,
		Block:    block,
		NoAck:    false,
	})
	if err != nil {
		return 0, err
	}

	msgIDToKey := make(map[string]aggregateKey)
	window := make(map[aggregateKey]*bucket)
	var allIDs []string
	var poisonIDs []string

	for _, stream := range streams {
		for _, msg := range stream.Messages {
			if streamStart == streamStartPending {
				r.warnIfStale(msg.ID)
			}
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
			eventID, _ := event[fieldEventID].(string)

			rawAmount := event[fieldAmount]
			amount, err := parseRelayAmount(rawAmount)
			if err != nil {
				poisonIDs = append(poisonIDs, msg.ID)
				r.logger.Warn("dropping usage outbox message with unparseable amount",
					zap.String("stream_id", msg.ID),
					zap.String("meter", meter),
					zap.Any("amount", rawAmount),
					zap.Error(err),
				)
				continue
			}

			if amount == 0 {
				poisonIDs = append(poisonIDs, msg.ID)
				r.logger.Debug("silently dropping zero-amount outbox message",
					zap.String("stream_id", msg.ID),
					zap.String("meter", meter),
				)
				continue
			}

			keyEventID := ""
			if isCreditMeter(meter) {
				keyEventID = eventID
			}

			key := aggregateKey{
				companyID:         companyID,
				meter:             meter,
				billingCycleStart: billingCycleStart,
				eventID:           keyEventID,
			}
			msgIDToKey[msg.ID] = key

			if b, exists := window[key]; exists {
				b.amount += amount
				b.timestamp = timestamp
				if !isCreditMeter(meter) {
					b.eventID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(b.eventID+":"+eventID)).String()
				}
			} else {
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
		poisonCtx, poisonCancel := context.WithTimeout(ctx, 5*time.Second)
		_ = r.valkey.XAck(poisonCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, poisonIDs...)
		poisonCancel()
	}

	attempted := len(allIDs) - len(poisonIDs)

	if len(window) == 0 {
		return attempted, nil
	}

	r.logger.Debug("usage outbox relay: aggregated batch",
		zap.Int("raw_events", attempted),
		zap.Int("poison_dropped", len(poisonIDs)),
		zap.Int("buckets", len(window)),
	)

	pubCtx, pubCancel := context.WithTimeout(ctx, 15*time.Second)
	defer pubCancel()

	var syncEvents []map[string]interface{}
	failedKeys := make(map[aggregateKey]struct{})

	for key, b := range window {
		if b.meter == MeterCreditTopupRequest {
			if err := r.natsPublisher.PublishCreditTopupTrigger(pubCtx, bucketToMap(b)); err != nil {
				r.logger.Error("failed to publish credit topup trigger to NATS",
					zap.String("company_id", b.companyID),
					zap.Error(err),
				)
				failedKeys[key] = struct{}{}
			}
		} else {
			syncEvents = append(syncEvents, bucketToMap(b))
		}
	}

	if len(syncEvents) > 0 {
		r.logger.Debug("usage outbox relay: aggregated batch",
			zap.Int("sync_events", len(syncEvents)),
			zap.Any("sync_events", syncEvents),
		)
		if err := r.natsPublisher.PublishUsageSyncBatch(pubCtx, syncEvents); err != nil {
			r.logger.Error("failed to publish usage sync batch to NATS", zap.Error(err))
			for key, b := range window {
				if b.meter != MeterCreditTopupRequest {
					failedKeys[key] = struct{}{}
				}
			}
		}
	}

	var ackIDs []string
	for _, id := range allIDs {
		key, mapped := msgIDToKey[id]
		if !mapped {
			continue
		}
		if _, failed := failedKeys[key]; !failed {
			ackIDs = append(ackIDs, id)
		}
	}

	if len(ackIDs) > 0 {
		ackCtx, ackCancel := context.WithTimeout(ctx, 5*time.Second)
		if ackErr := r.valkey.XAck(ackCtx, database.UsageOutboxStreamKey, usageOutboxGroupName, ackIDs...); ackErr != nil {
			r.logger.Warn("failed to ACK outbox entries — may be redelivered",
				zap.Int("count", len(ackIDs)),
				zap.Error(ackErr),
			)
		}
		ackCancel()
	}

	return attempted, nil
}

func (r *UsageOutboxRelay) warnIfStale(msgID string) {
	parts := strings.SplitN(msgID, "-", 2)
	if len(parts) < 2 {
		return
	}
	ms, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
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

// MapUsageOutboxEvent is an exported wrapper around mapUsageOutboxEvent (primarily for tests).
func MapUsageOutboxEvent(values map[string]interface{}) (map[string]interface{}, bool) {
	return mapUsageOutboxEvent(values)
}

func parseRelayAmount(v interface{}) (int64, error) {
	switch value := v.(type) {
	case string:
		return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	case int64:
		return value, nil
	case float64:
		return int64(value), nil
	default:
		return 0, fmt.Errorf("unsupported amount type %T", v)
	}
}

// ParseRelayAmount is an exported wrapper around parseRelayAmount (primarily for tests).
func ParseRelayAmount(v interface{}) (int64, error) {
	return parseRelayAmount(v)
}

func bucketToMap(b *bucket) map[string]interface{} {
	return map[string]interface{}{
		fieldEventID:           b.eventID,
		fieldCompanyID:         b.companyID,
		fieldMeter:             b.meter,
		fieldAmount:            strconv.FormatInt(b.amount, 10),
		fieldBillingCycleStart: b.billingCycleStart,
		fieldTimestamp:         b.timestamp,
	}
}
