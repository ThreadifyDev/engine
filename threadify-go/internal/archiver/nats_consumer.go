package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

type StreamEvent struct {
	StreamID string
	Data     map[string]string
}

type MetricsInvalidator interface {
	InvalidateEntityMetrics(ctx context.Context, profileID string) error
}

type NATSConsumer struct {
	js                 JetStreamPublisher
	db                 DBExecer
	metricsInvalidator MetricsInvalidator
	writer             ConsumerWriter
	batchSize          int
	batchTimeout       time.Duration
	consumerName       string
	stopOnce           sync.Once
	stopChan           chan struct{}
	cfg                *config.Config
	logger             *zap.Logger
}

func NewNATSConsumer(
	js JetStreamPublisher,
	db DBExecer,
	metricsInvalidator MetricsInvalidator,
	batchSize int,
	batchTimeout time.Duration,
	consumerName string,
	cfg *config.Config,
	logger *zap.Logger,
) (*NATSConsumer, error) {
	return &NATSConsumer{
		js:                 js,
		db:                 db,
		metricsInvalidator: metricsInvalidator,
		writer:             NewPostgresWriter(db, logger),
		batchSize:          batchSize,
		batchTimeout:       batchTimeout,
		consumerName:       consumerName,
		stopChan:           make(chan struct{}),
		cfg:                cfg,
		logger:             logger,
	}, nil
}

func (c *NATSConsumer) Start(ctx context.Context) error {
	c.logger.Info("starting NATS archival consumer", zap.String("consumer", c.consumerName))

	var wg sync.WaitGroup

	consumers := []struct {
		streamName string
		subject    string
		processor  func(context.Context, []jetstream.Msg) error
	}{
		{"activity_log", "activity.log", c.processActivityLog},
		{"thread_metadata", "metadata.thread", c.processThreadMetadata},
		{"thread_access", "access.thread", c.processThreadAccess},
		{"thread_validations", "validations.thread", c.processThreadValidations},
		{"thread_notifications", "notifications.thread", c.processThreadNotifications},
		{"usage_sync", "usage.sync", c.processUsageSync},
	}

	for _, consumer := range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.consumeStream(ctx, consumer.streamName, consumer.subject, consumer.processor)
		}()
	}

	c.logger.Info("NATS archival consumer started", zap.String("consumer", c.consumerName))

	<-ctx.Done()
	c.Stop()
	wg.Wait()
	return nil
}

func (c *NATSConsumer) Stop() {
	c.stopOnce.Do(func() {
		c.logger.Info("stopping NATS archival consumer")
		close(c.stopChan)
	})
}

func (c *NATSConsumer) consumeStream(ctx context.Context, streamName, subject string, processor func(context.Context, []jetstream.Msg) error) {
	c.logger.Info("starting stream consumer", zap.String("stream", streamName), zap.String("subject", subject))

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable:       fmt.Sprintf("archiver-%s", streamName),
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    c.cfg.NATS.ArchiverMaxDeliver,
		AckWait:       time.Duration(c.cfg.NATS.ArchiverAckWaitSeconds) * time.Second,
		FilterSubject: subject,
	})
	if err != nil {
		c.logger.Error("failed to create consumer", zap.String("stream", streamName), zap.Error(err))
		return
	}

	iter, err := consumer.Messages()
	if err != nil {
		c.logger.Error("failed to get message iterator", zap.String("stream", streamName), zap.Error(err))
		return
	}
	defer iter.Stop()

	msgChan := make(chan jetstream.Msg, c.batchSize)
	go func() {
		backoff := 100 * time.Millisecond
		for {
			msg, err := iter.Next()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
					strings.Contains(err.Error(), "iterator closed") {
					close(msgChan)
					return
				}
				if ctx.Err() != nil {
					close(msgChan)
					return
				}
				c.logger.Error("error fetching message", zap.String("stream", streamName), zap.Error(err))
				StreamConsumptionErrors.WithLabelValues(streamName).Inc()
				select {
				case <-time.After(backoff):
					backoff = min(backoff*2, 30*time.Second)
				case <-ctx.Done():
					close(msgChan)
					return
				}
				continue
			}
			backoff = 100 * time.Millisecond
			msgChan <- msg
		}
	}()

	batch := make([]jetstream.Msg, 0, c.batchSize)
	ticker := time.NewTicker(c.batchTimeout)
	defer ticker.Stop()

	flush := func(reason string) {
		if len(batch) == 0 {
			return
		}
		BatchSize.WithLabelValues(streamName).Observe(float64(len(batch)))
		start := time.Now()
		err := processor(ctx, batch)
		elapsed := time.Since(start)
		BatchProcessingDuration.WithLabelValues(streamName).Observe(elapsed.Seconds())
		if err != nil {
			if strings.Contains(err.Error(), "wait for thread metadata") {
				c.logger.Debug("batch processing delayed (waiting for thread metadata)", zap.String("stream", streamName), zap.Error(err))
			} else {
				c.logger.Error("failed to process batch", zap.String("stream", streamName), zap.Error(err))
			}
			BatchesProcessed.WithLabelValues(streamName, "error").Inc()
			for _, msg := range batch {
				if errors.Is(err, ErrThreadNotFound) {
					msg.NakWithDelay(5 * time.Second)
				} else {
					msg.Nak()
				}
				RetryAttempts.WithLabelValues(streamName).Inc()
			}
		} else {
			BatchesProcessed.WithLabelValues(streamName, "success").Inc()
			for _, msg := range batch {
				msg.Ack()
			}
		}
		batch = batch[:0]
		BufferFlushes.WithLabelValues(streamName, reason).Inc()
	}

	for {
		select {
		case <-c.stopChan:
			for {
				select {
				case msg, ok := <-msgChan:
					if !ok {
						flush("shutdown")
						return
					}
					StreamMessagesConsumed.WithLabelValues(streamName).Inc()
					batch = append(batch, msg)
				default:
					flush("shutdown")
					return
				}
			}

		case <-ticker.C:
			flush("time")

		case msg, ok := <-msgChan:
			if !ok {
				flush("shutdown")
				return
			}
			StreamMessagesConsumed.WithLabelValues(streamName).Inc()
			batch = append(batch, msg)
			if len(batch) >= c.batchSize {
				flush("size")
			}
		}
	}
}

