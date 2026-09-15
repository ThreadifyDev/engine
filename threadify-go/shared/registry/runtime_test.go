package registry

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func unlimited() Entitlements {
	return Entitlements{Revision: "1", InputBandwidthBytes: -1, OutputBandwidthBytes: -1, InputRequestsPerSecond: -1, EntityProfileLimit: -1}
}
func testSnapshot() Snapshot {
	return Snapshot{AccountID: "account-1", WorkspaceID: "workspace-1", Email: "owner@example.test", Entitlements: unlimited(), HeartbeatIntervalSeconds: 60, GracePeriodSeconds: 300}
}
func TestHeartbeatAuthorizationAndLastVerifiedPolicy(t *testing.T) {
	snapshot := testSnapshot()
	secret := "license-value"
	var heartbeats atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("missing license authorization")
		}
		if req.Method == http.MethodPost {
			data, _ := io.ReadAll(req.Body)
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write(data)
			want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
			if req.Header.Get("X-Threadify-Signature") != want {
				t.Error("invalid signature")
			}
			heartbeats.Add(1)
		}
		_ = json.NewEncoder(w).Encode(snapshot)
	}))
	defer server.Close()
	r := &Runtime{cfg: Config{URL: server.URL, LicenseKey: secret}, client: server.Client()}
	if err := r.refresh(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := r.refresh(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if heartbeats.Load() != 1 {
		t.Fatal("heartbeat not sent")
	}
	r.verified = time.Now().Add(-301 * time.Second)
	if got, err := r.Snapshot(); err != nil || got != snapshot {
		t.Fatalf("missed heartbeat discarded verified policy: %+v %v", got, err)
	}
	snapshot.Suspended = true
	if err := r.refresh(context.Background(), true); !errors.Is(err, ErrUnverified) {
		t.Fatalf("suspension accepted: %v", err)
	}
}
func TestStartRequiresLicense(t *testing.T) {
	for _, k := range []string{"THREADIFY_REGISTRY_URL", "THREADIFY_LICENSE_KEY", "THREADIFY_INSTALLATION_ID", "THREADIFY_COMPANY_ID"} {
		t.Setenv(k, "")
	}
	if _, err := Start(context.Background(), Config{}, nil); err == nil {
		t.Fatal("unlicensed production start succeeded")
	}
}
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("THREADIFY_REGISTRY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_REGISTRY_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "registry_test_" + strings.ReplaceAll(newID(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		SetDefault(nil)
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(ctx, `CREATE TABLE companies(id text PRIMARY KEY,name text);CREATE TABLE entity_profile(id text PRIMARY KEY,company_id text NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}
func TestDurableAllowancesRestartAndIdempotentReports(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	snapshot := testSnapshot()
	snapshot.Entitlements.InputBandwidthBytes = 100
	var mu sync.Mutex
	seen := map[string]bool{}
	var total int64
	failResponse := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/threadify/usage-reports" {
			var body struct {
				Reports []Report `json:"reports"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, r := range body.Reports {
				if !seen[r.ReportID] {
					total += r.Count
					seen[r.ReportID] = true
				}
			}
			if failResponse {
				failResponse = false
				w.WriteHeader(503)
				return
			}
			_, _ = io.WriteString(w, `{"status":"ok","accepted":0}`)
			return
		}
		_ = json.NewEncoder(w).Encode(snapshot)
	}))
	defer server.Close()
	cfg := Config{URL: server.URL, LicenseKey: "license-one"}
	r, err := Start(ctx, cfg, pool)
	if err != nil {
		t.Fatal(err)
	}
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := r.Check(ctx, InputBytes, 10)
			if err == nil {
				allowed.Add(1)
			} else if !errors.Is(err, ErrLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("concurrent quota overspend: allowed %d", allowed.Load())
	}
	id := r.cfg.InstallationID
	r.Close()
	cfg.LicenseKey = "rotated-license"
	r, err = Start(ctx, cfg, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.cfg.InstallationID != id {
		t.Fatal("license rotation changed installation identity")
	}
	if err = r.Check(ctx, InputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("restart reset quota: %v", err)
	}
	// The immediate startup report encountered the simulated lost response.
	// Its committed IDs must remain available for an idempotent retry.
	var pending int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM threadify_registry_outbox`).Scan(&pending); err != nil || pending != 10 {
		t.Fatalf("lost retry data: %d %v", pending, err)
	}
	if err = r.FlushUsage(ctx); err != nil {
		t.Fatal(err)
	}
	if total != 100 {
		t.Fatalf("retry double counted %d", total)
	}
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM threadify_registry_outbox`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("acknowledged outbox not drained: %d %v", pending, err)
	}
	var columns int
	if err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND (column_name LIKE '%limit%' OR column_name LIKE '%entitlement%' OR column_name LIKE '%license%')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("entitlements persisted: %d %v", columns, err)
	}
}
func TestProfileLimitSerializesConcurrentCreators(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.EntityProfileLimit = 1
	r := &Runtime{cfg: Config{CompanyID: "company"}, pool: pool, snapshot: s, verified: time.Now()}
	SetDefault(r)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback(ctx)
			err = GuardProfileCreation(ctx, tx, "company", 1)
			if errors.Is(err, ErrLimit) {
				return
			}
			if err != nil {
				t.Error(err)
				return
			}
			if _, err = tx.Exec(ctx, `INSERT INTO entity_profile(id,company_id) VALUES($1,'company')`, newID()); err != nil {
				t.Error(err)
				return
			}
			if err = tx.Commit(ctx); err != nil {
				t.Error(err)
				return
			}
			allowed.Add(1)
		}()
	}
	wg.Wait()
	if allowed.Load() != 1 {
		t.Fatalf("profile quota overspend: %d", allowed.Load())
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = GuardProfileCreation(ctx, tx, "company", 0); err != nil {
		t.Fatalf("existing profile update denied: %v", err)
	}
}

