package nats

import (
	"fmt"
	"time"

	"threadify-go/shared/config"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// Client wraps NATS connection and JetStream context
type Client struct {
	conn   *nats.Conn
	js     nats.JetStreamContext
	cfg    *config.NATSConfig
	logger *zap.Logger
}

// NewClient creates a new NATS client with JetStream
func NewClient(cfg *config.NATSConfig, logger *zap.Logger) (*Client, error) {
	// Connect to NATS
	nc, err := nats.Connect(
		cfg.URL,
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		logger.Error("failed to connect to NATS", zap.Error(err))
		return nil, fmt.Errorf("failed to connect to message broker")
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		logger.Error("failed to create JetStream context", zap.Error(err))
		return nil, fmt.Errorf("failed to initialize message broker")
	}

	client := &Client{
		conn:   nc,
		js:     js,
		cfg:    cfg,
		logger: logger,
	}

	return client, nil
}

func (c *Client) InitializeOutboxStream() error {
	streamConfig := &nats.StreamConfig{
		Name:      StreamOutboxTriggers,
		Subjects:  []string{SubjectOutboxTrigger},
		Retention: nats.WorkQueuePolicy,
		MaxAge:    24 * time.Hour,
		Storage:   nats.FileStorage,
		Replicas:  1,
		Discard:   nats.DiscardOld,
		NoAck:     false,
	}

	_, err := c.js.AddStream(streamConfig)
	if err != nil {
		_, err = c.js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create/update outbox stream: %w", err)
		}
	}

	c.logger.Info("initialized OUTBOX_TRIGGERS stream")
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
