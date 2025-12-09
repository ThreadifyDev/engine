package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	activeConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_connections",
			Help: "Number of active connections",
		},
	)

	contractValidationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "contract_validation_total",
			Help: "Total number of contract validations",
		},
		[]string{"status"},
	)

	contractVersionsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "contract_versions_created_total",
			Help: "Total number of contract versions created",
		},
	)

	websocketConnectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "websocket_connections_total",
			Help: "Total number of WebSocket connections",
		},
		[]string{"action"},
	)

	activeWebsocketConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_websocket_connections",
			Help: "Number of active WebSocket connections",
		},
	)

	databaseQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_query_duration_seconds",
			Help:    "Database query latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	redisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "redis_operation_duration_seconds",
			Help:    "Redis operation latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
)

// PrometheusMiddleware records HTTP metrics
func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		activeConnections.Inc()
		defer activeConnections.Dec()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path, status).Observe(duration)
	}
}

// RecordContractValidation records contract validation metrics
func RecordContractValidation(isValid bool) {
	status := "success"
	if !isValid {
		status = "failed"
	}
	contractValidationTotal.WithLabelValues(status).Inc()
}

// RecordContractVersionCreated increments contract version counter
func RecordContractVersionCreated() {
	contractVersionsCreated.Inc()
}

// RecordWebSocketConnection records WebSocket connection metrics
func RecordWebSocketConnection(action string) {
	websocketConnectionsTotal.WithLabelValues(action).Inc()
}

// IncrementActiveWebSocketConnections increments active WebSocket connections
func IncrementActiveWebSocketConnections() {
	activeWebsocketConnections.Inc()
}

// DecrementActiveWebSocketConnections decrements active WebSocket connections
func DecrementActiveWebSocketConnections() {
	activeWebsocketConnections.Dec()
}

// RecordDatabaseQuery records database query duration
func RecordDatabaseQuery(operation string, duration time.Duration) {
	databaseQueryDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordRedisOperation records Redis operation duration
func RecordRedisOperation(operation string, duration time.Duration) {
	redisOperationDuration.WithLabelValues(operation).Observe(duration.Seconds())
}
