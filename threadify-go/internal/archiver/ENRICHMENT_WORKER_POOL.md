# Enrichment Worker Pool - Archiver

## Overview
The archiver now includes a dedicated **Enrichment Worker Pool** for handling async data enrichment tasks, similar to the worker pools used in the main engine.

## Configuration

### Location
`config/config.yaml` under the `archiver` section:

```yaml
archiver:
  enabled: true
  metrics_port: 9091
  
  # Enrichment worker pool for async data enrichment tasks
  enrichment_workers:
    min_workers: 5               # CPU cores / 2
    max_workers: 20              # 2x CPU cores
    queue_size: 2000
    scale_up_threshold: 200
    scale_down_after_ms: 60000   # 60 seconds
    job_timeout_ms: 30000        # 30 seconds
    submit_retry_wait_ms: 10     # Brief retry before dropping
```

### Configuration Parameters

- **min_workers**: Minimum number of workers to maintain (default: CPU cores / 2)
- **max_workers**: Maximum number of workers (default: 2x CPU cores)
- **queue_size**: Buffered channel capacity for pending jobs
- **scale_up_threshold**: Queue depth that triggers spawning a new worker
- **scale_down_after_ms**: Idle duration before a worker exits (milliseconds)
- **job_timeout_ms**: Maximum duration for a single job (milliseconds)
- **submit_retry_wait_ms**: Wait time when queue is full before dropping job

## Architecture

### Worker Pool Pattern
The enrichment worker pool uses the same bounded worker pool implementation as the engine:
- **Dynamic Scaling**: Workers scale up/down based on queue pressure
- **Bounded Concurrency**: Prevents unbounded goroutine growth
- **Graceful Degradation**: Jobs are dropped if queue is full (configurable retry)
- **Timeout Protection**: Jobs have configurable timeouts

### Integration Points

The worker pool is initialized in `/cmd/archiver/main.go`:

```go
// Initialize enrichment worker pool
enrichmentPool := initEnrichmentWorkerPool(cfg, logger)
logger.Info("enrichment worker pool initialized",
    zap.Int("min_workers", cfg.Archiver.EnrichmentWorkers.MinWorkers),
    zap.Int("max_workers", cfg.Archiver.EnrichmentWorkers.MaxWorkers),
    zap.Int("queue_size", cfg.Archiver.EnrichmentWorkers.QueueSize),
)
```

Shutdown is handled gracefully:

```go
// Shutdown enrichment worker pool first (allow pending jobs to complete)
logger.Info("shutting down enrichment worker pool...")
if err := enrichmentPool.Shutdown(context.Background()); err != nil {
    logger.Warn("enrichment pool shutdown error", zap.Error(err))
}
```

## Usage Examples

### Submitting Enrichment Jobs

To submit a job to the enrichment worker pool, you can access it from your consumer or writer:

```go
// Example: Enrich thread metadata with external data
enrichmentPool.Submit(func(ctx context.Context) {
    // Your enrichment logic here
    threadID := "thread-123"
    
    // Fetch external data
    externalData, err := fetchExternalData(ctx, threadID)
    if err != nil {
        logger.Error("enrichment failed", zap.Error(err))
        return
    }
    
    // Update database with enriched data
    if err := db.UpdateThreadEnrichment(ctx, threadID, externalData); err != nil {
        logger.Error("failed to save enrichment", zap.Error(err))
    }
})
```

### Blocking Submit (Critical Jobs)

