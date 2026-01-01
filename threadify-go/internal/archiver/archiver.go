package archiver

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BufferConfig holds buffer configuration
type BufferConfig struct {
	Size          int
	FlushInterval time.Duration
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

// StreamConfig holds stream configuration
type StreamConfig struct {
	ConsumerGroup string
	BlockTimeout  time.Duration
	BatchSize     int
}

// ArchiverConfig holds complete archiver configuration
type ArchiverConfig struct {
	Buffers map[string]BufferConfig
	Retry   RetryConfig
	Streams StreamConfig
}

// QueueHandler holds queue-specific components
type QueueHandler struct {
	streamName string
	buffer     *EventBuffer
	consumer   *Consumer
	writer     *Writer
	writeFunc  WriteFunc
}

// Archiver orchestrates consumers, buffers, and writers for multiple queues
// Main entry point for the archiver service
type Archiver struct {
	config       ArchiverConfig
	reader       StreamReader
	valkeyClient ValkeyClient
	instanceName string
	queues       map[string]*QueueHandler
	mu           sync.RWMutex
	wg           sync.WaitGroup
}

// NewArchiver creates a new archiver instance
func NewArchiver(config ArchiverConfig, reader StreamReader, valkeyClient ValkeyClient, instanceName string) *Archiver {
	return &Archiver{
		config:       config,
		reader:       reader,
		valkeyClient: valkeyClient,
		instanceName: instanceName,
		queues:       make(map[string]*QueueHandler),
	}
}

// RegisterQueue registers a queue for archiving (adds "streams:" prefix)
func (a *Archiver) RegisterQueue(queueName string, bufferSize int, flushInterval time.Duration, writeFunc WriteFunc) {
	streamName := fmt.Sprintf("streams:%s", queueName)
	a.RegisterStream(queueName, streamName, bufferSize, flushInterval, writeFunc)
}

// RegisterStream registers a stream with full stream name (no prefix added)
func (a *Archiver) RegisterStream(queueName string, streamName string, bufferSize int, flushInterval time.Duration, writeFunc WriteFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()

	consumerName := fmt.Sprintf("%s-%s", a.instanceName, queueName)

	// Create buffer
	buffer := NewEventBuffer(bufferSize, flushInterval)

	// Create consumer
	consumer := NewConsumer(
		streamName,
		a.config.Streams.ConsumerGroup,
		consumerName,
		a.reader,
		buffer,
		a.config.Streams.BatchSize,
		a.config.Streams.BlockTimeout,
	)

	// Create writer
	writer := NewWriter(
		a.valkeyClient,
		a.config.Retry.MaxAttempts,
		a.config.Retry.InitialBackoff,
		a.config.Retry.MaxBackoff,
	)

	a.queues[queueName] = &QueueHandler{
		streamName: streamName,
		buffer:     buffer,
		consumer:   consumer,
		writer:     writer,
		writeFunc:  writeFunc,
	}
}

// Start starts all consumers and flush monitors
func (a *Archiver) Start(ctx context.Context) {
	a.mu.RLock()
	// Copy queues map to avoid holding lock during goroutine startup
	queuesCopy := make(map[string]*QueueHandler, len(a.queues))
	for k, v := range a.queues {
		queuesCopy[k] = v
	}
	a.mu.RUnlock()

	for queueName, handler := range queuesCopy {
		a.startQueueConsumer(ctx, queueName, handler)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Wait for all goroutines to finish
	a.wg.Wait()
}

// StartQueueConsumer starts consumer and flush monitor for a specific queue
// Can be called dynamically after Start() for new queues
func (a *Archiver) StartQueueConsumer(ctx context.Context, queueName string) error {
	a.mu.RLock()
	handler, exists := a.queues[queueName]
	a.mu.RUnlock()

	if !exists {
		return fmt.Errorf("queue not found: %s", queueName)
	}

	a.startQueueConsumer(ctx, queueName, handler)
	return nil
}

// startQueueConsumer is the internal method to start consumer and monitor
func (a *Archiver) startQueueConsumer(ctx context.Context, queueName string, handler *QueueHandler) {
	// Start consumer goroutine
	a.wg.Add(1)
	go func(name string, h *QueueHandler) {
		defer a.wg.Done()
		fmt.Printf("Starting consumer for queue: %s\n", name)
		h.consumer.Run(ctx)
		fmt.Printf("Consumer stopped for queue: %s\n", name)
	}(queueName, handler)

	// Start flush monitor goroutine
	a.wg.Add(1)
	go func(name string, h *QueueHandler) {
		defer a.wg.Done()
		fmt.Printf("Starting flush monitor for queue: %s\n", name)
		a.monitorFlush(ctx, name, h)
		fmt.Printf("Flush monitor stopped for queue: %s\n", name)
	}(queueName, handler)
}

// monitorFlush monitors buffer and flushes when needed
func (a *Archiver) monitorFlush(ctx context.Context, queueName string, handler *QueueHandler) {
	// Use the flush interval from buffer config, or default to 1 second
	checkInterval := handler.buffer.flushInterval
	if checkInterval == 0 {
		checkInterval = 1 * time.Second
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before exit
			a.flush(ctx, queueName, handler)
			return
		case <-ticker.C:
			if handler.buffer.ShouldFlush() {
				a.flush(ctx, queueName, handler)
			}
		}
	}
}

// flush flushes buffer to Postgres
func (a *Archiver) flush(ctx context.Context, queueName string, handler *QueueHandler) {
	events := handler.buffer.GetAndClear()
	if len(events) == 0 {
		return
	}

	fmt.Printf("Flushing %d events from queue: %s\n", len(events), queueName)

	err := handler.writer.ProcessBatch(
		ctx,
		handler.streamName,
		a.config.Streams.ConsumerGroup,
		events,
		handler.writeFunc,
	)

	if err != nil {
		fmt.Printf("Error flushing queue %s: %v\n", queueName, err)
		// Events are not ACKed, will be retried on next consumer read
	}
}
