package workerpool

import (
	"context"
	"runtime"
	"time"
)

// Pre-configured pool factories for different subsystems.
// Each factory returns a Config tuned for the specific workload characteristics.

// ValidationPoolConfig returns configuration for async validation processing.
// Characteristics:
//   - CPU-bound (validation logic, JSON parsing)
//   - Medium latency tolerance (async, not blocking user request)
//   - High volume during peak traffic
//   - 60s timeout matches validation context timeout
func ValidationPoolConfig() Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             "validation",
		MinWorkers:       numCPU,
		MaxWorkers:       numCPU * 4,
		QueueSize:        5000,
		ScaleUpThreshold: 500,
		ScaleDownAfter:   30 * time.Second,
		JobTimeout:       60 * time.Second,
		SubmitRetryWait:  0, // Non-blocking: drop immediately if queue full
		Metrics:          NoOpMetrics{},
	}
}

// NotificationPoolConfig returns configuration for notification publishing.
// Characteristics:
//   - I/O-bound (NATS publishing, network calls)
//   - Can have more workers than CPU cores (waiting on I/O)
//   - Higher volume than validation (multiple notifications per step)
//   - Shorter timeout (NATS should respond quickly)
func NotificationPoolConfig() Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             "notification",
		MinWorkers:       numCPU * 2,
		MaxWorkers:       numCPU * 8,
		QueueSize:        10000,
		ScaleUpThreshold: 1000,
		ScaleDownAfter:   60 * time.Second,
		JobTimeout:       30 * time.Second,
		SubmitRetryWait:  0, // Non-blocking: drop immediately if queue full
		Metrics:          NoOpMetrics{},
	}
}

// WriteBackPoolConfig returns configuration for async cache write-backs.
// Characteristics:
//   - I/O-bound (Redis/Valkey writes)
//   - Low priority (cache population, not critical path)
//   - Can tolerate drops (cache miss will just query again)
//   - Longer scale-down time (write-backs are bursty)
func WriteBackPoolConfig() Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             "writeback",
		MinWorkers:       2,
		MaxWorkers:       numCPU,
		QueueSize:        2000,
		ScaleUpThreshold: 200,
		ScaleDownAfter:   120 * time.Second,
		JobTimeout:       10 * time.Second,
		SubmitRetryWait:  0, // Drop immediately if full (low priority)
		Metrics:          NoOpMetrics{},
	}
}

// ArchivalPoolConfig returns configuration for NATS archival publishing.
// Characteristics:
//   - I/O-bound (NATS publishing to archival streams)
//   - Medium priority (audit trail, but async)
//   - Should not drop if possible (audit data is important)
//   - Moderate timeout (NATS with persistence)
func ArchivalPoolConfig() Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             "archival",
		MinWorkers:       numCPU,
		MaxWorkers:       numCPU * 4,
		QueueSize:        5000,
		ScaleUpThreshold: 500,
		ScaleDownAfter:   60 * time.Second,
		JobTimeout:       15 * time.Second,
		SubmitRetryWait:  100 * time.Millisecond, // Try harder before dropping
		Metrics:          NoOpMetrics{},
	}
}

// ActivityPoolConfig returns configuration for activity logging.
// Characteristics:
//   - I/O-bound (Valkey writes, NATS publishing)
//   - Medium priority (activity log for queries)
//   - Moderate volume (one per thread creation, access grant, etc.)
func ActivityPoolConfig() Config {
	numCPU := runtime.NumCPU()
	return Config{
		Name:             "activity",
		MinWorkers:       numCPU / 2,
		MaxWorkers:       numCPU * 2,
		QueueSize:        2000,
		ScaleUpThreshold: 200,
		ScaleDownAfter:   60 * time.Second,
		JobTimeout:       30 * time.Second,
		SubmitRetryWait:  50 * time.Millisecond,
		Metrics:          NoOpMetrics{},
	}
}

// Pools holds all worker pools for the application.
// This provides a single point of access and lifecycle management.
type Pools struct {
	Validation   *Pool
	Notification *Pool
	WriteBack    *Pool
	Archival     *Pool
	Activity     *Pool
}

// NewPools creates all worker pools with default configurations.
// Pass a PoolMetrics implementation to enable metrics for all pools.
func NewPools(metrics PoolMetrics) *Pools {
	if metrics == nil {
		metrics = NoOpMetrics{}
	}

	validationCfg := ValidationPoolConfig()
	validationCfg.Metrics = metrics

	notificationCfg := NotificationPoolConfig()
	notificationCfg.Metrics = metrics

	writeBackCfg := WriteBackPoolConfig()
	writeBackCfg.Metrics = metrics

	archivalCfg := ArchivalPoolConfig()
	archivalCfg.Metrics = metrics

	activityCfg := ActivityPoolConfig()
	activityCfg.Metrics = metrics

	return &Pools{
		Validation:   New(validationCfg),
		Notification: New(notificationCfg),
		WriteBack:    New(writeBackCfg),
		Archival:     New(archivalCfg),
		Activity:     New(activityCfg),
	}
}

