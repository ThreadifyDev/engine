package archiver

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Stream Consumption Metrics
	StreamMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_stream_messages_consumed_total",
			Help: "Total number of messages consumed from Redis streams by stream name",
		},
		[]string{"stream"},
	)

	StreamConsumptionErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_stream_consumption_errors_total",
			Help: "Total number of stream consumption errors by stream name",
		},
		[]string{"stream"},
	)

	// Batch Processing Metrics
	BatchesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_batches_processed_total",
			Help: "Total number of batches processed by stream and status",
		},
		[]string{"stream", "status"}, // status: success, error
	)

	BatchProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_archiver_batch_processing_duration_seconds",
			Help:    "Batch processing duration in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"stream"},
	)

	BatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_archiver_batch_size",
			Help:    "Number of messages in each batch",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"stream"},
	)

	// Database Write Metrics
	DatabaseWritesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_database_writes_total",
			Help: "Total number of database writes by table and status",
		},
		[]string{"table", "status"},
	)

	DatabaseWriteDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_archiver_database_write_duration_seconds",
			Help:    "Database write operation duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"table"},
	)

	// Pending Messages Metrics
	PendingMessages = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "threadify_archiver_pending_messages",
			Help: "Number of pending messages in stream consumer group",
		},
		[]string{"stream"},
	)

	// Buffer Metrics
	BufferSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "threadify_archiver_buffer_size",
			Help: "Current size of archival buffer",
		},
		[]string{"stream"},
	)

	BufferFlushes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_buffer_flushes_total",
			Help: "Total number of buffer flushes by stream and trigger",
		},
		[]string{"stream", "trigger"}, // trigger: size, time, shutdown
	)

	// Retry Metrics
	RetryAttempts = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_archiver_retry_attempts_total",
			Help: "Total number of retry attempts by stream",
		},
		[]string{"stream"},
	)

	// Lag Metrics
	ConsumerLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "threadify_archiver_consumer_lag_seconds",
			Help: "Consumer lag in seconds (time since message was produced)",
		},
		[]string{"stream"},
	)
)