func (c *NATSConsumer) parseMsgs(streamName string, msgs []jetstream.Msg) ([]StreamEvent, []jetstream.Msg) {
	events := make([]StreamEvent, 0, len(msgs))
	var failed []jetstream.Msg
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			c.logger.Error("failed to unmarshal message", zap.String("stream", streamName), zap.Error(err))
			failed = append(failed, msg)
			continue
		}
		events = append(events, StreamEvent{StreamID: msg.Subject(), Data: convertToStringMap(data)})
	}
	return events, failed
}

func (c *NATSConsumer) logDroppedMalformed(streamName string, failed []jetstream.Msg) {
	if len(failed) > 0 {
		c.logger.Warn("dropping malformed messages from batch",
			zap.String("stream", streamName),
			zap.Int("count", len(failed)),
		)
	}
}

func (c *NATSConsumer) logPerf(subject string, count int, start time.Time) {
	duration := time.Since(start)
	rate := 0.0
	if duration > 0 {
		rate = float64(count) / duration.Seconds()
	}
	c.logger.Info("processed messages",
		zap.String("subject", subject),
		zap.Int("count", count),
		zap.Duration("duration", duration),
		zap.Float64("rate", rate),
	)
}

func (c *NATSConsumer) processActivityLog(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()

	events := make([]StreamEvent, 0, len(msgs))
	var allSubSteps []map[string]interface{}
	var failed []jetstream.Msg

	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			c.logger.Error("failed to unmarshal activity log message", zap.Error(err))
			failed = append(failed, msg)
			continue
		}
		if eventType, ok := data["type"].(string); ok && eventType == "substeps_batch" {
			if substeps, ok := data["substeps"].([]interface{}); ok {
				for _, ss := range substeps {
					if subStepMap, ok := ss.(map[string]interface{}); ok {
						allSubSteps = append(allSubSteps, subStepMap)
					}
				}
			}
		} else {
			threadID, _ := data["threadId"].(string)
			if strings.TrimSpace(threadID) == "" {
				c.logger.Warn("dropping activity log message without threadId",
					zap.String("type", fmt.Sprintf("%v", data["type"])),
				)
				failed = append(failed, msg)
				continue
			}
			events = append(events, StreamEvent{StreamID: msg.Subject(), Data: convertToStringMap(data)})
		}
	}

	c.logDroppedMalformed("activity_log", failed)

	if len(events) > 0 {
		if err := c.writer.WriteActivityLog(ctx, events); err != nil {
			return err
		}
	}
	if len(allSubSteps) > 0 {
		if err := c.writer.WriteSubSteps(ctx, allSubSteps); err != nil {
			return err
		}
	}

	c.logPerf("activity.log", len(msgs), start)
	return nil
}

func (c *NATSConsumer) processThreadMetadata(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	events, failed := c.parseMsgs("thread_metadata", msgs)
	c.logDroppedMalformed("thread_metadata", failed)
	profileIDs, err := c.writer.WriteThreadMetadata(ctx, events)
	if err != nil {
		return err
	}

	if c.metricsInvalidator != nil {
		for _, profileID := range profileIDs {
			if profileID != "" {
				_ = c.metricsInvalidator.InvalidateEntityMetrics(ctx, profileID)
			}
		}
	}

	c.logPerf("metadata.thread", len(msgs), start)
	return nil
}

func (c *NATSConsumer) processThreadAccess(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	events, failed := c.parseMsgs("thread_access", msgs)
	c.logDroppedMalformed("thread_access", failed)
	err := c.writer.WriteThreadAccess(ctx, events)
	if errors.Is(err, ErrThreadNotFound) {
		c.logger.Debug("thread access arrived before thread metadata, will retry",
			zap.Int("count", len(msgs)),
		)
		c.logPerf("access.thread", len(msgs), start)
		return err
	}
	c.logPerf("access.thread", len(msgs), start)
	return err
}

