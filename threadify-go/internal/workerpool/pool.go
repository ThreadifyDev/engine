// Package workerpool provides a reusable, bounded worker pool with dynamic scaling.
// It is designed to prevent unbounded goroutine growth while maintaining throughput.
package workerpool

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Job represents a unit of work to be executed by a worker.
// The context provides timeout and cancellation signals.
type Job func(ctx context.Context)

// PoolMetrics defines the interface for recording pool metrics.
// Implementations can integrate with Prometheus, StatsD, or other systems.
type PoolMetrics interface {
	RecordSubmit(poolName string)
	RecordComplete(poolName string, duration time.Duration)
	RecordDrop(poolName string)
	SetQueueDepth(poolName string, depth int)
	SetActiveWorkers(poolName string, count int)
	SetTotalWorkers(poolName string, count int)
}

// NoOpMetrics is a no-op implementation of PoolMetrics for testing or when metrics are disabled.
type NoOpMetrics struct{}

func (n NoOpMetrics) RecordSubmit(string)                  {}
func (n NoOpMetrics) RecordComplete(string, time.Duration) {}
func (n NoOpMetrics) RecordDrop(string)                    {}
func (n NoOpMetrics) SetQueueDepth(string, int)            {}
func (n NoOpMetrics) SetActiveWorkers(string, int)         {}
func (n NoOpMetrics) SetTotalWorkers(string, int)          {}

// Config holds worker pool configuration.
type Config struct {
	// Name identifies the pool for logging and metrics (e.g., "validation", "notification")
	Name string

	// MinWorkers is the minimum number of workers to maintain (default: NumCPU)
	MinWorkers int

	// MaxWorkers is the hard upper bound on workers (default: NumCPU * 4)
	MaxWorkers int

	// QueueSize is the buffered channel capacity (default: 1000)
	QueueSize int

	// ScaleUpThreshold is the queue depth that triggers spawning a new worker (default: 100)
	ScaleUpThreshold int

	// ScaleDownAfter is the idle duration before a worker exits (default: 30s)
	ScaleDownAfter time.Duration

	// JobTimeout is the maximum duration for a single job (default: 60s, 0 = no timeout)
	JobTimeout time.Duration

	// SubmitRetryWait is how long to wait when queue is full before dropping (default: 50ms, 0 = drop immediately)
	SubmitRetryWait time.Duration

	// Metrics is the optional metrics interface (default: NoOpMetrics)
	Metrics PoolMetrics
}

// DefaultConfig returns sensible defaults based on CPU count.
func DefaultConfig(name string) Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             name,
		MinWorkers:       numCPU,
		MaxWorkers:       numCPU * 4,
		QueueSize:        1000,
		ScaleUpThreshold: 100,
		ScaleDownAfter:   30 * time.Second,
		JobTimeout:       60 * time.Second,
		SubmitRetryWait:  50 * time.Millisecond,
		Metrics:          NoOpMetrics{},
	}
}

// Stats contains current pool statistics.
type Stats struct {
	ActiveWorkers int32 // Workers currently executing jobs
	TotalWorkers  int32 // Total workers (active + idle)
	QueueDepth    int   // Jobs waiting in queue
	JobsSubmitted int64 // Total jobs submitted
	JobsCompleted int64 // Total jobs completed
	JobsDropped   int64 // Total jobs dropped due to backpressure
}

// Pool manages a bounded pool of worker goroutines.
type Pool struct {
	config Config
	jobs   chan Job
	quit   chan struct{}
	wg     sync.WaitGroup

	// Atomic counters for lock-free metrics
	activeWorkers atomic.Int32
	totalWorkers  atomic.Int32
	jobsSubmitted atomic.Int64
	jobsCompleted atomic.Int64
	jobsDropped   atomic.Int64

	// Scaling control
	scalerQuit chan struct{}
	started    atomic.Bool
	stopped    atomic.Bool
}

