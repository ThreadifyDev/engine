package registry

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"threadify-go/shared/testutil/natsfixture"
)

// Two independent runtimes share durable state while SQL is unavailable.
func TestJetStreamAdmissionSurvivesDatabaseOutageAndProjectsOnce(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputBandwidthBytes = 100
	appPool, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: appPool, snapshot: s, verified: time.Now()}
	if err = r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	js := natsfixture.New(t)
	if err = r.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	other := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: appPool, snapshot: s, verified: time.Now()}
	if err = other.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	appPool.Close()
	for i := 0; i < 10; i++ {
		instance := r
		if i%2 == 0 {
			instance = other
		}
		if err = instance.Check(ctx, InputBytes, 10); err != nil {
			t.Fatal(err)
		}
	}
	if err = r.Check(ctx, InputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("outage bypassed quota: %v", err)
	}
	if err = r.ProjectUsage(ctx); err == nil {
		t.Fatal("closed database projected successfully")
	}
	recovered, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	r.pool = recovered
	other.pool = recovered
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			instance := r
			if i%2 == 0 {
				instance = other
			}
			if err := instance.ProjectUsage(ctx); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	var usage, reports int64
	if err = pool.QueryRow(ctx, `SELECT (SELECT sum(count) FROM threadify_registry_usage WHERE bucket_seconds>1),(SELECT sum(count) FROM threadify_registry_outbox)`).Scan(&usage, &reports); err != nil || usage != 100 || reports != 100 {
		t.Fatalf("projection duplicated/lost usage: %d %d %v", usage, reports, err)
	}
}

type uncertainUpdateKV struct {
	jetstream.KeyValue
	attempts int
}

// Simulate a durable write whose acknowledgement never reaches the caller.
func (kv *uncertainUpdateKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	kv.attempts++
	seq, err := kv.KeyValue.Update(ctx, key, value, revision)
	if err != nil {
		return seq, err
	}
	return 0, context.DeadlineExceeded
}

func TestJetStreamUncertainAcknowledgementDoesNotRetryDebit(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputBandwidthBytes = 10
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	enableTestMeter(t, r)
	meter := r.meter.Load()
	kv := &uncertainUpdateKV{KeyValue: meter.kv}
	meter.kv = kv
	if err := r.Check(ctx, InputBytes, 6); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("uncertain update should fail: %v", err)
	}
	if kv.attempts != 1 {
		t.Fatalf("uncertain update retried %d times", kv.attempts)
	}
	meter.kv = kv.KeyValue
	if err := r.Check(ctx, InputBytes, 5); !errors.Is(err, ErrLimit) {
		t.Fatalf("uncertain debit was forgotten: %v", err)
	}
	if err := r.Check(ctx, InputBytes, 4); err != nil {
		t.Fatalf("uncertain debit was duplicated: %v", err)
	}
	projectTestUsage(t, r)
	var reported int64
	if err := pool.QueryRow(ctx, `SELECT sum(count) FROM threadify_registry_outbox`).Scan(&reported); err != nil || reported != 10 {
		t.Fatalf("uncertain usage reporting: %d %v", reported, err)
	}
}

// Restart after an unprojected write reads KV, rather than an older SQL count.
func TestJetStreamRestartMissingStateAndBrokerFailure(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.OutputBandwidthBytes = 10
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	js := natsfixture.New(t)
	if err := r.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := r.Check(ctx, OutputBytes, 10); err != nil {
		t.Fatal(err)
	}
	restarted := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := restarted.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Check(ctx, OutputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("restart reset quota: %v", err)
	}
	meter := r.meter.Load()
	entry, err := meter.kv.Get(ctx, meter.key)
	if err != nil {
		t.Fatal(err)
	}
	if err = meter.kv.Delete(ctx, meter.key); err != nil {
		t.Fatal(err)
	}
	if err = r.Check(ctx, OutputBytes, 1); err == nil {
		t.Fatal("missing live state admitted")
	}
	missing := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: pool, snapshot: s, verified: time.Now()}
	if err = missing.EnableJetStream(ctx, js); err == nil {
		t.Fatal("startup replenished missing live state")
	}
	if _, err = meter.kv.Create(ctx, meter.key, entry.Value()); err != nil {
		t.Fatal(err)
	}
	if err = missing.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	if err = missing.Check(ctx, OutputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	js.Conn().Close()
	if err = r.Check(ctx, InputBytes, 1); err == nil {
		t.Fatal("broker outage admitted traffic")
	}
}

// Migrating old counters neither replenishes allowance nor resends old usage.
func TestJetStreamMigrationPreservesUsageAndRejectsRollback(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputBandwidthBytes = 100
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	seconds := int64(month.AddDate(0, 1, 0).Sub(month) / time.Second)
	if _, err := pool.Exec(ctx, `INSERT INTO threadify_registry_usage(account_id,metric,bucket_start,bucket_seconds,count) VALUES($1,$2,$3,$4,80)`, r.accountID, InputBytes, month, seconds); err != nil {
		t.Fatal(err)
	}
	js := natsfixture.New(t)
	if err := r.EnableJetStream(ctx, js); err != nil {
		t.Fatal(err)
	}
	original, err := r.meter.Load().kv.Get(ctx, r.meter.Load().key)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Check(ctx, InputBytes, 21); !errors.Is(err, ErrLimit) {
		t.Fatalf("migration forgot usage: %v", err)
	}
	if err = r.Check(ctx, InputBytes, 20); err != nil {
		t.Fatal(err)
	}
	projectTestUsage(t, r)
	var delta int64
	if err = pool.QueryRow(ctx, `SELECT sum(count) FROM threadify_registry_outbox`).Scan(&delta); err != nil || delta != 20 {
		t.Fatalf("migration re-reported usage: %d %v", delta, err)
	}
	var state usageState
	if err = json.Unmarshal(original.Value(), &state); err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal(state)
	if _, err = r.meter.Load().kv.Put(ctx, r.meter.Load().key, stale); err != nil {
		t.Fatal(err)
	}
	restarted := &Runtime{cfg: r.cfg, accountID: r.accountID, pool: pool, snapshot: s, verified: time.Now()}
	if err = restarted.EnableJetStream(ctx, js); err == nil {
		t.Fatal("stale broker restore rolled back counters")
	}
}
