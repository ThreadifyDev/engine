package archiver

import (
	"context"
	"fmt"
	"time"
)

// StreamReader interface for reading from Valkey streams
// Implemented by: internal/repository/valkey/client.go
type StreamReader interface {
	XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]StreamEvent, error)
}

// Consumer reads events from a Valkey stream and adds them to a buffer
// Used by: archiver.go (orchestrates multiple consumers)
type Consumer struct {
	streamName   string
	groupName    string
	consumerName string
	reader       StreamReader
	buffer       *EventBuffer
	batchSize    int
	blockTimeout time.Duration
}

// NewConsumer creates a new stream consumer
func NewConsumer(streamName, groupName, consumerName string, reader StreamReader, buffer *EventBuffer, batchSize int, blockTimeout time.Duration) *Consumer {
	return &Consumer{
		streamName:   streamName,
		groupName:    groupName,
		consumerName: consumerName,
		reader:       reader,
		buffer:       buffer,
		batchSize:    batchSize,
		blockTimeout: blockTimeout,
	}
}

// Read reads a batch of events from the stream and adds them to the buffer
func (c *Consumer) Read(ctx context.Context) error {
	readStart := time.Now()
	events, err := c.reader.XReadGroup(ctx, c.groupName, c.consumerName, c.streamName, c.batchSize, c.blockTimeout)
	if err != nil {
		return fmt.Errorf("failed to read from stream %s: %w", c.streamName, err)
	}

	if len(events) > 0 {
		readDuration := time.Since(readStart)
		fmt.Printf("📥 [Consumer %s] Read %d events from stream %s in %v\n", c.consumerName, len(events), c.streamName, readDuration)

		// Add events to buffer with timing
		for i, event := range events {
			if !c.buffer.Add(event) {
				fmt.Printf("⚠️  [Consumer %s] Buffer full, dropping event %s\n", c.consumerName, event.StreamID)
				// TODO: Implement backpressure or retry mechanism
				continue
			}
			// Log first event details for timing analysis
			if i == 0 {
				fmt.Printf("   ⏱️  First event StreamID: %s (received at %s)\n", event.StreamID, time.Now().Format("15:04:05.000"))
			}
		}
	}

	return nil
}

// Run continuously reads from the stream until context is cancelled
func (c *Consumer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := c.Read(ctx); err != nil {
				// Log error but continue (transient errors shouldn't stop consumer)
				fmt.Printf("Consumer %s error: %v\n", c.consumerName, err)
				// Brief pause before retry
				time.Sleep(1 * time.Second)
			}
		}
	}
}
