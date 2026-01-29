package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Engine Metrics
var (
	// HTTP/WebSocket Metrics
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_requests_total",
			Help: "Total number of requests by action and status",
		},
		[]string{"action", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
		},
		[]string{"action"},
	)

	// WebSocket Connection Metrics
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "threadify_active_connections",
			Help: "Number of active WebSocket connections",
		},
	)

	// Thread Operation Metrics
	ThreadsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_threads_created_total",
			Help: "Total number of threads created",
		},
	)

	ThreadsCompleted = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_threads_completed_total",
			Help: "Total number of threads completed",
		},
	)

	StepsRecorded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_steps_recorded_total",
			Help: "Total number of steps recorded by status",
		},
		[]string{"status"},
	)

	// NATS Metrics
	NATSPublishTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_nats_publish_total",
			Help: "Total number of NATS publishes by subject and status",
		},
		[]string{"subject", "status"},
	)

	NATSPublishDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_nats_publish_duration_seconds",
			Help:    "NATS publish duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"subject"},
	)

	// Redis/Valkey Metrics
	RedisOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_redis_operations_total",
			Help: "Total number of Redis operations by operation and status",
		},
		[]string{"operation", "status"},
	)

	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_redis_operation_duration_seconds",
			Help:    "Redis operation duration in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
		},
		[]string{"operation"},
	)

	// API Key Cache Metrics
	APIKeyCacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_key_cache_hits_total",
			Help: "Total number of API key cache hits",
		},
	)

	APIKeyCacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_key_cache_misses_total",
			Help: "Total number of API key cache misses",
		},
	)

	// Validation Metrics
	ValidationViolations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_validation_violations_total",
			Help: "Total number of validation violations by severity",
		},
		[]string{"severity"},
	)

	// Error Metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"error_type"},
	)

	// Granular Operation Metrics for Performance Analysis
	OperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_operation_duration_seconds",
			Help:    "Duration of specific operations within handlers",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"handler", "operation"},
	)
)
