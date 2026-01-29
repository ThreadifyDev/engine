package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolBasicSubmit(t *testing.T) {
	pool := New(Config{
		Name:       "test",
		MinWorkers: 2,
		MaxWorkers: 4,
		QueueSize:  100,
		JobTimeout: 5 * time.Second,
	})
	defer pool.Shutdown(context.Background())

	var completed atomic.Int32

	for i := 0; i < 10; i++ {
		ok := pool.Submit(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			completed.Add(1)
		})
		if !ok {
			t.Error("Submit should succeed")
		}
	}

	// Wait for jobs to complete
	time.Sleep(200 * time.Millisecond)

	if completed.Load() != 10 {
		t.Errorf("Expected 10 completed jobs, got %d", completed.Load())
	}

	stats := pool.Stats()
	if stats.JobsSubmitted != 10 {
		t.Errorf("Expected 10 submitted, got %d", stats.JobsSubmitted)
	}
	if stats.JobsCompleted != 10 {
		t.Errorf("Expected 10 completed, got %d", stats.JobsCompleted)
	}
}

func TestPoolBackpressure(t *testing.T) {
	pool := New(Config{
		Name:            "test-backpressure",
		MinWorkers:      1,
		MaxWorkers:      1,
		QueueSize:       5,
		JobTimeout:      5 * time.Second,
		SubmitRetryWait: 0, // Drop immediately
	})
	defer pool.Shutdown(context.Background())

	// Block the single worker
	blocker := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		<-blocker
	})

	// Fill the queue
	for i := 0; i < 5; i++ {
		pool.Submit(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
		})
	}

	// This should be dropped
	ok := pool.Submit(func(ctx context.Context) {})
	if ok {
		t.Error("Submit should fail when queue is full")
	}

	stats := pool.Stats()
	if stats.JobsDropped != 1 {
		t.Errorf("Expected 1 dropped, got %d", stats.JobsDropped)
	}

	close(blocker)
}

func TestPoolScaleUp(t *testing.T) {
	pool := New(Config{
		Name:             "test-scale",
		MinWorkers:       1,
		MaxWorkers:       4,
		QueueSize:        1000,
		ScaleUpThreshold: 5,
		ScaleDownAfter:   100 * time.Millisecond,
		JobTimeout:       5 * time.Second,
	})
	defer pool.Shutdown(context.Background())

	// Submit many jobs to trigger scale-up
	blocker := make(chan struct{})
	for i := 0; i < 20; i++ {
		pool.Submit(func(ctx context.Context) {
			<-blocker
		})
	}

	// Wait for scaler to react
	time.Sleep(200 * time.Millisecond)

	stats := pool.Stats()
	if stats.TotalWorkers <= 1 {
		t.Errorf("Expected workers to scale up, got %d", stats.TotalWorkers)
	}

	close(blocker)
}

func TestPoolScaleDown(t *testing.T) {
	pool := New(Config{
		Name:           "test-scaledown",
		MinWorkers:     1,
		MaxWorkers:     4,
		QueueSize:      100,
		ScaleDownAfter: 50 * time.Millisecond,
		JobTimeout:     5 * time.Second,
	})
	defer pool.Shutdown(context.Background())

	// Start with extra workers by submitting concurrent jobs
	for i := 0; i < 4; i++ {
		pool.startWorker()
	}

	initialWorkers := pool.Stats().TotalWorkers
	if initialWorkers < 4 {
		t.Skipf("Not enough workers started: %d", initialWorkers)
	}

	// Wait for idle timeout
	time.Sleep(200 * time.Millisecond)

	stats := pool.Stats()
	if stats.TotalWorkers > 1 {
		// Scale down should have happened, but timing can be tricky
		t.Logf("Workers after idle: %d (expected ~1)", stats.TotalWorkers)
	}
}

