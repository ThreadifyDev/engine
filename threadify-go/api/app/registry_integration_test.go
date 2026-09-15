package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"threadify-go/api/internal/handlers"
	"threadify-go/shared/registry"
	"threadify-go/shared/testutil/registryfixture"
)

// TestRegistryProxyAccounting verifies the real API proxy, signed Engine hop,
// and durable counters together, including local errors on formerly exempt routes.
func TestRegistryProxyAccounting(t *testing.T) {
	dsn := os.Getenv("THREADIFY_REGISTRY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires an explicitly supplied disposable PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "proxy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, "CREATE TABLE companies(id uuid PRIMARY KEY,name text)"); err != nil {
		t.Fatal(err)
	}
	fixture := registryfixture.New(t, uuid.NewString())
	runtime, err := registry.Start(ctx, registry.Config{URL: fixture.URL, LicenseKey: registryfixture.License}, pool)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	registry.SetDefault(runtime)
	defer registry.SetDefault(nil)
	responseBody := []byte(`{"data":{"ok":true}}`)
	engine := httptest.NewServer(runtime.WrapEngine(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(responseBody)
	})))
	defer engine.Close()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	proxy := handlers.NewGraphQLProxyHandler(engine.URL + "/graphql")
	router.POST("/api/graphql", proxy.ProxyGraphQL)
	api := httptest.NewServer(wrapRegistryManagement(router, runtime))
	defer api.Close()
	payload := []byte(`{"query":"{ok}"}`)
	req, _ := http.NewRequest(http.MethodPost, api.URL+"/api/graphql", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer local-test-user")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || !bytes.Equal(body, responseBody) {
		t.Fatalf("proxy response %d %s", res.StatusCode, body)
	}
	for metric, want := range map[string]int64{registry.InputBytes: int64(len(payload)), registry.OutputBytes: int64(len(responseBody)), registry.InputRequests: 1, registry.OutputMessages: 1} {
		var got int64
		if err = pool.QueryRow(ctx, "SELECT COALESCE(sum(count),0) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1", metric).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s=%d want%d: forwarding counted traffic incorrectly", metric, got, want)
		}
	}
	// An unknown proxy-like path must still count at the API boundary.
	res, err = http.Get(api.URL + "/api/contracts/not-a-real-route")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("unknown path status%d", res.StatusCode)
	}
	var requests int64
	if err = pool.QueryRow(ctx, "SELECT COALESCE(sum(count),0) FROM threadify_registry_usage WHERE metric=$1 AND bucket_seconds>1", registry.InputRequests).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("unknown route bypassed request accounting: %d", requests)
	}
}