func (c *NATSConsumer) processThreadValidations(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	events, failed := c.parseMsgs("thread_validations", msgs)
	c.logDroppedMalformed("thread_validations", failed)
	err := c.writer.WriteThreadValidations(ctx, events)
	c.logPerf("validations.thread", len(msgs), start)
	return err
}

func (c *NATSConsumer) processThreadNotifications(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	events, failed := c.parseMsgs("thread_notifications", msgs)
	c.logDroppedMalformed("thread_notifications", failed)
	err := c.writer.WriteThreadNotifications(ctx, events)
	c.logPerf("notifications.thread", len(msgs), start)
	return err
}

func (c *NATSConsumer) processUsageSync(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	c.logger.Debug("received usage sync messages", zap.Int("count", len(msgs)))
	events := make([]UsageSyncEvent, 0, len(msgs)*4)

	for _, msg := range msgs {
		var dataArray []map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &dataArray); err != nil {
			c.logger.Error("failed to unmarshal usage sync message", zap.Error(err))
			_ = msg.Term()
			continue
		}

		if len(dataArray) == 0 {
			c.logger.Warn("usage sync message contains empty array")
			_ = msg.Term()
			continue
		}

		msgEvents := c.parseBatchEvents(msg, dataArray)
		if len(msgEvents) == 0 {
			_ = msg.Term()
			continue
		}

		events = append(events, msgEvents...)
	}

	if len(events) == 0 {
		c.logPerf("usage.sync", len(msgs), start)
		return nil
	}

	if err := c.writer.SyncUsageMeters(ctx, events); err != nil {
		return err
	}

	c.logPerf("usage.sync", len(msgs), start)
	return nil
}

func (c *NATSConsumer) parseBatchEvents(msg jetstream.Msg, dataArray []map[string]interface{}) []UsageSyncEvent {
	events := make([]UsageSyncEvent, 0, len(dataArray))
	seqFallback := ""

	for i, data := range dataArray {
		companyID, _ := data["company_id"].(string)
		meter, _ := data["meter"].(string)

		if companyID == "" || meter == "" {
			c.logger.Error("usage sync event missing required fields",
				zap.Int("index", i),
				zap.String("company_id", companyID),
				zap.String("meter", meter),
			)
			continue
		}

		amount, err := parseUsageAmount(data["amount"])
		if err != nil {
			c.logger.Error("invalid usage amount in event",
				zap.Int("index", i),
				zap.Any("amount", data["amount"]),
				zap.Error(err),
			)
			continue
		}

		billingCycleStart, err := parseUsageTimestamp(data["billing_cycle_start"])
		if err != nil || billingCycleStart.IsZero() {
			c.logger.Error("missing or invalid billing_cycle_start in event",
				zap.Int("index", i),
				zap.Any("billing_cycle_start", data["billing_cycle_start"]),
				zap.Error(err),
			)
			continue
		}

		occurredAt, err := parseUsageTimestamp(data["timestamp"])
		if err != nil || occurredAt.IsZero() {
			c.logger.Error("missing or invalid timestamp in event",
				zap.Int("index", i),
				zap.Any("timestamp", data["timestamp"]),
				zap.Error(err),
			)
			continue
		}

		eventID, _ := data["event_id"].(string)
		if eventID == "" {
			if seqFallback == "" {
				if metadata, metaErr := msg.Metadata(); metaErr == nil && metadata != nil {
					seqFallback = fmt.Sprintf("nats:usage.sync:%d", metadata.Sequence.Stream)
				}
			}
			if seqFallback != "" {
				eventID = fmt.Sprintf("%s:%d", seqFallback, i)
			}
		}
		if eventID == "" {
			c.logger.Error("usage sync event has no resolvable event_id", zap.Int("index", i))
			continue
		}

		events = append(events, UsageSyncEvent{
			EventID:           eventID,
			CompanyID:         companyID,
			Meter:             meter,
			Amount:            amount,
			BillingCycleStart: billingCycleStart,
			OccurredAt:        occurredAt,
		})
	}

	return events
}

func parseUsageAmount(v interface{}) (int64, error) {
	switch value := v.(type) {
	case float64:
		return int64(value), nil
	case int64:
		return value, nil
	case int:
		return int64(value), nil
	case json.Number:
		return value.Int64()
	case string:
		return strconv.ParseInt(value, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported amount type %T", v)
	}
}

func parseUsageTimestamp(v interface{}) (time.Time, error) {
	switch value := v.(type) {
	case nil:
		return time.Time{}, nil
	case string:
		if strings.TrimSpace(value) == "" {
			return time.Time{}, nil
		}
		t, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return time.Time{}, err
		}
		return t.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", v)
	}
}

func convertToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string, len(data))
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}
