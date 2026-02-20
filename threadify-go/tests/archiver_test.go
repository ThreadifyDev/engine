package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestArchiver_StartStop(t *testing.T) {
	// Create mock dependencies
	mockReader := new(MockStreamReader)
	mockValkey := new(MockValkeyClient)

	// Create archiver with minimal config
	config := ArchiverConfig{
		Streams: StreamConfig{
			ConsumerGroup: "test-group",
			BlockTimeout:  1 * time.Second,
			BatchSize:     10,
		},
		Buffers: map[string]BufferConfig{
			"test_stream": {
				Size:          10,
				FlushInterval: 1 * time.Second,
			},
		},
		Retry: RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     1 * time.Second,
		},
	}

	// Mock stream reads (return empty to avoid processing)
	mockReader.On("XReadGroup", context.Background(), "test-group", "archiver-test_stream", "streams:test_stream", 10, 1*time.Second).
		Return([]StreamEvent{}, nil).Maybe()

	archiver := NewArchiver(config, mockReader, mockValkey, "archiver")

	// Start archiver
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should start and stop cleanly
	archiver.Start(ctx)

	// Wait for context to expire
	<-ctx.Done()

	// No assertions needed - just verify it doesn't panic or hang
	assert.True(t, true)
}

func TestArchiver_RegisterQueue(t *testing.T) {
	mockReader := new(MockStreamReader)
	mockValkey := new(MockValkeyClient)

	config := ArchiverConfig{
		Streams: StreamConfig{
			ConsumerGroup: "test-group",
			BlockTimeout:  1 * time.Second,
			BatchSize:     10,
		},
		Buffers: map[string]BufferConfig{},
		Retry: RetryConfig{
			MaxAttempts:    3,
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     1 * time.Second,
		},
	}

	archiver := NewArchiver(config, mockReader, mockValkey, "archiver")

	// Register a queue
	writeFunc := func(ctx context.Context, events []StreamEvent) error {
		return nil
	}

	archiver.RegisterQueue("test_queue", 10, 1*time.Second, writeFunc)

	// Verify queue was registered
	assert.Contains(t, archiver.queues, "test_queue")
}
