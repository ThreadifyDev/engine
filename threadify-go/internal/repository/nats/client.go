package nats

import (
	"fmt"
	"log"
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
	nc, err := nats.Connect(
		cfg.URL,
		nats.Name(cfg.ClientID),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Printf("Failed to connect to NATS: %v", err)
		return nil, fmt.Errorf("failed to connect to message broker")
	}

	// Create JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		log.Printf("Failed to create JetStream context: %v", err)
		return nil, fmt.Errorf("failed to initialize message broker")
	}

	client := &Client{
		conn: nc,
		js:   js,
		cfg:  cfg,
	}

	// Initialize notification stream
	if err := client.initializeNotificationStream(); err != nil {
		nc.Close()
		log.Printf("Failed to initialize notification stream: %v", err)
		return nil, fmt.Errorf("failed to initialize notifications")
	}

	// Initialize dead letter queue for failed notifications
	if err := client.initializeDeadLetterQueue(); err != nil {
		nc.Close()
		log.Printf("Failed to initialize dead letter queue: %v", err)
		return nil, fmt.Errorf("failed to initialize DLQ")
	}

	// Initialize archival streams
	if err := client.initializeArchivalStreams(); err != nil {
		nc.Close()
		log.Printf("Failed to initialize archival streams: %v", err)
		return nil, fmt.Errorf("failed to initialize archival system")
	}

	return client, nil
}

// initializeNotificationStream creates or updates the notification stream
func (c *Client) initializeNotificationStream() error {
	streamConfig := &nats.StreamConfig{
		Name:       c.cfg.StreamName,
		Subjects:   []string{"notifications.user.>"},
		Retention:  nats.WorkQueuePolicy, // Delete after consumer ACK
		MaxAge:     3 * 24 * time.Hour,   // 3 days
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

	log.Printf("[NATS] Initialized NOTIFICATIONS stream with subjects: notifications.user.>, retention: 3 days")
	return nil
}

// initializeDeadLetterQueue creates or updates the dead letter queue stream
func (c *Client) initializeDeadLetterQueue() error {
	streamConfig := &nats.StreamConfig{
		Name:      "NOTIFICATIONS_DLQ",
		Subjects:  []string{"notifications.dlq.>"},
		Retention: nats.LimitsPolicy,  // Keep messages (not WorkQueue)
		MaxAge:    7 * 24 * time.Hour, // 7 days retention
		Storage:   nats.FileStorage,
		Replicas:  1,
		Discard:   nats.DiscardOld,
		MaxMsgs:   10000,             // Limit to 10k failed messages
		MaxBytes:  100 * 1024 * 1024, // 100MB max
		NoAck:     false,
	}

	// Try to add stream, update if it already exists
	_, err := c.js.AddStream(streamConfig)
	if err != nil {
		// If stream exists, try to update it
		_, err = c.js.UpdateStream(streamConfig)
		if err != nil {
			return fmt.Errorf("failed to create/update DLQ stream: %w", err)
		}
	}

	log.Printf("[NATS] Initialized NOTIFICATIONS_DLQ stream (7 days retention, 10k messages max)")
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
		{"step_state", []string{"state.step"}},
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
