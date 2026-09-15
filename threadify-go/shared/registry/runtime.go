// Package registry implements Threadify's product-specific Registry license
// contract. Entitlements are deliberately held only in process memory.
package registry

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultURL lets standard deployments configure only their license key.
	DefaultURL     = "https://registry.usefused.com"
	InputBytes     = "threadify.input.bytes"
	OutputBytes    = "threadify.output.bytes"
	InputRequests  = "threadify.input.requests"
	OutputMessages = "threadify.output.messages"
)

var ErrLimit = errors.New("Registry allowance exceeded")
var ErrUnverified = errors.New("Threadify license is suspended or has not been verified")

type Config struct {
	BrowserOrigin  string `yaml:"browser_origin" mapstructure:"browser_origin"`
	URL            string `yaml:"url" mapstructure:"url"`
	LicenseKey     string `yaml:"license_key" mapstructure:"license_key"`
	InstallationID string `yaml:"installation_id" mapstructure:"installation_id"`
	CompanyID      string `yaml:"company_id" mapstructure:"company_id"`
}
type Entitlements struct {
	Revision               string `json:"revision"`
	InputBandwidthBytes    int64  `json:"input_bandwidth_bytes"`
	OutputBandwidthBytes   int64  `json:"output_bandwidth_bytes"`
	InputRequestsPerSecond int64  `json:"input_requests_per_second"`
	EntityProfileLimit     int64  `json:"entity_profile_limit"`
}
type Snapshot struct {
	AccountID                string       `json:"account_id"`
	WorkspaceID              string       `json:"workspace_id"`
	Email                    string       `json:"email"`
	Entitlements             Entitlements `json:"entitlements"`
	HeartbeatIntervalSeconds int          `json:"heartbeat_interval_seconds"`
	GracePeriodSeconds       int          `json:"grace_period_seconds"`
	Suspended                bool         `json:"suspended"`
}
type Report struct {
	ReportID      string    `json:"report_id"`
	Metric        string    `json:"metric"`
	BucketStart   time.Time `json:"bucket_start"`
	BucketSeconds int       `json:"bucket_seconds"`
	Count         int64     `json:"count"`
}
type Runtime struct {
	cfg       Config
	accountID string
	pool      *pgxpool.Pool
	client    *http.Client
	mu        sync.RWMutex
	snapshot  Snapshot
	verified  time.Time
	cancel    context.CancelFunc
	done      chan struct{}
}

var current atomic.Pointer[Runtime]

