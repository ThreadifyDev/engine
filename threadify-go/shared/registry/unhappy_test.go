package registry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Bad upstream responses cannot replace the last verified policy or verification timestamp.
func TestInvalidRefreshPreservesVerification(t *testing.T) {
	for _, name := range []string{"malformed", "trailing JSON", "oversized", "missing account", "missing workspace", "missing revision", "negative limit", "zero interval", "short grace", "overflow grace", "changed account", "changed workspace", "unavailable", "timeout"} {
		t.Run(name, func(t *testing.T) {
			s := testSnapshot()
			status := http.StatusOK
			switch name {
			case "missing account":
				s.AccountID = ""
			case "missing workspace":
				s.WorkspaceID = ""
			case "missing revision":
				s.Entitlements.Revision = ""
			case "negative limit":
				s.Entitlements.InputBandwidthBytes = -2
			case "zero interval":
				s.HeartbeatIntervalSeconds = 0
			case "short grace":
				s.GracePeriodSeconds = 1
			case "overflow grace":
				s.GracePeriodSeconds = math.MaxInt64/int(time.Second) + 1
			case "changed account":
				s.AccountID = "other-account"
			case "changed workspace":
				s.WorkspaceID = "other-workspace"
			case "unavailable":
				status = http.StatusServiceUnavailable
			}
			body, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "malformed":
				body = []byte(`{"account_id":`)
			case "trailing JSON":
				body = append(body, []byte(` {"extra":true}`)...)
			case "oversized":
				body = append(body, []byte(strings.Repeat(" ", 1<<20))...)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if name == "timeout" {
					select {
					case <-req.Context().Done():
					case <-time.After(time.Second):
					}
					return
				}
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			good := testSnapshot()
			verified := time.Now().Add(-time.Minute)
			r := &Runtime{cfg: Config{URL: server.URL, LicenseKey: "test"}, client: server.Client(), snapshot: good, verified: verified}
			r.client.Timeout = 100 * time.Millisecond
			if err = r.refresh(context.Background(), true); err == nil {
				t.Fatal("invalid heartbeat renewed verification")
			}
			if r.snapshot != good || r.verified != verified {
				t.Fatal("invalid response overwrote last verified snapshot")
			}
			if _, err = r.Snapshot(); err != nil {
				t.Fatalf("last verified policy lost: %v", err)
			}
			r.verified = time.Now().Add(-365 * 24 * time.Hour)
			if got, snapshotErr := r.Snapshot(); snapshotErr != nil || got != good {
				t.Fatalf("failed heartbeat discarded old verified policy: %+v %v", got, snapshotErr)
			}
		})
	}
}

