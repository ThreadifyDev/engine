package archiver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/database"
)

// StreamEvent represents an event from a NATS stream
type StreamEvent struct {
	StreamID string
	Data     map[string]string
}

// NATSConsumer consumes archival events from NATS JetStream
type NATSConsumer struct {
	js           jetstream.JetStream
	db           *database.PostgresDB
	writer       *PostgresWriter
	batchSize    int
	batchTimeout time.Duration
	consumerName string
	stopChan     chan struct{}
}

// NewNATSConsumer creates a new NATS consumer for archival
func NewNATSConsumer(nc *nats.Conn, db *database.PostgresDB, batchSize int, batchTimeout time.Duration, consumerName string) (*NATSConsumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return &NATSConsumer{
		js:           js,
		db:           db,
		writer:       NewPostgresWriter(db),
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		consumerName: consumerName,
		stopChan:     make(chan struct{}),
	}, nil
}

// Start begins consuming from all archival streams
func (c *NATSConsumer) Start(ctx context.Context) error {
	fmt.Printf("🚀 Starting NATS archival consumer: %s\n", c.consumerName)

	// Start consumers for each stream
	go c.consumeStream(ctx, "activity_log", "activity.log", c.processActivityLog)
	go c.consumeStream(ctx, "thread_metadata", "metadata.thread", c.processThreadMetadata)
	go c.consumeStream(ctx, "thread_access", "access.thread", c.processThreadAccess)
	go c.consumeStream(ctx, "thread_validations", "validations.thread", c.processThreadValidations)

	fmt.Printf("✅ NATS archival consumer started successfully\n")
	return nil
}

// Stop gracefully stops the consumer
func (c *NATSConsumer) Stop() {
	fmt.Printf("🛑 Stopping NATS archival consumer...\n")
	close(c.stopChan)
}

// consumeStream consumes messages from a specific NATS stream
func (c *NATSConsumer) consumeStream(ctx context.Context, streamName, subject string, processor func(context.Context, []jetstream.Msg) error) {
	fmt.Printf("📡 Starting consumer for stream: %s (subject: %s)\n", streamName, subject)

	// Create or get consumer
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable:       fmt.Sprintf("archiver-%s", streamName),
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    10,
		AckWait:       30 * time.Second,
		FilterSubject: subject,
	})
	if err != nil {
		fmt.Printf("❌ Failed to create consumer for %s: %v\n", streamName, err)
		return
	}

	// Batch processing
	batch := make([]jetstream.Msg, 0, c.batchSize)
	ticker := time.NewTicker(c.batchTimeout)
	defer ticker.Stop()

	// Consume messages using iterator
	iter, err := consumer.Messages()
	if err != nil {
		fmt.Printf("❌ Failed to get message iterator for %s: %v\n", streamName, err)
		return
	}
	defer iter.Stop()

	// Process messages in goroutine
	msgChan := make(chan jetstream.Msg, c.batchSize)
	go func() {
		for {
			msg, err := iter.Next()
			if err != nil {
				if err == context.Canceled || err == context.DeadlineExceeded {
					close(msgChan)
					return
				}
				fmt.Printf("⚠️ Error fetching message from %s: %v\n", streamName, err)
				time.Sleep(time.Second)
				continue
			}
			msgChan <- msg
		}
	}()

	for {
		select {
		case <-c.stopChan:
			// Process remaining batch before stopping
			if len(batch) > 0 {
				if err := processor(ctx, batch); err != nil {
					fmt.Printf("❌ Failed to process final batch for %s: %v\n", streamName, err)
				}
			}
			return

		case <-ticker.C:
			// Process batch on timeout
			if len(batch) > 0 {
				if err := processor(ctx, batch); err != nil {
					fmt.Printf("❌ Failed to process batch for %s: %v\n", streamName, err)
					// NACK messages for retry
					for _, msg := range batch {
						msg.Nak()
					}
				} else {
					// ACK messages on success
					for _, msg := range batch {
						msg.Ack()
					}
				}
				batch = batch[:0] // Clear batch
			}

		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			batch = append(batch, msg)

			// Process batch when full
			if len(batch) >= c.batchSize {
				if err := processor(ctx, batch); err != nil {
					fmt.Printf("❌ Failed to process batch for %s: %v\n", streamName, err)
					// NACK messages for retry
					for _, msg := range batch {
						msg.Nak()
					}
				} else {
					// ACK messages on success
					for _, msg := range batch {
						msg.Ack()
					}
				}
				batch = batch[:0] // Clear batch
			}
		}
	}
}