func SetDefault(r *Runtime) { current.Store(r) }
func Default() *Runtime     { return current.Load() }
func (r *Runtime) OwnerEmail() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot.Email
}
func (r *Runtime) AccountID() string {
	if r == nil {
		return ""
	}
	return r.accountID
}
func (r *Runtime) CompanyID() string {
	if r == nil {
		return ""
	}
	return r.cfg.CompanyID
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// resolveConfig preserves explicit configuration, then environment overrides, before production defaults.
func resolveConfig(c Config) Config {
	// A configured test Registry takes precedence over environment and production defaults.
	if c.URL == "" {
		c.URL = os.Getenv("THREADIFY_REGISTRY_URL")
	}
	// Only an omitted endpoint falls back; malformed explicit endpoints still fail startup validation.
	if c.URL == "" {
		c.URL = DefaultURL
	}
	// Credential resolution is unchanged: a default endpoint never supplies a license.
	if c.LicenseKey == "" {
		c.LicenseKey = os.Getenv("THREADIFY_LICENSE_KEY")
	}
	// Optional identity overrides remain available for existing installations.
	if c.InstallationID == "" {
		c.InstallationID = os.Getenv("THREADIFY_INSTALLATION_ID")
	}
	// Company binding keeps its established explicit-configuration precedence.
	if c.CompanyID == "" {
		c.CompanyID = os.Getenv("THREADIFY_COMPANY_ID")
	}
	if c.BrowserOrigin == "" {
		c.BrowserOrigin = os.Getenv("THREADIFY_BROWSER_ORIGIN")
	}
	return c
}
func Start(ctx context.Context, cfg Config, pool *pgxpool.Pool) (*Runtime, error) {
	cfg = resolveConfig(cfg)
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || cfg.LicenseKey == "" {
		return nil, errors.New("Registry URL and license key are required")
	}

	if pool == nil {
		return nil, errors.New("Registry usage accounting requires PostgreSQL")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('threadify:registry:schema',0))`); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS threadify_registry_installation (singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), installation_id text NOT NULL)`); err != nil {
		return nil, err
	}
	proposedID := cfg.InstallationID
	if proposedID == "" {
		proposedID = newID()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO threadify_registry_installation(singleton,installation_id) VALUES(true,$1) ON CONFLICT DO NOTHING`, proposedID); err != nil {
		return nil, err
	}
	var installationID string
	if err := tx.QueryRow(ctx, `SELECT installation_id FROM threadify_registry_installation WHERE singleton=true`).Scan(&installationID); err != nil {
		return nil, err
	}
	if cfg.InstallationID != "" && cfg.InstallationID != installationID {
		return nil, errors.New("configured installation ID differs from database binding")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	cfg.InstallationID = installationID

	r := &Runtime{cfg: cfg, pool: pool, client: &http.Client{Timeout: 15 * time.Second}, done: make(chan struct{})}
	if err := r.refresh(ctx, false); err != nil {
		return nil, fmt.Errorf("Threadify Registry handshake: %w", err)
	}
	r.accountID = r.snapshot.AccountID
	if r.cfg.CompanyID == "" {
		r.cfg.CompanyID = r.snapshot.AccountID
	}
	if err := r.initializeStore(ctx); err != nil {
		return nil, err
	}
	// Attempt liveness immediately, but a failed heartbeat cannot invalidate the
	// successful handshake. Explicit suspension/revocation still denies access.
	_ = r.refresh(ctx, true)
	if _, err := r.Snapshot(); err != nil {
		return nil, fmt.Errorf("Threadify Registry initial heartbeat: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Recover committed usage from a previous process before the normal timer.
	// A delivery failure retains the outbox and does not invalidate a verified license.
	_ = r.FlushUsage(ctx)
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	go r.run(runCtx)
	return r, nil
}
func (r *Runtime) Close() {
	if r != nil && r.cancel != nil {
		r.cancel()
		<-r.done
	}
}
func (r *Runtime) Snapshot() (Snapshot, error) {
	if r == nil {
		return Snapshot{}, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	// Heartbeats refresh policy; their age is not a runtime kill switch. Keep
	// enforcing the last verified in-memory limits until Registry replaces them.
	if r.snapshot.Suspended || r.verified.IsZero() {
		return Snapshot{}, ErrUnverified
	}
	return r.snapshot, nil
}
func (r *Runtime) CheckCompany(company string) error {
	if r == nil {
		return nil
	}
	if _, err := r.Snapshot(); err != nil {
		return err
	}
	if company != r.cfg.CompanyID {
		return errors.New("company is not provisioned by this Threadify license")
	}
	return nil
}
func (r *Runtime) request(ctx context.Context, method, path string, body any, out any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.cfg.URL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.cfg.LicenseKey)
	req.Header.Set("X-Threadify-Installation-ID", r.cfg.InstallationID)
	if body != nil {
		mac := hmac.New(sha256.New, []byte(r.cfg.LicenseKey))
		mac.Write(data)
		req.Header.Set("X-Threadify-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if res.StatusCode == 401 || res.StatusCode == 403 {
			r.mu.Lock()
			r.snapshot.Suspended = true
			r.mu.Unlock()
		}
		return fmt.Errorf("Registry returned HTTP %d", res.StatusCode)
	}
	// Validate the entire bounded response; a valid prefix must not hide trailing
	// JSON or an oversized payload that would otherwise renew verification.
	const maxResponseBytes = 1 << 20
	data, err = io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxResponseBytes {
		return errors.New("Registry response is too large")
	}
	return json.Unmarshal(data, out)
}
func (r *Runtime) refresh(ctx context.Context, heartbeat bool) error {
	var s Snapshot
	method, path := http.MethodGet, "/api/threadify/handshake"
	var body any
	if heartbeat {
		method = http.MethodPost
		path = "/api/threadify/heartbeat"
		r.mu.RLock()
		rev := r.snapshot.Entitlements.Revision
		r.mu.RUnlock()
		body = map[string]any{"reported_at": time.Now().UTC(), "installation_id": r.cfg.InstallationID, "version": "threadify", "build_hash": "", "applied_entitlement_revision": rev}
	}
	if err := r.request(ctx, method, path, body, &s); err != nil {
		return err
	}
	if s.AccountID == "" || s.WorkspaceID == "" || s.Entitlements.Revision == "" || s.HeartbeatIntervalSeconds <= 0 || s.GracePeriodSeconds < s.HeartbeatIntervalSeconds {
		return errors.New("invalid Registry entitlement response")
	}
	// Reject durations that would overflow the heartbeat timer; grace is metadata.
	if int64(s.GracePeriodSeconds) > int64((1<<63-1)/time.Second) {
		return errors.New("invalid Registry verification duration")
	}
	for _, v := range []int64{s.Entitlements.InputBandwidthBytes, s.Entitlements.OutputBandwidthBytes, s.Entitlements.InputRequestsPerSecond, s.Entitlements.EntityProfileLimit} {
		if v < -1 {
			return errors.New("invalid Registry limit")
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snapshot.AccountID != "" && r.snapshot.AccountID != s.AccountID {
		return errors.New("Registry account binding changed")
	}
	// A heartbeat cannot silently move the installation to a different workspace.
	if r.snapshot.WorkspaceID != "" && r.snapshot.WorkspaceID != s.WorkspaceID {
		return errors.New("Registry workspace binding changed")
	}
	r.snapshot = s
	r.verified = time.Now()
	if s.Suspended {
		return ErrUnverified
	}
	return nil
}
func (r *Runtime) run(ctx context.Context) {
	defer close(r.done)
	for {
		r.mu.RLock()
		interval := r.snapshot.HeartbeatIntervalSeconds
		r.mu.RUnlock()
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		_ = r.refresh(ctx, true)
		_ = r.FlushUsage(ctx)
	}
}

// Usage is one durable traffic counter increment.
type Usage struct {
	Metric string
	Count  int64
}

// Check reserves usage before forwarding traffic.
func (r *Runtime) Check(ctx context.Context, metric string, n int64) error {
	return r.CheckBatch(ctx, Usage{Metric: metric, Count: n})
}

// CheckBatch admits all increments together, or none. Entitlements stay in memory;
// counters and reporting outbox entries commit in the same transaction.
func (r *Runtime) CheckBatch(ctx context.Context, usage ...Usage) error {
	if r == nil {
		return nil
	}
	s, err := r.Snapshot()
	if err != nil {
		return err
	}
	reservations := make([]usageReservation, 0, len(usage))
	for _, u := range usage {
		if u.Count < 0 {
			return errors.New("negative usage")
		}
		monthly, rate := int64(-1), int64(-1)
		switch u.Metric {
		case InputBytes:
			monthly = s.Entitlements.InputBandwidthBytes
		case OutputBytes:
			monthly = s.Entitlements.OutputBandwidthBytes
		case InputRequests:
			rate = s.Entitlements.InputRequestsPerSecond
		case OutputMessages:
		default:
			return errors.New("unknown Registry usage metric")
		}
		if u.Count != 0 {
			reservations = append(reservations, usageReservation{u.Metric, u.Count, monthly, rate})
		}
	}
	if len(reservations) == 0 {
		return nil
	}
	return r.reserveMany(ctx, reservations, time.Now().UTC())
}
