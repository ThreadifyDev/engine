package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"go.uber.org/zap"
)

type StreamEvent struct {
	StreamID string
	Data     map[string]string
}

type NATSConsumer struct {
	js           jetstream.JetStream
	db           *database.PostgresDB
	writer       *PostgresWriter
	batchSize    int
	batchTimeout time.Duration
	consumerName string
	stopChan     chan struct{}
	cfg          *config.Config
	logger       *zap.Logger
}

func NewNATSConsumer(nc *nats.Conn, db *database.PostgresDB, batchSize int, batchTimeout time.Duration, consumerName string, cfg *config.Config, logger *zap.Logger) (*NATSConsumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	return &NATSConsumer{
		js:           js,
		db:           db,
		writer:       NewPostgresWriter(db, logger),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		consumerName: consumerName,
		stopChan:     make(chan struct{}),
		cfg:          cfg,
		logger:       logger,
	}, nil
}

func (c *NATSConsumer) Start(ctx context.Context) error {
	c.logger.Info("starting NATS archival consumer", zap.String("consumer", c.consumerName))

	go c.consumeStream(ctx, "activity_log", "activity.log", c.processActivityLog)
	go c.consumeStream(ctx, "thread_metadata", "metadata.thread", c.processThreadMetadata)
	go c.consumeStream(ctx, "thread_access", "access.thread", c.processThreadAccess)
	go c.consumeStream(ctx, "thread_validations", "validations.thread", c.processThreadValidations)

	c.logger.Info("NATS archival consumer started", zap.String("consumer", c.consumerName))
	return nil
}

func (c *NATSConsumer) Stop() {
	c.logger.Info("stopping NATS archival consumer")
	close(c.stopChan)
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
		for {
			msg, err := iter.Next()
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					close(msgChan)
					return
				}
				c.logger.Error("error fetching message", zap.String("stream", streamName), zap.Error(err))
				StreamConsumptionErrors.WithLabelValues(streamName).Inc()
				time.Sleep(time.Second)
				continue
			}
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
		BatchProcessingDuration.WithLabelValues(streamName).Observe(time.Since(start).Seconds())
		if err != nil {
			c.logger.Error("failed to process batch", zap.String("stream", streamName), zap.Error(err))
			BatchesProcessed.WithLabelValues(streamName, "error").Inc()
			for _, msg := range batch {
				msg.Nak()
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
			flush("shutdown")
			return
		case <-ticker.C:
			flush("time")
		case msg, ok := <-msgChan:
			if !ok {
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

func (c *NATSConsumer) parseMsgs(streamName string, msgs []jetstream.Msg) []StreamEvent {
	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			c.logger.Error("failed to unmarshal message", zap.String("stream", streamName), zap.Error(err))
			continue
		}
		events = append(events, StreamEvent{StreamID: msg.Subject(), Data: convertToStringMap(data)})
	}
	return events
}

func (c *NATSConsumer) logPerf(subject string, count int, start time.Time) {
	duration := time.Since(start)
	c.logger.Info("processed messages",
		zap.String("subject", subject),
		zap.Int("count", count),
		zap.Duration("duration", duration),
		zap.Float64("rate", float64(count)/duration.Seconds()),
	)
}

func (c *NATSConsumer) processActivityLog(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()

	events := make([]StreamEvent, 0, len(msgs))
	var allSubSteps []map[string]interface{}

	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			c.logger.Error("failed to unmarshal activity log message", zap.Error(err))
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
			events = append(events, StreamEvent{StreamID: msg.Subject(), Data: convertToStringMap(data)})
		}
	}

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
	err := c.writer.WriteThreadMetadata(ctx, c.parseMsgs("thread_metadata", msgs))
	c.logPerf("metadata.thread", len(msgs), start)
	return err
}

func (c *NATSConsumer) processThreadAccess(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	err := c.writer.WriteThreadAccess(ctx, c.parseMsgs("thread_access", msgs))
	c.logPerf("access.thread", len(msgs), start)
	return err
}

func (c *NATSConsumer) processThreadValidations(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}
	start := time.Now()
	err := c.writer.WriteValidationResults(ctx, c.parseMsgs("thread_validations", msgs))
	c.logPerf("validations.thread", len(msgs), start)
	return err
}

func convertToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string, len(data))
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}
