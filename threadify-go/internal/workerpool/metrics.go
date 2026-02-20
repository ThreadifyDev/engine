package workerpool

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// PrometheusMetrics implements PoolMetrics using Prometheus.
type PrometheusMetrics struct {
	jobsSubmitted *prometheus.CounterVec
	jobsCompleted *prometheus.CounterVec
	jobsDropped   *prometheus.CounterVec
	queueDepth    *prometheus.GaugeVec
	activeWorkers *prometheus.GaugeVec
	totalWorkers  *prometheus.GaugeVec
	jobDuration   *prometheus.HistogramVec
}

// NewPrometheusMetrics creates a new PrometheusMetrics instance.
// All metrics are registered with the default Prometheus registry.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		jobsSubmitted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "workerpool_jobs_submitted_total",
				Help: "Total number of jobs submitted to the worker pool",
			},
			[]string{"pool"},
		),
		jobsCompleted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "workerpool_jobs_completed_total",
				Help: "Total number of jobs completed by the worker pool",
			},
			[]string{"pool"},
		),
		jobsDropped: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "workerpool_jobs_dropped_total",
				Help: "Total number of jobs dropped due to backpressure",
			},
			[]string{"pool"},
		),
		queueDepth: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "workerpool_queue_depth",
				Help: "Current number of jobs waiting in the queue",
			},
			[]string{"pool"},
		),
		activeWorkers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "workerpool_active_workers",
				Help: "Current number of workers actively processing jobs",
			},
			[]string{"pool"},
		),
		totalWorkers: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "workerpool_total_workers",
				Help: "Current total number of workers (active + idle)",
			},
			[]string{"pool"},
		),
		jobDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "workerpool_job_duration_seconds",
				Help:    "Duration of job execution in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
			},
			[]string{"pool"},
		),
	}
}

// RecordSubmit increments the jobs submitted counter.
func (m *PrometheusMetrics) RecordSubmit(poolName string) {
	m.jobsSubmitted.WithLabelValues(poolName).Inc()
}

// RecordComplete increments the jobs completed counter and records duration.
func (m *PrometheusMetrics) RecordComplete(poolName string, duration time.Duration) {
	m.jobsCompleted.WithLabelValues(poolName).Inc()
	m.jobDuration.WithLabelValues(poolName).Observe(duration.Seconds())
}

// RecordDrop increments the jobs dropped counter.
func (m *PrometheusMetrics) RecordDrop(poolName string) {
	m.jobsDropped.WithLabelValues(poolName).Inc()
}

// SetQueueDepth sets the current queue depth gauge.
func (m *PrometheusMetrics) SetQueueDepth(poolName string, depth int) {
	m.queueDepth.WithLabelValues(poolName).Set(float64(depth))
}

// SetActiveWorkers sets the current active workers gauge.
func (m *PrometheusMetrics) SetActiveWorkers(poolName string, count int) {
	m.activeWorkers.WithLabelValues(poolName).Set(float64(count))
}

// SetTotalWorkers sets the current total workers gauge.
func (m *PrometheusMetrics) SetTotalWorkers(poolName string, count int) {
	m.totalWorkers.WithLabelValues(poolName).Set(float64(count))
}

// NewPrometheusMetricsWithRegistry creates metrics registered to a custom registry.
// Useful for testing or when you need isolated metrics.
func NewPrometheusMetricsWithRegistry(reg prometheus.Registerer) *PrometheusMetrics {
	jobsSubmitted := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workerpool_jobs_submitted_total",
			Help: "Total number of jobs submitted to the worker pool",
		},
		[]string{"pool"},
	)
	jobsCompleted := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workerpool_jobs_completed_total",
			Help: "Total number of jobs completed by the worker pool",
		},
		[]string{"pool"},
	)
	jobsDropped := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "workerpool_jobs_dropped_total",
			Help: "Total number of jobs dropped due to backpressure",
		},
		[]string{"pool"},
	)
	queueDepth := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workerpool_queue_depth",
			Help: "Current number of jobs waiting in the queue",
		},
		[]string{"pool"},
	)
	activeWorkers := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workerpool_active_workers",
			Help: "Current number of workers actively processing jobs",
		},
		[]string{"pool"},
	)
	totalWorkers := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "workerpool_total_workers",
			Help: "Current total number of workers (active + idle)",
		},
		[]string{"pool"},
	)
	jobDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "workerpool_job_duration_seconds",
			Help:    "Duration of job execution in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15),
		},
		[]string{"pool"},
	)

	reg.MustRegister(jobsSubmitted, jobsCompleted, jobsDropped, queueDepth, activeWorkers, totalWorkers, jobDuration)

	return &PrometheusMetrics{
		jobsSubmitted: jobsSubmitted,
		jobsCompleted: jobsCompleted,
		jobsDropped:   jobsDropped,
		queueDepth:    queueDepth,
		activeWorkers: activeWorkers,
		totalWorkers:  totalWorkers,
		jobDuration:   jobDuration,
	}
}
