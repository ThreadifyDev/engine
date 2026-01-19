package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// NotificationsSent tracks total notifications sent to clients
	NotificationsSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_notifications_sent_total",
			Help: "Total number of notifications sent to clients",
		},
		[]string{"owner_id", "step_name", "contract", "status"},
	)

	// NotificationsAcked tracks total notifications ACKed by clients
	NotificationsAcked = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_notifications_acked_total",
			Help: "Total number of notifications ACKed by clients",
		},
		[]string{"owner_id"},
	)

	// NotificationAckLatency tracks time from send to ACK
	NotificationAckLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "threadify_notification_ack_latency_seconds",
			Help:    "Time from notification send to client ACK",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
		},
		[]string{"owner_id"},
	)

	// ActiveConsumers tracks number of active NATS consumers
	ActiveConsumers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "threadify_active_consumers",
			Help: "Number of active NATS consumers per owner",
		},
		[]string{"owner_id"},
	)

	// ConsumerCreated tracks consumer creation events
	ConsumerCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_consumers_created_total",
			Help: "Total number of NATS consumers created",
		},
		[]string{"owner_id"},
	)

	// ConsumerDeleted tracks consumer deletion events
	ConsumerDeleted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_consumers_deleted_total",
			Help: "Total number of NATS consumers deleted",
		},
		[]string{"owner_id"},
	)

	// BackpressureEvents tracks when MaxAckPending is reached
	BackpressureEvents = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_backpressure_events_total",
			Help: "Number of times backpressure was triggered (MaxAckPending reached)",
		},
		[]string{"owner_id", "session_id"},
	)

	// FilterSubjectsUpdated tracks subscription changes
	FilterSubjectsUpdated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_filter_subjects_updated_total",
			Help: "Number of times FilterSubjects were updated",
		},
		[]string{"owner_id", "session_id"},
	)

	// PushErrors tracks errors during notification push
	PushErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_push_errors_total",
			Help: "Number of errors during notification push to clients",
		},
		[]string{"owner_id", "session_id", "error_type"},
	)

	// RateLimitExceeded tracks rate limit hits
	RateLimitExceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_rate_limit_exceeded_total",
			Help: "Number of times consumer rate limit was exceeded",
		},
		[]string{"owner_id"},
	)

	// DeadLetterMessages tracks messages moved to DLQ
	DeadLetterMessages = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "threadify_dead_letter_messages_total",
			Help: "Number of messages moved to dead letter queue",
		},
		[]string{"owner_id", "step_name", "reason"},
	)
)
