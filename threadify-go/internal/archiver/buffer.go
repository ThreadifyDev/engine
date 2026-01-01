package archiver

import (
	"sync"
	"time"
)

// StreamEvent represents an event read from a Valkey stream
type StreamEvent struct {
	StreamID string            // Stream entry ID for ACK
	Data     map[string]string // Event data
}

// EventBuffer is a thread-safe buffer for batching events before writing to Postgres
// Used by: consumer.go (adds events), writer.go (reads and clears)
type EventBuffer struct {
	events        []StreamEvent
	mu            sync.RWMutex
	maxSize       int
	maxBufferSize int // Hard limit to prevent unbounded growth
	flushInterval time.Duration
	firstAdded    time.Time
}

// NewEventBuffer creates a new event buffer with size and time-based flush triggers
func NewEventBuffer(maxSize int, flushInterval time.Duration) *EventBuffer {
	return &EventBuffer{
		events:        make([]StreamEvent, 0, maxSize),
		maxSize:       maxSize,
		maxBufferSize: maxSize * 5, // Default: 5x maxSize as hard limit
		flushInterval: flushInterval,
	}
}

// NewEventBufferWithLimit creates a buffer with custom max buffer size
func NewEventBufferWithLimit(maxSize int, maxBufferSize int, flushInterval time.Duration) *EventBuffer {
	return &EventBuffer{
		events:        make([]StreamEvent, 0, maxSize),
		maxSize:       maxSize,
		maxBufferSize: maxBufferSize,
		flushInterval: flushInterval,
	}
}

// Add adds an event to the buffer (thread-safe)
// Returns false if buffer is at max capacity (hard limit)
func (b *EventBuffer) Add(event StreamEvent) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check hard limit to prevent unbounded growth
	if len(b.events) >= b.maxBufferSize {
		return false // Buffer full, cannot add
	}

	// Set first added time if this is the first event
	if len(b.events) == 0 {
		b.firstAdded = time.Now()
	}

	b.events = append(b.events, event)
	return true
}

// ShouldFlush returns true if buffer should be flushed (size or time trigger)
func (b *EventBuffer) ShouldFlush() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.events) == 0 {
		return false
	}

	// Size trigger
	if len(b.events) >= b.maxSize {
		return true
	}

	// Time trigger
	if time.Since(b.firstAdded) >= b.flushInterval {
		return true
	}

	return false
}

// GetAndClear returns all events and clears the buffer (thread-safe)
func (b *EventBuffer) GetAndClear() []StreamEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	events := make([]StreamEvent, len(b.events))
	copy(events, b.events)

	// Clear buffer
	b.events = b.events[:0]
	b.firstAdded = time.Time{}

	return events
}

// Len returns the current number of events in buffer
func (b *EventBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.events)
}
