package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type Client struct {
	conn   *nats.Conn
	js     jetstream.JetStream
	cfg    *config.NATSConfig
	logger *zap.Logger
}

func NewClient(cfg *config.NATSConfig, logger *zap.Logger, options ...nats.Option) (*Client, error) {
	opts := []nats.Option{
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	opts = append(opts, options...)
	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to message broker: %w", err)
	}

	js, err := jetstream.New(nc, jetstream.WithPublishAsyncMaxPending(256))
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("initialize message broker: %w", err)
	}

	c := &Client{conn: nc, js: js, cfg: cfg, logger: logger}

	return c, nil
}

func (c *Client) InitStreams(ctx context.Context) error {
	steps := []struct {
		fn  func(context.Context) error
		msg string
	}{
		{c.initializeNotificationStream, "initialize notifications"},
		{c.initializeDeadLetterQueue, "initialize DLQ"},
		{c.initializeArchivalStreams, "initialize archival system"},
		{c.initializeOutboxStream, "initialize outbox triggers"},
	}
	for _, step := range steps {
		if err := step.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", step.msg, err)
		}
	}
	return nil
}

func (c *Client) ensureStream(ctx context.Context, cfg jetstream.StreamConfig) error {
	if _, err := c.js.CreateOrUpdateStream(ctx, cfg); err != nil {
		return fmt.Errorf("create/update stream %q: %w", cfg.Name, err)
	}
	return nil
}

func (c *Client) initializeNotificationStream(ctx context.Context) error {
	err := c.ensureStream(ctx, jetstream.StreamConfig{
		Name:       c.cfg.StreamName,
		Subjects:   []string{SubjectNotificationsUser},
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     3 * 24 * time.Hour,
		Storage:    jetstream.FileStorage,
		Replicas:   1,
		Discard:    jetstream.DiscardOld,
		MaxMsgs:    0,
		MaxBytes:   0,
		Duplicates: 5 * time.Minute,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized notifications stream", zap.String("subject", SubjectNotificationsUser))
	return nil
}

func (c *Client) initializeDeadLetterQueue(ctx context.Context) error {
	err := c.ensureStream(ctx, jetstream.StreamConfig{
		Name:      StreamNotificationsDLQ,
		Subjects:  []string{SubjectNotificationsDLQ},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
		Discard:   jetstream.DiscardOld,
		MaxMsgs:   10000,
		MaxBytes:  100 * 1024 * 1024,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized notifications DLQ stream")
	return nil
}

func (c *Client) initializeArchivalStreams(ctx context.Context) error {
	maxBytes := c.cfg.ArchivalMaxBytes
	if maxBytes == 0 {
		maxBytes = 1024 * 1024 * 1024
	}
	if maxBytes < 0 || c.cfg.ArchivalMaxAgeHours < 0 {
		return fmt.Errorf("archival storage limit must be positive and max age cannot be negative")
	}
	streams := []struct {
		name    string
		subject []string
	}{
		{StreamActivityLog, []string{SubjectActivityLog}},
		{StreamThreadMetadata, []string{SubjectThreadMetadata}},
		{StreamThreadAccess, []string{SubjectThreadAccess}},
		{StreamThreadValidations, []string{SubjectThreadValidations}},
		{StreamThreadNotifications, []string{SubjectThreadNotifications}},
		{StreamStepState, []string{SubjectStepState}},
		{StreamUsageSync, []string{SubjectUsageSync, SubjectCreditTopup}},
	}

	for _, stream := range streams {
		// NATS cannot change retention to/from WorkQueuePolicy. Never delete or
		// recreate an existing stream here: its unarchived events may be unique.
		existing, err := c.js.Stream(ctx, stream.name)
		if err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
			return fmt.Errorf("inspect archival stream %q: %w", stream.name, err)
		}
		if err == nil && existing.CachedInfo().Config.Retention != jetstream.WorkQueuePolicy {
			return fmt.Errorf("archival stream %q requires an explicit migration to work-queue retention: drain pending events using the previous engine and archiver, preserve the original store, then migrate to a fresh store or explicitly recreate the drained stream", stream.name)
		}
		if err := c.ensureStream(ctx, jetstream.StreamConfig{
			Name:       stream.name,
			Subjects:   stream.subject,
			Retention:  jetstream.WorkQueuePolicy,
			MaxAge:     time.Duration(c.cfg.ArchivalMaxAgeHours) * time.Hour,
			Storage:    jetstream.FileStorage,
			Replicas:   1,
			Discard:    jetstream.DiscardNew,
			MaxMsgs:    0,
			MaxBytes:   maxBytes,
			Duplicates: 5 * time.Minute,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) initializeOutboxStream(ctx context.Context) error {
	err := c.ensureStream(ctx, jetstream.StreamConfig{
		Name:      StreamOutboxTriggers,
		Subjects:  []string{SubjectOutboxTrigger},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
		Discard:   jetstream.DiscardOld,
		MaxMsgs:   0,
		MaxBytes:  0,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized outbox triggers stream", zap.String("subject", SubjectOutboxTrigger))
	return nil
}

func (c *Client) JetStream() jetstream.JetStream {
	return c.js
}

func (c *Client) Conn() *nats.Conn {
	return c.conn
}

func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

func (c *Client) FetchMessage(subject, consumerName string, timeout time.Duration) (*domain.NATSMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	defer cancel()

	consumerConfig := jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       time.Duration(c.cfg.AckWaitSeconds) * time.Second,
		MaxDeliver:    3,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	}

	cons, err := c.js.CreateOrUpdateConsumer(ctx, c.cfg.StreamName, consumerConfig)
	if err != nil {
		return nil, fmt.Errorf("create consumer: %w", err)
	}

	batch, err := cons.Fetch(1, jetstream.FetchMaxWait(timeout))
	if err != nil {
		return nil, err
	}

	msg := <-batch.Messages()
	if msg == nil {
		return nil, fmt.Errorf("no messages available")
	}

	if err := msg.Ack(); err != nil {
		return nil, fmt.Errorf("ACK message: %w", err)
	}
	return &domain.NATSMessage{Subject: msg.Subject(), Data: msg.Data()}, nil
}