// convertToStringMap converts map[string]interface{} to map[string]string
func convertToStringMap(data map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range data {
		result[k] = fmt.Sprintf("%v", v)
	}
	return result
}

// processActivityLog processes activity log messages
func (c *NATSConsumer) processActivityLog(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}

	start := time.Now()
	fmt.Printf("📝 Processing %d activity log messages\n", len(msgs))

	// Convert NATS messages to StreamEvent format for existing writer
	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			fmt.Printf("❌ Failed to unmarshal activity log message: %v\n", err)
			continue
		}

		events = append(events, StreamEvent{
			StreamID: msg.Subject(),
			Data:     convertToStringMap(data),
		})
	}

	// Use existing Postgres writer
	err := c.writer.WriteActivityLog(ctx, events)
	duration := time.Since(start)
	fmt.Printf("⏱️  [NATS-PERF] Processed %d activity_log messages in %v (%.2f msg/s)\n",
		len(msgs), duration, float64(len(msgs))/duration.Seconds())
	return err
}

// processThreadMetadata processes thread metadata messages
func (c *NATSConsumer) processThreadMetadata(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}

	start := time.Now()
	fmt.Printf("📝 Processing %d thread metadata messages\n", len(msgs))

	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			fmt.Printf("❌ Failed to unmarshal thread metadata message: %v\n", err)
			continue
		}

		events = append(events, StreamEvent{
			StreamID: msg.Subject(),
			Data:     convertToStringMap(data),
		})
	}

	err := c.writer.WriteThreadMetadata(ctx, events)
	duration := time.Since(start)
	fmt.Printf("⏱️  [NATS-PERF] Processed %d metadata.thread messages in %v (%.2f msg/s)\n",
		len(msgs), duration, float64(len(msgs))/duration.Seconds())
	return err
}

// processThreadAccess processes thread access messages
func (c *NATSConsumer) processThreadAccess(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}

	start := time.Now()
	fmt.Printf("📝 Processing %d thread access messages\n", len(msgs))

	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			fmt.Printf("❌ Failed to unmarshal thread access message: %v\n", err)
			continue
		}

		events = append(events, StreamEvent{
			StreamID: msg.Subject(),
			Data:     convertToStringMap(data),
		})
	}

	err := c.writer.WriteThreadAccess(ctx, events)
	duration := time.Since(start)
	fmt.Printf("⏱️  [NATS-PERF] Processed %d access.thread messages in %v (%.2f msg/s)\n",
		len(msgs), duration, float64(len(msgs))/duration.Seconds())
	return err
}

// processThreadValidations processes thread validation messages
func (c *NATSConsumer) processThreadValidations(ctx context.Context, msgs []jetstream.Msg) error {
	if len(msgs) == 0 {
		return nil
	}

	start := time.Now()
	fmt.Printf("📝 Processing %d thread validation messages\n", len(msgs))

	events := make([]StreamEvent, 0, len(msgs))
	for _, msg := range msgs {
		var data map[string]interface{}
		if err := json.Unmarshal(msg.Data(), &data); err != nil {
			fmt.Printf("❌ Failed to unmarshal thread validation message: %v\n", err)
			continue
		}

		events = append(events, StreamEvent{
			StreamID: msg.Subject(),
			Data:     convertToStringMap(data),
		})
	}

	err := c.writer.WriteValidationResults(ctx, events)
	duration := time.Since(start)
	fmt.Printf("⏱️  [NATS-PERF] Processed %d validations.thread messages in %v (%.2f msg/s)\n",
		len(msgs), duration, float64(len(msgs))/duration.Seconds())
	return err
}