// Monthly byte budgets and per-second incoming request counts are independent controls.
func TestBidirectionalLimitsAndRateWindows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	for _, metric := range []string{InputBytes, OutputBytes} {
		if err := r.reserve(ctx, metric, 4, 10, -1, now); err != nil {
			t.Fatal(err)
		}
		// Multiple byte reservations in the same second consume only monthly bandwidth.
		if err := r.reserve(ctx, metric, 6, 10, -1, now); err != nil {
			t.Fatal(err)
		}
		if err := r.reserve(ctx, metric, 1, 10, -1, now.Add(time.Second)); !errors.Is(err, ErrLimit) {
			t.Fatalf("%s monthly limit: %v", metric, err)
		}
		if err := r.reserve(ctx, metric, 10, 10, -1, now.AddDate(0, 1, 0)); err != nil {
			t.Fatalf("%s calendar rollover: %v", metric, err)
		}
	}
	for _, metric := range []string{InputRequests} {
		if err := r.reserve(ctx, metric, 2, -1, 2, now); err != nil {
			t.Fatal(err)
		}
		if err := r.reserve(ctx, metric, 1, -1, 2, now); !errors.Is(err, ErrLimit) {
			t.Fatalf("%s per-second limit: %v", metric, err)
		}
		if err := r.reserve(ctx, metric, 2, -1, 2, now.Add(time.Second)); err != nil {
			t.Fatalf("%s next second: %v", metric, err)
		}
	}
	if err := r.reserve(ctx, InputBytes, 1, 0, -1, now); !errors.Is(err, ErrLimit) {
		t.Fatalf("zero bandwidth accepted: %v", err)
	}
	if err := r.reserve(ctx, InputRequests, 1, -1, 0, now); !errors.Is(err, ErrLimit) {
		t.Fatalf("zero incoming request rate accepted: %v", err)
	}
	var byteRateBuckets int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM threadify_registry_usage WHERE metric IN ($1,$2,$3) AND bucket_seconds=1`, InputBytes, OutputBytes, OutputMessages).Scan(&byteRateBuckets); err != nil || byteRateBuckets != 0 {
		t.Fatalf("byte rate buckets=%d error=%v", byteRateBuckets, err)
	}
}

// A one-request rate allows a large body and response within the monthly budget.
func TestRequestRateDoesNotLimitPayloadBytes(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputRequestsPerSecond = 1
	s.Entitlements.InputBandwidthBytes = 1 << 20
	s.Entitlements.OutputBandwidthBytes = 1 << 20
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, usage := range []struct {
		metric string
		count  int64
	}{{InputRequests, 1}, {InputBytes, 256 << 10}, {OutputMessages, 100}, {OutputBytes, 256 << 10}} {
		if err := r.Check(ctx, usage.metric, usage.count); err != nil {
			t.Fatalf("%s rejected: %v", usage.metric, err)
		}
	}
}

func TestHTTPDenialsBeforeInputAndOutput(t *testing.T) {
	for _, direction := range []string{"input", "output", "request"} {
		t.Run(direction, func(t *testing.T) {
			pool := testPool(t)
			s := testSnapshot()
			switch direction {
			case "input":
				s.Entitlements.InputBandwidthBytes = 1
			case "output":
				s.Entitlements.OutputBandwidthBytes = 1
			case "request":
				s.Entitlements.InputRequestsPerSecond = 0
			}
			r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
			if err := r.initializeStore(context.Background()); err != nil {
				t.Fatal(err)
			}
			acceptedInput := false
			h := r.Wrap(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if _, err := io.ReadAll(req.Body); err != nil {
					http.Error(w, "malformed payload", 400)
					return
				}
				acceptedInput = true
				w.Header().Set("Content-Length", "7")
				w.WriteHeader(200)
				_, _ = w.Write([]byte("payload"))
			}))
			req := httptest.NewRequest("POST", "/graphql", strings.NewReader("payload"))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != 429 {
				t.Fatalf("%s denial status %d body %q", direction, w.Code, w.Body.String())
			}
			if direction == "input" && acceptedInput {
				t.Fatal("over-quota input reached handler")
			}
			if direction == "output" && w.Body.Len() != 0 {
				t.Fatal("over-quota output was sent")
			}
		})
	}
}

// One incoming request may stream many responses without an outgoing rate cap.
func TestIncomingRateAllowsMultipleSSEMessages(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	s.Entitlements.InputRequestsPerSecond = 1
	s.Entitlements.OutputBandwidthBytes = 1024
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	const event = "data: ok\n\n"
	h := r.Wrap(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 20; i++ {
			if _, err := io.WriteString(w, event); err != nil {
				t.Errorf("response %d rejected: %v", i, err)
				return
			}
			w.(http.Flusher).Flush()
		}
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
	if w.Code != 200 || w.Body.String() != strings.Repeat(event, 20) {
		t.Fatalf("stream status=%d bytes=%d", w.Code, w.Body.Len())
	}
	for metric, want := range map[string]int64{InputRequests: 1, OutputMessages: 20, OutputBytes: int64(len(event) * 20)} {
		var count int64
		if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1`, metric).Scan(&count); err != nil || count != want {
			t.Fatalf("%s count=%d want=%d error=%v", metric, count, want, err)
		}
	}
	var outputRateBuckets int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds=1`, OutputMessages).Scan(&outputRateBuckets); err != nil || outputRateBuckets != 0 {
		t.Fatalf("outgoing rate buckets=%d error=%v", outputRateBuckets, err)
	}
}
