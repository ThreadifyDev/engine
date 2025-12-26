package archiver

import (
	"context"
	"fmt"
	"time"
)

// ValkeyClient interface for stream ACK operations
// Implemented by: internal/repository/valkey/client.go
type ValkeyClient interface {
	XAck(ctx context.Context, stream, group string, ids []string) error
}

// WriteFunc is a function that writes events to Postgres
// Returns error if write fails
type WriteFunc func(ctx context.Context, events []StreamEvent) error

// Writer handles batch writing to Postgres with retry logic
// Used by: archiver.go (orchestrates batch writes)
type Writer struct {
	valkeyClient   ValkeyClient
	maxRetries     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// NewWriter creates a new batch writer with retry configuration
func NewWriter(valkeyClient ValkeyClient, maxRetries int, initialBackoff, maxBackoff time.Duration) *Writer {
	return &Writer{
		valkeyClient:   valkeyClient,
		maxRetries:     maxRetries,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
	}
}

// ProcessBatch writes events to Postgres and ACKs them in Valkey stream
// Retries with exponential backoff on failure
func (w *Writer) ProcessBatch(ctx context.Context, stream, group string, events []StreamEvent, writeFunc WriteFunc) error {
	if len(events) == 0 {
		return nil
	}

	// Try to write with retries
	var lastErr error
	backoff := w.initialBackoff

	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}

			// Exponential backoff
			backoff *= 2
			if backoff > w.maxBackoff {
				backoff = w.maxBackoff
			}
		}

		// Attempt write
		err := writeFunc(ctx, events)
		if err == nil {
			// Success - ACK the stream entries
			return w.ackEvents(ctx, stream, group, events)
		}

		lastErr = err
	}

	// Max retries exceeded
	return fmt.Errorf("max retries exceeded after %d attempts: %w", w.maxRetries, lastErr)
}

// ackEvents acknowledges stream entries after successful write
func (w *Writer) ackEvents(ctx context.Context, stream, group string, events []StreamEvent) error {
	ids := make([]string, len(events))
	for i, event := range events {
		ids[i] = event.StreamID
	}

	return w.valkeyClient.XAck(ctx, stream, group, ids)
}