// New creates and starts a new worker pool with the given configuration.
func New(cfg Config) *Pool {
	// Apply defaults for zero values
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = runtime.NumCPU()
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = cfg.MinWorkers * 4
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		cfg.MaxWorkers = cfg.MinWorkers
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.ScaleUpThreshold <= 0 {
		cfg.ScaleUpThreshold = 100
	}
	if cfg.ScaleDownAfter <= 0 {
		cfg.ScaleDownAfter = 30 * time.Second
	}
	if cfg.Metrics == nil {
		cfg.Metrics = NoOpMetrics{}
	}

	p := &Pool{
		config:     cfg,
		jobs:       make(chan Job, cfg.QueueSize),
		quit:       make(chan struct{}),
		scalerQuit: make(chan struct{}),
	}

	// Start minimum workers
	for i := 0; i < cfg.MinWorkers; i++ {
		p.startWorker()
	}

	// No separate scaler goroutine - scaling happens on Submit

	p.started.Store(true)
	return p
}

// Submit adds a job to the queue using the hybrid approach:
// 1. Try non-blocking submit
// 2. If full, wait up to SubmitRetryWait
// 3. If still full, drop the job
// Returns true if job was queued, false if dropped.
func (p *Pool) Submit(job Job) bool {
	if p.stopped.Load() {
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return false
	}

	// Fast path: non-blocking
	select {
	case p.jobs <- job:
		p.jobsSubmitted.Add(1)
		p.config.Metrics.RecordSubmit(p.config.Name)
		queueDepth := len(p.jobs)
		p.config.Metrics.SetQueueDepth(p.config.Name, queueDepth)
		// Scale dynamically based on queue pressure
		p.scaleIfNeeded(queueDepth)
		return true
	default:
	}

	// No retry configured - drop immediately
	if p.config.SubmitRetryWait <= 0 {
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return false
	}

	// Slow path: wait with timeout
	select {
	case p.jobs <- job:
		p.jobsSubmitted.Add(1)
		p.config.Metrics.RecordSubmit(p.config.Name)
		queueDepth := len(p.jobs)
		p.config.Metrics.SetQueueDepth(p.config.Name, queueDepth)
		// Scale dynamically based on queue pressure
		p.scaleIfNeeded(queueDepth)
		return true
	case <-time.After(p.config.SubmitRetryWait):
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return false
	case <-p.quit:
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return false
	}
}

// SubmitWait adds a job, blocking until queued or context is cancelled.
// Use this when job delivery is critical and caller can wait.
func (p *Pool) SubmitWait(ctx context.Context, job Job) error {
	if p.stopped.Load() {
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return context.Canceled
	}

	select {
	case p.jobs <- job:
		p.jobsSubmitted.Add(1)
		p.config.Metrics.RecordSubmit(p.config.Name)
		queueDepth := len(p.jobs)
		p.config.Metrics.SetQueueDepth(p.config.Name, queueDepth)
		// Scale dynamically based on queue pressure
		p.scaleIfNeeded(queueDepth)
		return nil
	case <-ctx.Done():
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return ctx.Err()
	case <-p.quit:
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return context.Canceled
	}
}

// TrySubmit attempts a non-blocking submit. Returns false immediately if queue is full.
// Use this when you want explicit control over backpressure handling.
func (p *Pool) TrySubmit(job Job) bool {
	if p.stopped.Load() {
		return false
	}

	select {
	case p.jobs <- job:
		p.jobsSubmitted.Add(1)
		p.config.Metrics.RecordSubmit(p.config.Name)
		p.config.Metrics.SetQueueDepth(p.config.Name, len(p.jobs))
		return true
	default:
		p.jobsDropped.Add(1)
		p.config.Metrics.RecordDrop(p.config.Name)
		return false
	}
}

// startWorker spawns a new worker goroutine.
func (p *Pool) startWorker() {
	p.wg.Add(1)
	p.totalWorkers.Add(1)
	p.config.Metrics.SetTotalWorkers(p.config.Name, int(p.totalWorkers.Load()))

	go func() {
		defer p.wg.Done()
		defer func() {
			p.totalWorkers.Add(-1)
			p.config.Metrics.SetTotalWorkers(p.config.Name, int(p.totalWorkers.Load()))
		}()

		idleTimer := time.NewTimer(p.config.ScaleDownAfter)
		defer idleTimer.Stop()

		for {
			select {
			case job, ok := <-p.jobs:
				if !ok {
					return // Channel closed
				}

				// Reset idle timer
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(p.config.ScaleDownAfter)

				// Execute job with metrics
				p.activeWorkers.Add(1)
				p.config.Metrics.SetActiveWorkers(p.config.Name, int(p.activeWorkers.Load()))

				startTime := time.Now()
				p.executeJob(job)
				duration := time.Since(startTime)

				p.activeWorkers.Add(-1)
				p.config.Metrics.SetActiveWorkers(p.config.Name, int(p.activeWorkers.Load()))

				p.jobsCompleted.Add(1)
				p.config.Metrics.RecordComplete(p.config.Name, duration)
				p.config.Metrics.SetQueueDepth(p.config.Name, len(p.jobs))

			case <-idleTimer.C:
				// Scale down if above minimum workers
				if p.totalWorkers.Load() > int32(p.config.MinWorkers) {
					return // Exit this worker
				}
				// Reset timer if we're at minimum
				idleTimer.Reset(p.config.ScaleDownAfter)

			case <-p.quit:
				return
			}
		}
	}()
}

