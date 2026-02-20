package archiver

import "time"

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