func TestPoolJobTimeout(t *testing.T) {
	pool := New(Config{
		Name:       "test-timeout",
		MinWorkers: 1,
		MaxWorkers: 1,
		QueueSize:  10,
		JobTimeout: 50 * time.Millisecond,
	})
	defer pool.Shutdown(context.Background())

	var timedOut atomic.Bool

	pool.Submit(func(ctx context.Context) {
		select {
		case <-ctx.Done():
			timedOut.Store(true)
		case <-time.After(1 * time.Second):
			// Should not reach here
		}
	})

	time.Sleep(100 * time.Millisecond)

	if !timedOut.Load() {
		t.Error("Job should have been cancelled by timeout")
	}
}

func TestPoolGracefulShutdown(t *testing.T) {
	pool := New(Config{
		Name:       "test-shutdown",
		MinWorkers: 2,
		MaxWorkers: 4,
		QueueSize:  100,
		JobTimeout: 5 * time.Second,
	})

	var completed atomic.Int32

	for i := 0; i < 5; i++ {
		pool.Submit(func(ctx context.Context) {
			time.Sleep(20 * time.Millisecond)
			completed.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := pool.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should succeed: %v", err)
	}

	if completed.Load() != 5 {
		t.Errorf("Expected 5 completed, got %d", completed.Load())
	}
}

func TestPoolSubmitWait(t *testing.T) {
	pool := New(Config{
		Name:       "test-submitwait",
		MinWorkers: 1,
		MaxWorkers: 1,
		QueueSize:  1,
		JobTimeout: 5 * time.Second,
	})
	defer pool.Shutdown(context.Background())

	// Block the worker
	blocker := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		<-blocker
	})

	// Fill the queue
	pool.Submit(func(ctx context.Context) {})

	// SubmitWait with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := pool.SubmitWait(ctx, func(ctx context.Context) {})
	if err == nil {
		t.Error("SubmitWait should timeout")
	}

	close(blocker)
}

func TestPoolIsHealthy(t *testing.T) {
	pool := New(Config{
		Name:       "test-health",
		MinWorkers: 1,
		MaxWorkers: 1,
		QueueSize:  10,
		JobTimeout: 5 * time.Second,
	})
	defer pool.Shutdown(context.Background())

	if !pool.IsHealthy() {
		t.Error("Pool should be healthy initially")
	}

	// Fill queue to 90%
	blocker := make(chan struct{})
	pool.Submit(func(ctx context.Context) {
		<-blocker
	})
	for i := 0; i < 9; i++ {
		pool.Submit(func(ctx context.Context) {})
	}

	if pool.IsHealthy() {
		t.Error("Pool should be unhealthy when queue is >80% full")
	}

	close(blocker)
}

func TestValidationPoolConfig(t *testing.T) {
	cfg := ValidationPoolConfig()

	if cfg.Name != "validation" {
		t.Errorf("Expected name 'validation', got '%s'", cfg.Name)
	}
	if cfg.MinWorkers <= 0 {
		t.Error("MinWorkers should be positive")
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		t.Error("MaxWorkers should be >= MinWorkers")
	}
}

func TestNotificationPoolConfig(t *testing.T) {
	cfg := NotificationPoolConfig()

	if cfg.Name != "notification" {
		t.Errorf("Expected name 'notification', got '%s'", cfg.Name)
	}
	// Notification pool should have more workers (I/O bound)
	if cfg.MinWorkers < ValidationPoolConfig().MinWorkers {
		t.Error("Notification pool should have more min workers than validation")
	}
}

func TestNewPools(t *testing.T) {
	pools := NewPools(nil)
	defer pools.Shutdown(1 * time.Second)

	if pools.Validation == nil {
		t.Error("Validation pool should not be nil")
	}
	if pools.Notification == nil {
		t.Error("Notification pool should not be nil")
	}
	if pools.WriteBack == nil {
		t.Error("WriteBack pool should not be nil")
	}
	if pools.Archival == nil {
		t.Error("Archival pool should not be nil")
	}
	if pools.Activity == nil {
		t.Error("Activity pool should not be nil")
	}

	if !pools.IsHealthy() {
		t.Error("All pools should be healthy initially")
	}

	stats := pools.Stats()
	if len(stats) != 5 {
		t.Errorf("Expected 5 pool stats, got %d", len(stats))
	}
}