// executeJob runs a job with timeout if configured.
func (p *Pool) executeJob(job Job) {
	// Recover from panics to prevent worker death
	defer func() {
		if r := recover(); r != nil {
			// Log panic but keep worker alive
			// In production, you'd want proper logging here
		}
	}()

	if p.config.JobTimeout > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), p.config.JobTimeout)
		defer cancel()
		job(ctx)
	} else {
		job(context.Background())
	}
}

// scaleIfNeeded dynamically scales workers based on queue pressure.
// Formula: targetWorkers = minWorkers + ceil(queueDepth / queuePerWorker)
// where queuePerWorker = queueSize / (maxWorkers - minWorkers)
func (p *Pool) scaleIfNeeded(queueDepth int) {
	if p.stopped.Load() {
		return
	}

	currentWorkers := int(p.totalWorkers.Load())
	minWorkers := p.config.MinWorkers
	maxWorkers := p.config.MaxWorkers

	// If already at max, nothing to do
	if currentWorkers >= maxWorkers {
		return
	}

	// Calculate how many jobs each additional worker should handle
	// This distributes the queue evenly across the worker range
	workerRange := maxWorkers - minWorkers
	if workerRange <= 0 {
		return // No scaling range
	}

	queuePerWorker := p.config.QueueSize / workerRange
	if queuePerWorker <= 0 {
		queuePerWorker = 1 // Avoid division by zero
	}

	// Calculate target workers based on current queue depth
	// Example: 10 min, 30 max, 1000 queue size
	// queuePerWorker = 1000 / (30-10) = 50
	// If queueDepth = 100, targetWorkers = 10 + ceil(100/50) = 12
	additionalWorkers := (queueDepth + queuePerWorker - 1) / queuePerWorker // ceil division
	targetWorkers := minWorkers + additionalWorkers

	// Cap at max workers
	if targetWorkers > maxWorkers {
		targetWorkers = maxWorkers
	}

	// Scale up if needed (one at a time to avoid thundering herd)
	if targetWorkers > currentWorkers {
		p.startWorker()
	}
}

// Stats returns current pool statistics.
func (p *Pool) Stats() Stats {
	return Stats{
		ActiveWorkers: p.activeWorkers.Load(),
		TotalWorkers:  p.totalWorkers.Load(),
		QueueDepth:    len(p.jobs),
		JobsSubmitted: p.jobsSubmitted.Load(),
		JobsCompleted: p.jobsCompleted.Load(),
		JobsDropped:   p.jobsDropped.Load(),
	}
}

// Name returns the pool's configured name.
func (p *Pool) Name() string {
	return p.config.Name
}

// QueueDepth returns the current number of jobs waiting in the queue.
func (p *Pool) QueueDepth() int {
	return len(p.jobs)
}

// IsHealthy returns true if the pool is running and not overloaded.
// Overloaded is defined as queue depth > 80% of capacity.
func (p *Pool) IsHealthy() bool {
	if p.stopped.Load() {
		return false
	}
	return len(p.jobs) < (p.config.QueueSize * 80 / 100)
}

// Shutdown gracefully stops the pool, waiting for in-flight jobs to complete.
// The context can be used to set a deadline for shutdown.
func (p *Pool) Shutdown(ctx context.Context) error {
	if p.stopped.Swap(true) {
		return nil // Already stopped
	}

	// Stop scaler
	close(p.scalerQuit)

	// Signal workers to stop
	close(p.quit)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ShutdownNow stops the pool immediately without waiting for jobs to complete.
// Jobs in the queue will be dropped.
func (p *Pool) ShutdownNow() {
	if p.stopped.Swap(true) {
		return // Already stopped
	}

	close(p.scalerQuit)
	close(p.quit)
	// Don't wait for workers
}
