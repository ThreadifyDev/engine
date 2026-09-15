package registry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"threadify-go/shared/testutil/natsfixture"
)

func TestJetStreamLocalAdmissionAvoidsRetryAmplification(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputBandwidthBytes = 128
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	enableTestMeter(t, r)
	meter := r.meter.Load()
	kv := &countedUsageKV{KeyValue: meter.kv}
	meter.kv = kv
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.Check(ctx, InputBytes, 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if kv.reads.Load() != 0 || kv.writes.Load() != 128 || kv.conflicts.Load() != 0 {
		t.Fatalf("local broker amplification: reads=%d writes=%d conflicts=%d", kv.reads.Load(), kv.writes.Load(), kv.conflicts.Load())
	}
	if err := r.Check(ctx, InputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("cap: %v", err)
	}
	// A deadline while queued must not wait for the writer or consume allowance.
	meter.admission <- struct{}{}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	err := r.Check(bounded, OutputBytes, 1)
	cancel()
	<-meter.admission
	if !errors.Is(err, context.DeadlineExceeded) || kv.writes.Load() != 128 {
		t.Fatalf("queued deadline: %v, writes=%d", err, kv.writes.Load())
	}
}

type pausedUsageKV struct {
	jetstream.KeyValue
	read, resume chan struct{}
}

func (kv *pausedUsageKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	entry, err := kv.KeyValue.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	close(kv.read)
	select {
	case <-kv.resume:
		return entry, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type fixedUsageJS struct {
	jetstream.JetStream
	kv jetstream.KeyValue
}

func (js fixedUsageJS) KeyValue(context.Context, string) (jetstream.KeyValue, error) {
	return js.kv, nil
}

// A newer checkpoint must not overtake startup's already-read KV snapshot.
func TestJetStreamStartupCoordinatesWithProjection(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	js := natsfixture.New(t)
	if err := r.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := r.Check(ctx, InputBytes, 1); err != nil {
		t.Fatal(err)
	}
	if err := r.ProjectUsage(ctx); err != nil {
		t.Fatal(err)
	}
	paused := &pausedUsageKV{KeyValue: r.meter.Load().kv, read: make(chan struct{}), resume: make(chan struct{})}
	restarted := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: pool, snapshot: s, verified: time.Now()}
	ready := make(chan error, 1)
	go func() { ready <- restarted.EnableJetStream(ctx, fixedUsageJS{JetStream: js, kv: paused}) }()
	select {
	case <-paused.read:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := r.Check(ctx, InputBytes, 1); err != nil {
		t.Fatal(err)
	}
	projected := make(chan error, 1)
	go func() { projected <- r.ProjectUsage(ctx) }()
	select {
	case err := <-projected:
		close(paused.resume)
		t.Fatalf("projection overtook startup snapshot: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(paused.resume)
	for _, done := range []chan error{ready, projected} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	// Startup's cached snapshot is now stale; CAS must refresh it before admission.
	restarted.meter.Load().kv = r.meter.Load().kv
	if err := restarted.Check(ctx, InputBytes, 1); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ProjectUsage(ctx); err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := pool.QueryRow(ctx, `SELECT sum(count) FROM threadify_registry_outbox`).Scan(&total); err != nil || total != 3 {
		t.Fatalf("startup lost usage: %d %v", total, err)
	}
}
