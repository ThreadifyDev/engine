package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

type Client struct {
	conn   *nats.Conn
	js     nats.JetStreamContext
	cfg    *config.NATSConfig
	logger *zap.Logger
}

func NewClient(cfg *config.NATSConfig, logger *zap.Logger) (*Client, error) {
	nc, err := nats.Connect(
		cfg.URL,
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to message broker: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("initialize message broker: %w", err)
	}

	c := &Client{conn: nc, js: js, cfg: cfg, logger: logger}

	steps := []struct {
		fn  func() error
		msg string
	}{
		{c.initializeNotificationStream, "initialize notifications"},
		{c.initializeDeadLetterQueue, "initialize DLQ"},
		{c.initializeArchivalStreams, "initialize archival system"},
		{c.initializeOutboxStream, "initialize outbox triggers"},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			nc.Close()
			return nil, fmt.Errorf("%s: %w", step.msg, err)
		}
	}

	return c, nil
}

// ensureStream creates or updates a JetStream stream.
func (c *Client) ensureStream(cfg *nats.StreamConfig) error {
	if _, err := c.js.AddStream(cfg); err != nil {
		if _, err = c.js.UpdateStream(cfg); err != nil {
			return fmt.Errorf("create/update stream %q: %w", cfg.Name, err)
		}
	}
	return nil
}

func (c *Client) initializeNotificationStream() error {
	err := c.ensureStream(&nats.StreamConfig{
		Name:       c.cfg.StreamName,
		Subjects:   []string{SubjectNotificationsUser},
		Retention:  nats.WorkQueuePolicy,
		MaxAge:     3 * 24 * time.Hour,
		Storage:    nats.FileStorage,
		Replicas:   1,
		Discard:    nats.DiscardOld,
		MaxMsgs:    -1,
		MaxBytes:   -1,
		Duplicates: 5 * time.Minute,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized notifications stream", zap.String("subject", SubjectNotificationsUser))
	return nil
}

func (c *Client) initializeDeadLetterQueue() error {
	err := c.ensureStream(&nats.StreamConfig{
		Name:      StreamNotificationsDLQ,
		Subjects:  []string{SubjectNotificationsDLQ},
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   nats.FileStorage,
		Replicas:  1,
		Discard:   nats.DiscardOld,
		MaxMsgs:   10000,
		MaxBytes:  100 * 1024 * 1024,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized notifications DLQ stream")
	return nil
}

func (c *Client) initializeArchivalStreams() error {
	streams := []struct {
		name    string
		subject []string
	}{
		{StreamActivityLog, []string{SubjectActivityLog}},
		{StreamThreadMetadata, []string{SubjectThreadMetadata}},
		{StreamThreadAccess, []string{SubjectThreadAccess}},
		{StreamThreadValidations, []string{SubjectThreadValidations}},
		{StreamStepState, []string{SubjectStepState}},
	}

	for _, stream := range streams {
		if err := c.ensureStream(&nats.StreamConfig{
			Name:       stream.name,
			Subjects:   stream.subject,
			Retention:  nats.LimitsPolicy,
			MaxAge:     24 * time.Hour,
			Storage:    nats.FileStorage,
			Replicas:   1,
			Discard:    nats.DiscardOld,
			MaxMsgs:    -1,
			MaxBytes:   -1,
			Duplicates: 5 * time.Minute,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) initializeOutboxStream() error {
	err := c.ensureStream(&nats.StreamConfig{
		Name:      StreamOutboxTriggers,
		Subjects:  []string{SubjectOutboxTrigger},
		Retention: nats.WorkQueuePolicy,
		MaxAge:    24 * time.Hour,
		Storage:   nats.FileStorage,
		Replicas:  1,
		Discard:   nats.DiscardOld,
		MaxMsgs:   -1,
		MaxBytes:  -1,
	})
	if err != nil {
		return err
	}
	c.logger.Info("initialized outbox triggers stream", zap.String("subject", SubjectOutboxTrigger))
	return nil
}

func (c *Client) JetStream() nats.JetStreamContext {
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

func (c *Client) FetchMessage(subject, consumerName string, timeout time.Duration) ([]byte, error) {
	consumerConfig := &nats.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: subject,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       time.Duration(c.cfg.AckWaitSeconds) * time.Second,
		MaxDeliver:    3,
		DeliverPolicy: nats.DeliverNewPolicy,
	}

	if _, err := c.js.AddConsumer(c.cfg.StreamName, consumerConfig); err != nil && err != nats.ErrConsumerNameAlreadyInUse {
		return nil, fmt.Errorf("create consumer: %w", err)
	}

	sub, err := c.js.PullSubscribe(subject, consumerName)
	if err != nil {
		return nil, fmt.Errorf("create pull subscription: %w", err)
	}
	defer sub.Unsubscribe()

	msgs, err := sub.Fetch(1, nats.MaxWait(timeout))
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages available")
	}

	if err := msgs[0].Ack(); err != nil {
		return nil, fmt.Errorf("ACK message: %w", err)
	}
	return msgs[0].Data, nil
}