For critical enrichment tasks that must complete:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := enrichmentPool.SubmitWait(ctx, func(jobCtx context.Context) {
    // Critical enrichment logic
    // This will block until queued or context is cancelled
})
if err != nil {
    logger.Error("failed to submit critical enrichment", zap.Error(err))
}
```

### Non-Blocking Submit (Best Effort)

For low-priority enrichment tasks:

```go
submitted := enrichmentPool.TrySubmit(func(ctx context.Context) {
    // Best-effort enrichment logic
    // Returns immediately if queue is full
})
if !submitted {
    logger.Warn("enrichment job dropped (queue full)")
}
```

## Use Cases

### 1. Entity Profile Enrichment
Enrich thread data with entity profile information (company details, user metadata, etc.):

```go
enrichmentPool.Submit(func(ctx context.Context) {
    profile, err := entityProfileRepo.GetProfile(ctx, entityID)
    if err != nil {
        return
    }
    // Attach profile data to thread metadata
    updateThreadWithProfile(ctx, threadID, profile)
})
```

### 2. External API Calls
Fetch data from external services without blocking the main archival pipeline:

```go
enrichmentPool.Submit(func(ctx context.Context) {
    // Call external API (e.g., geocoding, sentiment analysis)
    enrichedData, err := externalAPI.Enrich(ctx, threadData)
    if err != nil {
        logger.Warn("external enrichment failed", zap.Error(err))
        return
    }
    // Store enriched data
    saveEnrichedData(ctx, threadID, enrichedData)
})
```

### 3. Derived Metrics Calculation
Calculate derived metrics asynchronously:

```go
enrichmentPool.Submit(func(ctx context.Context) {
    // Calculate thread duration, step counts, etc.
    metrics := calculateThreadMetrics(threadID)
    saveThreadMetrics(ctx, threadID, metrics)
})
```

### 4. Data Validation & Cleanup
Perform data quality checks and cleanup:

```go
enrichmentPool.Submit(func(ctx context.Context) {
    // Validate data integrity
    if err := validateThreadData(ctx, threadID); err != nil {
        logger.Error("validation failed", zap.Error(err))
        // Trigger cleanup or alert
    }
})
```

## Monitoring

### Pool Statistics

Access pool statistics for monitoring:

```go
stats := enrichmentPool.Stats()
logger.Info("enrichment pool stats",
    zap.Int32("active_workers", stats.ActiveWorkers),
    zap.Int32("total_workers", stats.TotalWorkers),
    zap.Int("queue_depth", stats.QueueDepth),
    zap.Int64("jobs_submitted", stats.JobsSubmitted),
    zap.Int64("jobs_completed", stats.JobsCompleted),
    zap.Int64("jobs_dropped", stats.JobsDropped),
)
```

### Health Check

Check if the pool is healthy (not overloaded):

```go
if !enrichmentPool.IsHealthy() {
    logger.Warn("enrichment pool is overloaded",
        zap.Int("queue_depth", enrichmentPool.QueueDepth()),
    )
}
```

## Best Practices

1. **Job Timeout**: Ensure enrichment jobs respect the context timeout
2. **Error Handling**: Always handle errors gracefully (don't panic)
3. **Idempotency**: Make enrichment jobs idempotent when possible
4. **Logging**: Log enrichment failures for debugging
5. **Metrics**: Track enrichment success/failure rates
6. **Backpressure**: Use `TrySubmit()` for non-critical enrichments to avoid blocking

## Performance Characteristics

- **I/O-Bound**: Designed for I/O-bound tasks (API calls, database queries)
- **Moderate Priority**: Lower priority than critical archival tasks
- **Async**: Does not block the main archival pipeline
- **Scalable**: Workers scale based on queue pressure

## Comparison with Engine Worker Pools

| Feature | Engine Pools | Archiver Enrichment Pool |
|---------|-------------|--------------------------|
| Purpose | Validation, Notification, Archival | Data Enrichment |
| Priority | High (critical path) | Medium (async) |
| Workers | CPU-bound (validation) / I/O-bound (notification) | I/O-bound |
| Drop Policy | Configurable retry | Brief retry (10ms) |
| Timeout | 15-60s | 30s |

## Future Enhancements

1. **Metrics Integration**: Add Prometheus metrics for enrichment jobs
2. **Dead Letter Queue**: Store failed enrichment jobs for retry
3. **Priority Queues**: Support different priority levels for enrichment tasks
4. **Batch Enrichment**: Group similar enrichment tasks for efficiency
5. **Rate Limiting**: Add per-source rate limiting for external API calls
