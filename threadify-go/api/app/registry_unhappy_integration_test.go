package app

import (
	"bytes"
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
	"sync/atomic"
	"testing"
	"threadify-go/shared/testutil/natsfixture"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"threadify-go/api/internal/handlers"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/registry"
	"threadify-go/shared/testutil/registryfixture"
)

func unhappyRegistryPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("THREADIFY_REGISTRY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires an explicitly supplied disposable PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	schema := "api_negative_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	_, err = pool.Exec(ctx, `CREATE TABLE companies(id uuid PRIMARY KEY,name text);CREATE TABLE application_mutations(id bigserial PRIMARY KEY,payload text NOT NULL)`)
	require.NoError(t, err)
	return pool
}

// The mutable Registry double exercises real heartbeat transitions without
// exposing setters or test bypasses in the production licensing runtime.
func unhappyRegistryRuntime(t *testing.T, pool *pgxpool.Pool, fast bool) (*registry.Runtime, *atomic.Value) {
	t.Helper()
	mode := &atomic.Value{}
	mode.Store("healthy")
	accountID := uuid.NewString()
	snapshot := registry.Snapshot{AccountID: accountID, WorkspaceID: accountID, Email: "owner@example.com", HeartbeatIntervalSeconds: 60, GracePeriodSeconds: 300, Entitlements: registry.Entitlements{Revision: "negative-test-v1", InputBandwidthBytes: -1, OutputBandwidthBytes: -1, InputRequestsPerSecond: -1, EntityProfileLimit: -1}}
	if fast {
		snapshot.HeartbeatIntervalSeconds = 1
		snapshot.GracePeriodSeconds = 2
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+registryfixture.License {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "read", 400)
				return
			}
			mac := hmac.New(sha256.New, []byte(registryfixture.License))
			_, _ = mac.Write(body)
			if !hmac.Equal([]byte(r.Header.Get("X-Threadify-Signature")), []byte("sha256="+hex.EncodeToString(mac.Sum(nil)))) {
				http.Error(w, "signature", 403)
				return
			}
		}
		current := mode.Load().(string)
		if current == "offline" {
			http.Error(w, "temporarily unavailable", 503)
			return
		}
		if r.URL.Path == "/api/threadify/usage-reports" {
			_, _ = io.WriteString(w, `{"status":"ok","accepted":0}`)
			return
		}
		result := snapshot
		result.Suspended = current == "suspended"
		if current == "capped" {
			result.Entitlements.Revision = "negative-test-v2"
			result.Entitlements.InputBandwidthBytes = 0
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(server.Close)
	runtime, err := registry.Start(context.Background(), registry.Config{URL: server.URL, LicenseKey: registryfixture.License}, pool, natsfixture.New(t))
	require.NoError(t, err)
	registry.SetDefault(runtime)
	t.Cleanup(func() { registry.SetDefault(nil); runtime.Close() })
	return runtime, mode
}

