package nats

import (
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/config"
)

// Client wraps NATS connection and JetStream context
type Client struct {
	conn *nats.Conn
	js   nats.JetStreamContext
	cfg  *config.NATSConfig
}

// NewClient creates a new NATS client with JetStream
func NewClient(cfg *config.NATSConfig) (*Client, error) {
	// Connect to NATS
	nc, err := nats.Connect(cfg.URL,
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	client := &Client{
		conn: nc,
		js:   js,
		cfg:  cfg,
	}

	// Initialize notification stream
	if err := client.initializeNotificationStream(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to initialize notification stream: %w", err)
	}

	// Initialize archival streams
	if err := client.initializeArchivalStreams(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to initialize archival streams: %w", err)
	}

	return client, nil
}

// initializeNotificationStream creates or updates the notification stream
func (c *Client) initializeNotificationStream() error {
	streamConfig := &nats.StreamConfig{
		Name:       c.cfg.StreamName,
		Subjects:   []string{"thread.*.owner", "thread.*.participant", "thread.*.observer"},
		Retention:  nats.InterestPolicy, // Delete when all consumers ACK
		MaxAge:     time.Duration(c.cfg.MaxAgeHours) * time.Hour,
		Storage:    nats.FileStorage,
		Replicas:   1,
		Discard:    nats.DiscardOld,
		MaxMsgs:    -1, // Unlimited
		MaxBytes:   -1, // Unlimited
		NoAck:      false,
		Duplicates: 5 * time.Minute,
	}

	// Try to add stream, update if it already exists
	_, err := c.js.AddStream(streamConfig)
	if err != nil {
		// If stream exists, try to update it
		_, err = c.js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create/update notification stream: %w", err)
		}
	}

	return nil
}

// initializeArchivalStreams creates or updates archival streams
func (c *Client) initializeArchivalStreams() error {
	// Define archival streams
	streams := []struct {
		name     string
		subjects []string
	}{
		{"activity_log", []string{"activity.log"}},
		{"thread_metadata", []string{"metadata.thread"}},
		{"thread_access", []string{"access.thread"}},
		{"thread_validations", []string{"validations.thread"}},
	}

	for _, stream := range streams {
		streamConfig := &nats.StreamConfig{
			Name:       stream.name,
			Subjects:   stream.subjects,
			Retention:  nats.WorkQueuePolicy, // Delete after ACK
			MaxAge:     24 * time.Hour,       // Safety retention
			Storage:    nats.FileStorage,
			Replicas:   1,
			Discard:    nats.DiscardOld,
			MaxMsgs:    -1, // Unlimited
			MaxBytes:   -1, // Unlimited
			NoAck:      false,
			Duplicates: 5 * time.Minute,
		}

		// Try to add stream, update if it already exists
		_, err := c.js.AddStream(streamConfig)
		if err != nil {
			// If stream exists, try to update it
			_, err = c.js.UpdateStream(streamConfig)
			if err != nil {
				return fmt.Errorf("failed to create/update archival stream %s: %w", stream.name, err)
			}
		}
	}

	return nil
}

// JetStream returns the JetStream context
func (c *Client) JetStream() nats.JetStreamContext {
	return c.js
}

// Conn returns the underlying NATS connection
func (c *Client) Conn() *nats.Conn {
	return c.conn
}

// Close closes the NATS connection
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// IsConnected returns true if connected to NATS
func (c *Client) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// FetchMessage fetches a single message from a JetStream consumer
func (c *Client) FetchMessage(subject, consumerName string, timeout time.Duration) ([]byte, error) {
	// Create or get durable consumer
	consumerConfig := &nats.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: subject,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       time.Duration(c.cfg.AckWaitSeconds) * time.Second,
		MaxDeliver:    3,
		DeliverPolicy: nats.DeliverNewPolicy,
	}

	// Try to add consumer, ignore if it already exists
	_, err := c.js.AddConsumer(c.cfg.StreamName, consumerConfig)
	if err != nil && err != nats.ErrConsumerNameAlreadyInUse {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Fetch one message
	sub, err := c.js.PullSubscribe(subject, consumerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull subscription: %w", err)
	}
	defer sub.Unsubscribe()

	msgs, err := sub.Fetch(1, nats.MaxWait(timeout))
	if err != nil {
		return nil, err // Timeout or no messages
	}

	if len(msgs) == 0 {
		return nil, fmt.Errorf("no messages available")
	}

	msg := msgs[0]

	// ACK the message
	if err := msg.Ack(); err != nil {
		return nil, fmt.Errorf("failed to ACK message: %w", err)
	}

	return msg.Data, nil
}