// Startup requires a verified handshake and rejects explicit license denial.
func TestStartupFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name, path string
		status     int
	}{
		{"invalid license", "/api/threadify/handshake", 401},
		{"product denied", "/api/threadify/handshake", 403},
		{"handshake outage", "/api/threadify/handshake", 503},
		{"heartbeat revoked", "/api/threadify/heartbeat", 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := testPool(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path == test.path {
					w.WriteHeader(test.status)
					return
				}
				_ = json.NewEncoder(w).Encode(testSnapshot())
			}))
			defer server.Close()
			r, err := Start(context.Background(), Config{URL: server.URL, LicenseKey: "test"}, pool)
			if r != nil {
				r.Close()
				t.Fatal("failed verification exposed a runtime")
			}
			if err == nil {
				t.Fatal("startup verification failure ignored")
			}
			if test.path == "/api/threadify/handshake" {
				var companies int
				if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM companies`).Scan(&companies); err != nil || companies != 0 {
					t.Fatalf("invalid license provisioned company: count=%d error=%v", companies, err)
				}
			}
		})
	}
}

// A successful handshake is sufficient to operate when the initial heartbeat
// fails. An arbitrarily old snapshot must retain its actual limits, not grant
// unlimited use; a later heartbeat can replace those limits without a restart.
func TestInitialHeartbeatFailureRetainsVerifiedLimits(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	var recovered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/threadify/heartbeat" && !recovered.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s := testSnapshot()
		s.Entitlements.InputBandwidthBytes = 1
		if recovered.Load() {
			s.Entitlements.InputBandwidthBytes = 5
			s.Entitlements.Revision = "recovered"
		}
		_ = json.NewEncoder(w).Encode(s)
	}))
	defer server.Close()
	r, err := Start(ctx, Config{URL: server.URL, LicenseKey: "test"}, pool)
	if err != nil {
		t.Fatalf("heartbeat outage prevented verified startup: %v", err)
	}
	defer r.Close()
	r.mu.Lock()
	r.verified = time.Now().Add(-365 * 24 * time.Hour)
	r.mu.Unlock()
	if err := r.Check(ctx, InputBytes, 1); err != nil {
		t.Fatalf("old verification denied usage: %v", err)
	}
	if err := r.Check(ctx, InputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("outage bypassed last verified limit: %v", err)
	}
	recovered.Store(true)
	if err := r.refresh(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := r.Check(ctx, InputBytes, 4); err != nil {
		t.Fatalf("recovered allowance not applied: %v", err)
	}
	if err := r.Check(ctx, InputBytes, 1); !errors.Is(err, ErrLimit) {
		t.Fatalf("recovered allowance not enforced: %v", err)
	}
}

// TestStartupRejectsChangedDatabaseBinding preserves the original tenant and installation on restart.
func TestStartupRejectsChangedDatabaseBinding(t *testing.T) {
	for _, binding := range []string{"account", "company", "installation"} {
		t.Run(binding, func(t *testing.T) {
			pool := testPool(t)
			var changed atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				s := testSnapshot()
				if changed.Load() && binding == "account" {
					s.AccountID = "other-account"
				}
				_ = json.NewEncoder(w).Encode(s)
			}))
			defer server.Close()
			cfg := Config{URL: server.URL, LicenseKey: "test"}
			r, err := Start(context.Background(), cfg, pool)
			if err != nil {
				t.Fatal(err)
			}
			originalID := r.cfg.InstallationID
			r.Close()
			changed.Store(true)
			if binding == "company" {
				cfg.CompanyID = "other-company"
			}
			if binding == "installation" {
				cfg.InstallationID = newID()
			}
			r, err = Start(context.Background(), cfg, pool)
			if r != nil {
				r.Close()
				t.Fatal("changed binding exposed a runtime")
			}
			if err == nil {
				t.Fatal("changed binding accepted")
			}
			var companies int
			var account, company, installation string
			if err := pool.QueryRow(context.Background(), `SELECT account_id,company_id,installation_id,(SELECT COUNT(*) FROM companies) FROM threadify_registry_binding CROSS JOIN threadify_registry_installation`).Scan(&account, &company, &installation, &companies); err != nil {
				t.Fatal(err)
			}
			if account != testSnapshot().AccountID || company != account || installation != originalID || companies != 1 {
				t.Fatalf("failed restart changed database binding: %s %s %s companies=%d", account, company, installation, companies)
			}
		})
	}
}

// Explicit credential rejection still suspends access until verified recovery.
func TestCredentialRejectionImmediatelySuspends(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			r := &Runtime{cfg: Config{URL: server.URL, LicenseKey: "test"}, client: server.Client(), snapshot: testSnapshot(), verified: time.Now()}
			if err := r.refresh(context.Background(), true); err == nil {
				t.Fatal("rejected credential accepted")
			}
			if _, err := r.Snapshot(); !errors.Is(err, ErrUnverified) {
				t.Fatalf("revoked credential retained access: %v", err)
			}
			recovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { _ = json.NewEncoder(w).Encode(testSnapshot()) }))
			defer recovery.Close()
			r.cfg.URL = recovery.URL
			if err := r.refresh(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Snapshot(); err != nil {
				t.Fatalf("verified recovery stayed suspended: %v", err)
			}
		})
	}
}

// TestUsageInvalidAcknowledgementsRetainOutbox proves no report is lost on an ambiguous response.
func TestUsageInvalidAcknowledgementsRetainOutbox(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{`, `{"status":"error","accepted":0}`, `{"status":"ok"}`, `{"status":"ok","accepted":null}`, `{"status":"ok","accepted":-1}`, `{"status":"ok","accepted":2}`, `{"status":"ok","accepted":1} {}`} {
		t.Run(body, func(t *testing.T) {
			if _, err := pool.Exec(ctx, `DELETE FROM threadify_registry_outbox`); err != nil {
				t.Fatal(err)
			}
			if err := r.Check(ctx, InputBytes, 7); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { _, _ = io.WriteString(w, body) }))
			defer server.Close()
			r.cfg.URL, r.client = server.URL, server.Client()
			if err := r.FlushUsage(ctx); err == nil {
				t.Error("invalid acknowledgement accepted")
			}
			var pending int
			if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM threadify_registry_outbox`).Scan(&pending); err != nil || pending != 1 {
				t.Fatalf("report lost: pending=%d error=%v", pending, err)
			}
		})
	}
	// A duplicate retry legitimately acknowledges zero newly inserted records.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, `{"status":"ok","accepted":0}`)
	}))
	defer server.Close()
	r.cfg.URL, r.client = server.URL, server.Client()
	if err := r.FlushUsage(ctx); err != nil {
		t.Fatalf("idempotent acknowledgement rejected: %v", err)
	}
}

// TestUsageStorageFailureRollsBackAdmission checks failures after counter writes remain atomic.
func TestUsageStorageFailureRollsBackAdmission(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	s := testSnapshot()
	r := &Runtime{cfg: Config{CompanyID: "company", InstallationID: newID()}, accountID: s.AccountID, pool: pool, snapshot: s, verified: time.Now()}
	if err := r.initializeStore(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE threadify_registry_outbox ADD CONSTRAINT simulate_storage_failure CHECK(count=0)`); err != nil {
		t.Fatal(err)
	}
	if err := r.Check(ctx, InputBytes, 7); err == nil {
		t.Fatal("failed durable accounting admitted traffic")
	}
	var counters, pending int
	if err := pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM threadify_registry_usage),(SELECT COUNT(*) FROM threadify_registry_outbox)`).Scan(&counters, &pending); err != nil || counters != 0 || pending != 0 {
		t.Fatalf("partial accounting committed: counters=%d pending=%d error=%v", counters, pending, err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE threadify_registry_outbox DROP CONSTRAINT simulate_storage_failure`); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := r.Check(canceled, InputBytes, 7); err == nil {
		t.Fatal("canceled accounting admitted traffic")
	}
	if err := r.Check(ctx, InputBytes, 7); err != nil {
		t.Fatalf("storage recovery failed: %v", err)
	}
}