func assertNoApplicationMutations(t *testing.T, pool *pgxpool.Pool, want int64) {
	t.Helper()
	var count int64
	require.NoError(t, pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM application_mutations").Scan(&count))
	require.Equal(t, want, count)
}

func mutationHandler(t *testing.T, pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		payload, err := io.ReadAll(r.Body)
		if errors.Is(err, registry.ErrLimit) {
			http.Error(w, "input allowance exceeded", http.StatusTooManyRequests)
			return
		}
		if err != nil {
			t.Errorf("mutation read: %v", err)
			http.Error(w, "read failed", 500)
			return
		}
		_, err = pool.Exec(r.Context(), "INSERT INTO application_mutations(payload) VALUES($1)", string(payload))
		if err != nil {
			t.Errorf("mutation insert: %v", err)
			http.Error(w, "write failed", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"ok":true}}`)
	}
}

func TestLicensedCompanyMiddlewareRejectsForeignOrMissingIdentityWithoutMutation(t *testing.T) {
	pool := unhappyRegistryPool(t)
	runtime, _ := unhappyRegistryRuntime(t, pool, false)
	for _, tc := range []struct {
		name    string
		company any
		set     bool
	}{
		{name: "foreign company", company: uuid.NewString(), set: true},
		{name: "empty company", company: "", set: true},
		{name: "malformed company", company: 123, set: true},
		{name: "missing company"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.set {
					c.Set(sharedauth.CtxCompanyID, tc.company)
				}
				c.Next()
			})
			router.Use(licensedCompanyMiddleware())
			router.POST("/api/mutate", gin.WrapH(mutationHandler(t, pool)))
			recorder := httptest.NewRecorder()
			wrapRegistryManagement(router, runtime).ServeHTTP(recorder, httptest.NewRequest("POST", "/api/mutate", strings.NewReader("mutation")))
			require.Equal(t, http.StatusForbidden, recorder.Code)
			assertNoApplicationMutations(t, pool, 0)
		})
	}
	// Correctly bound identity still reaches the same handler after all denials.
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(sharedauth.CtxCompanyID, runtime.CompanyID()); c.Next() })
	router.Use(licensedCompanyMiddleware())
	router.POST("/api/mutate", gin.WrapH(mutationHandler(t, pool)))
	recorder := httptest.NewRecorder()
	wrapRegistryManagement(router, runtime).ServeHTTP(recorder, httptest.NewRequest("POST", "/api/mutate", strings.NewReader("mutation")))
	require.Equal(t, http.StatusOK, recorder.Code)
	assertNoApplicationMutations(t, pool, 1)
}

func TestRegistrySuspensionBlocksAPIMutationsAndRecoveryRestoresAccess(t *testing.T) {
	for _, modeName := range []string{"suspended"} {
		t.Run(modeName, func(t *testing.T) {
			pool := unhappyRegistryPool(t)
			runtime, mode := unhappyRegistryRuntime(t, pool, true)
			router := gin.New()
			router.POST("/api/mutate", gin.WrapH(mutationHandler(t, pool)))
			router.GET("/health", func(c *gin.Context) { c.Status(200) })
			api := wrapRegistryManagement(router, runtime)
			send := func(path string, method string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				api.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader("mutation")))
				return recorder
			}
			require.Equal(t, 200, send("/api/mutate", "POST").Code)
			assertNoApplicationMutations(t, pool, 1)
			mode.Store(modeName)
			require.Eventually(t, func() bool { _, err := runtime.Snapshot(); return err != nil }, 4*time.Second, 10*time.Millisecond, "runtime must observe explicit suspension")
			rejected := send("/api/mutate", "POST")
			require.Equal(t, http.StatusServiceUnavailable, rejected.Code)
			require.NotContains(t, rejected.Body.String(), registryfixture.License)
			assertNoApplicationMutations(t, pool, 1)
			require.Equal(t, 200, send("/health", "GET").Code, "operational diagnostics remain available")
			mode.Store("healthy")
			require.Eventually(t, func() bool { _, err := runtime.Snapshot(); return err == nil }, 3*time.Second, 10*time.Millisecond, "successful heartbeat restores verification")
			require.Equal(t, 200, send("/api/mutate", "POST").Code)
			assertNoApplicationMutations(t, pool, 2)
		})
	}
}

// Losing Registry connectivity does not revoke the last verified policy. Keep
// exercising actual mutations beyond the old grace duration, then prove that
// a recovered heartbeat still applies a newly restricted allowance.
func TestRegistryOutageKeepsAPIMutationsWorkingWithLastVerifiedLimits(t *testing.T) {
	pool := unhappyRegistryPool(t)
	runtime, mode := unhappyRegistryRuntime(t, pool, true)
	router := gin.New()
	router.POST("/api/mutate", gin.WrapH(mutationHandler(t, pool)))
	api := wrapRegistryManagement(router, runtime)
	send := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		api.ServeHTTP(recorder, httptest.NewRequest("POST", "/api/mutate", strings.NewReader("mutation")))
		return recorder
	}
	before, err := runtime.Snapshot()
	require.NoError(t, err)
	mode.Store("offline")
	deadline := time.Now().Add(time.Duration(before.GracePeriodSeconds)*time.Second + time.Second)
	var mutations int64
	for {
		require.Equal(t, http.StatusOK, send().Code, "transient heartbeat failure must not stop mutations")
		mutations++
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	after, err := runtime.Snapshot()
	require.NoError(t, err)
	require.Equal(t, before.Entitlements, after.Entitlements, "offline runtime retains verified allowances")
	assertNoApplicationMutations(t, pool, mutations)

	mode.Store("capped")
	require.Eventually(t, func() bool {
		snapshot, err := runtime.Snapshot()
		return err == nil && snapshot.Entitlements.Revision == "negative-test-v2" && snapshot.Entitlements.InputBandwidthBytes == 0
	}, 3*time.Second, 10*time.Millisecond, "recovery must refresh policy")
	require.Equal(t, http.StatusTooManyRequests, send().Code)
	assertNoApplicationMutations(t, pool, mutations)
	mode.Store("healthy")
	require.Eventually(t, func() bool {
		snapshot, err := runtime.Snapshot()
		return err == nil && snapshot.Entitlements.InputBandwidthBytes == -1
	}, 3*time.Second, 10*time.Millisecond)
	require.Equal(t, http.StatusOK, send().Code)
	assertNoApplicationMutations(t, pool, mutations+1)
}

// Exercise tampering between the real API proxy and the Engine wrapper. The
// application table proves that rejected proof/body/identity combinations never
// reach the mutation, while durable counters prove rejected attempts are metered.
func TestRegistryProxyRejectsTamperingAndReplayWithoutMutation(t *testing.T) {
	for _, kind := range []string{"signature", "body", "path", "query", "authorization", "content-type", "replay"} {
		t.Run(kind, func(t *testing.T) {
			pool := unhappyRegistryPool(t)
			runtime, _ := unhappyRegistryRuntime(t, pool, false)
			engineHandler := runtime.WrapEngine(mutationHandler(t, pool))
			var firstProof string
			engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "signature":
					r.Header.Set(registry.AccountingDelegationHeader, "invalid-signature")
				case "body":
					r.Body.Close()
					r.Body = io.NopCloser(strings.NewReader("changed payload"))
				case "path":
					r.URL.Path = "/v1/contracts"
				case "query":
					r.URL.RawQuery = "scope=other"
				case "authorization":
					r.Header.Set("Authorization", "Bearer another-user")
				case "content-type":
					r.Header.Set("Content-Type", "text/plain")
				case "replay":
					if firstProof == "" {
						firstProof = r.Header.Get(registry.AccountingDelegationHeader)
					} else {
						r.Header.Set(registry.AccountingDelegationHeader, firstProof)
					}
				}
				engineHandler.ServeHTTP(w, r)
			}))
			defer engine.Close()
			router := gin.New()
			router.POST("/api/graphql", handlers.NewGraphQLProxyHandler(engine.URL+"/graphql").ProxyGraphQL)
			api := wrapRegistryManagement(router, runtime)
			send := func() *httptest.ResponseRecorder {
				payload := bytes.NewBufferString(`{"query":"mutation {change}"}`)
				req := httptest.NewRequest("POST", "/api/graphql", payload)
				req.Header.Set("Authorization", "Bearer original-user")
				recorder := httptest.NewRecorder()
				api.ServeHTTP(recorder, req)
				return recorder
			}
			attempts := int64(1)
			mutations := int64(0)
			if kind == "replay" {
				require.Equal(t, 200, send().Code)
				attempts++
				mutations = 1
			}
			rejected := send()
			require.Equal(t, http.StatusUnauthorized, rejected.Code)
			assertNoApplicationMutations(t, pool, mutations)
			require.NotContains(t, rejected.Body.String(), registryfixture.License)
			require.NotContains(t, rejected.Body.String(), "original-user")
			var requests int64
			if err := runtime.ProjectUsage(context.Background()); err != nil {
				t.Fatal(err)
			}
			require.NoError(t, pool.QueryRow(context.Background(), "SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1", registry.InputRequests).Scan(&requests))
			require.GreaterOrEqual(t, requests, attempts, "invalid delegation cannot waive API request accounting")
			var output int64
			if err := runtime.ProjectUsage(context.Background()); err != nil {
				t.Fatal(err)
			}
			require.NoError(t, pool.QueryRow(context.Background(), "SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1", registry.OutputBytes).Scan(&output))
			require.Positive(t, output, "rejection output remains accounted")
		})
	}
}