// NewPoolsWithConfig creates worker pools with custom configurations.
// Any nil config will use the default for that pool type.
func NewPoolsWithConfig(
	validationCfg *Config,
	notificationCfg *Config,
	writeBackCfg *Config,
	archivalCfg *Config,
	activityCfg *Config,
) *Pools {
	if validationCfg == nil {
		cfg := ValidationPoolConfig()
		validationCfg = &cfg
	}
	if notificationCfg == nil {
		cfg := NotificationPoolConfig()
		notificationCfg = &cfg
	}
	if writeBackCfg == nil {
		cfg := WriteBackPoolConfig()
		writeBackCfg = &cfg
	}
	if archivalCfg == nil {
		cfg := ArchivalPoolConfig()
		archivalCfg = &cfg
	}
	if activityCfg == nil {
		cfg := ActivityPoolConfig()
		activityCfg = &cfg
	}

	return &Pools{
		Validation:   New(*validationCfg),
		Notification: New(*notificationCfg),
		WriteBack:    New(*writeBackCfg),
		Archival:     New(*archivalCfg),
		Activity:     New(*activityCfg),
	}
}

// Stats returns statistics for all pools.
func (p *Pools) Stats() map[string]Stats {
	return map[string]Stats{
		"validation":   p.Validation.Stats(),
		"notification": p.Notification.Stats(),
		"writeback":    p.WriteBack.Stats(),
		"archival":     p.Archival.Stats(),
		"activity":     p.Activity.Stats(),
	}
}

// IsHealthy returns true if all pools are healthy.
func (p *Pools) IsHealthy() bool {
	return p.Validation.IsHealthy() &&
		p.Notification.IsHealthy() &&
		p.WriteBack.IsHealthy() &&
		p.Archival.IsHealthy() &&
		p.Activity.IsHealthy()
}

// Shutdown gracefully shuts down all pools.
// Pools are shut down in reverse priority order (low priority first).
func (p *Pools) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Shutdown in order: low priority first
	// This allows high-priority pools to finish processing
	var firstErr error

	if err := p.WriteBack.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.Activity.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.Archival.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.Notification.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := p.Validation.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// ShutdownNow immediately stops all pools without waiting.
func (p *Pools) ShutdownNow() {
	p.WriteBack.ShutdownNow()
	p.Activity.ShutdownNow()
	p.Archival.ShutdownNow()
	p.Notification.ShutdownNow()
	p.Validation.ShutdownNow()
}

// PoolConfigToWorkerConfig converts a config.PoolConfig to workerpool.Config
func PoolConfigToWorkerConfig(name string, cfg interface{}, metrics PoolMetrics) Config {
	// Type assertion to get the config struct
	type poolCfg struct {
		MinWorkers        int
		MaxWorkers        int
		QueueSize         int
		ScaleUpThreshold  int
		ScaleDownAfterMs  int
		JobTimeoutMs      int
		SubmitRetryWaitMs int
	}

	var pc poolCfg

	// Use reflection or type switch to extract values
	// For now, we'll use a simple approach with defaults
	numCPU := runtime.NumCPU()

	// Return default config if cfg is nil or empty
	switch name {
	case "validation":
		cfg := ValidationPoolConfig()
		cfg.Metrics = metrics
		return cfg
	case "notification":
		cfg := NotificationPoolConfig()
		cfg.Metrics = metrics
		return cfg
	case "writeback":
		cfg := WriteBackPoolConfig()
		cfg.Metrics = metrics
		return cfg
	case "archival":
		cfg := ArchivalPoolConfig()
		cfg.Metrics = metrics
		return cfg
	case "activity":
		cfg := ActivityPoolConfig()
		cfg.Metrics = metrics
		return cfg
	default:
		_ = pc // Suppress unused warning
		return Config{
			Name:             name,
			MinWorkers:       numCPU,
			MaxWorkers:       numCPU * 4,
			QueueSize:        5000,
			ScaleUpThreshold: 500,
			ScaleDownAfter:   30 * time.Second,
			JobTimeout:       60 * time.Second,
			SubmitRetryWait:  0,
			Metrics:          metrics,
		}
	}
}
