package registry

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchUsageIsAtomicAcrossMetricsAndReplicas(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	// More contenders than either allowance, using both lock orders and separate runtimes.
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replica := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: pool}
			increments := []usageReservation{{InputBytes, 3, 30, -1}, {InputRequests, 1, -1, 10}}
			if i%2 == 0 {
				increments[0], increments[1] = increments[1], increments[0]
			}
			err := replica.reserveMany(ctx, increments, now)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrLimit) {
				t.Errorf("unexpected admission error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("admitted %d, want 10", accepted.Load())
	}
	var bytes, requests, outbox int64
	if err := pool.QueryRow(ctx, `SELECT
 (SELECT sum(count) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1),
 (SELECT sum(count) FROM threadify_registry_usage WHERE metric=$2 AND bucket_seconds>1),
 (SELECT count(*) FROM threadify_registry_outbox)`, InputBytes, InputRequests).Scan(&bytes, &requests, &outbox); err != nil {
		t.Fatal(err)
	}
	if bytes != 30 || requests != 10 || outbox != 20 {
		t.Fatalf("partial accounting: bytes=%d requests=%d outbox=%d", bytes, requests, outbox)
	}
	// First metric has room, second metric rejects: both counters and reports roll back.
	if err := r.reserveMany(ctx, []usageReservation{{InputBytes, 1, 100, -1}, {InputRequests, 1, -1, 0}}, now.Add(time.Second)); !errors.Is(err, ErrLimit) {
		t.Fatalf("rate rejection: %v", err)
	}
	var unchanged int64
	if err := pool.QueryRow(ctx, `SELECT sum(count) FROM threadify_registry_usage WHERE metric=$1`, InputBytes).Scan(&unchanged); err != nil || unchanged != 30 {
		t.Fatalf("rolled-back byte count=%d: %v", unchanged, err)
	}
}

func TestBatchUsageStorageFailureAndOverflow(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE threadify_registry_outbox ADD CONSTRAINT failed_second_metric CHECK(metric <> 'threadify.output.messages')`); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckBatch(ctx, Usage{OutputBytes, 7}, Usage{OutputMessages, 1}); err == nil {
		t.Fatal("storage failure admitted traffic")
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM threadify_registry_usage)+(SELECT count(*) FROM threadify_registry_outbox)`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("partial transaction: %d %v", rows, err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE threadify_registry_outbox DROP CONSTRAINT failed_second_metric`); err != nil {
		t.Fatal(err)
	}
	if err := r.Check(ctx, OutputBytes, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckBatch(ctx, Usage{OutputMessages, 1}, Usage{OutputBytes, 1}); !errors.Is(err, ErrLimit) {
		t.Fatalf("overflow admission: %v", err)
	}
	if err := r.CheckBatch(ctx, Usage{InputBytes, 1}, Usage{"invalid", 1}); err == nil {
		t.Fatal("invalid metric admitted")
	}
	if err := r.CheckBatch(ctx, Usage{InputBytes, 1}, Usage{InputRequests, -1}); err == nil {
		t.Fatal("negative increment admitted")
	}
}
