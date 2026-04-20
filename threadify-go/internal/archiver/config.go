package archiver

import "time"

type BufferConfig struct {
	Size          int
	FlushInterval time.Duration
}

type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type StreamConfig struct {
	ConsumerGroup string
	BlockTimeout  time.Duration
	BatchSize     int
}

type ArchiverConfig struct {
	Buffers map[string]BufferConfig
	Retry   RetryConfig
	Streams StreamConfig
}
