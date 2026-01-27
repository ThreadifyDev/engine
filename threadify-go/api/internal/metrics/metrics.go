package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Web API Metrics
var (
	// HTTP Request Metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_api_http_requests_total",
			Help: "Total number of HTTP requests by method, path, and status",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_api_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Authentication Metrics
	AuthAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_api_auth_attempts_total",
			Help: "Total number of authentication attempts by type and status",
		},
		[]string{"type", "status"}, // type: login, signup, verify_otp
	)

	// Service Account Metrics
	ServiceAccountsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_service_accounts_created_total",
			Help: "Total number of service accounts created",
		},
	)

	APIKeysCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_keys_created_total",
			Help: "Total number of API keys created",
		},
	)

	// Contract Metrics
	ContractsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_contracts_created_total",
			Help: "Total number of contracts created",
		},
	)

	ContractVersionsCreated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "threadify_api_contract_versions_created_total",
			Help: "Total number of contract versions created",
		},
	)

	// Database Operation Metrics
	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_api_database_operations_total",
			Help: "Total number of database operations by operation and status",
		},
		[]string{"operation", "status"},
	)

	DatabaseOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_api_database_operation_duration_seconds",
			Help:    "Database operation duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		},
		[]string{"operation"},
	)

	// Error Metrics
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_api_errors_total",
			Help: "Total number of errors by type",
		},
		[]string{"error_type"},
	)
)
