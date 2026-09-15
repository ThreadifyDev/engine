package archiver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"threadify-go/shared/registry"
)

func TestRegistryProfileLimitPreservesThreadRefs(t *testing.T) {
	dsn := os.Getenv("THREADIFY_REGISTRY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_REGISTRY_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "profile_registry_" + uuid.New().String()[:8]
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE companies(id text PRIMARY KEY,name text);
 CREATE TABLE threads(id text PRIMARY KEY,company_id text,has_entity_refs boolean DEFAULT false);
 CREATE TABLE thread_refs(thread_id text,ref_key text,ref_value text,updated_at timestamptz,UNIQUE(thread_id,ref_key));
 CREATE TABLE entity_profile_type(id text PRIMARY KEY,company_id text,type text[],archived_at timestamptz);
 CREATE TABLE entity_profile(id text PRIMARY KEY,company_id text,entity_profile_type_id text,name text,ref_key text,created_at timestamptz,last_active_at timestamptz,UNIQUE(company_id,entity_profile_type_id,ref_key));
 INSERT INTO threads(id,company_id) VALUES('thread-a','company'),('thread-b','company');
 INSERT INTO entity_profile_type(id,company_id,type) VALUES('customer-type','company',ARRAY['customer']);`)
	require.NoError(t, err)
	snapshot := registry.Snapshot{AccountID: "account", WorkspaceID: "account", Email: "owner@example.test", HeartbeatIntervalSeconds: 60, GracePeriodSeconds: 300, Entitlements: registry.Entitlements{Revision: "1", EntityProfileLimit: 1}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { require.NoError(t, json.NewEncoder(w).Encode(snapshot)) }))
	defer server.Close()
	licensed, err := registry.Start(ctx, registry.Config{URL: server.URL, LicenseKey: "license", CompanyID: "company"}, pool)
	require.NoError(t, err)
	defer licensed.Close()
	registry.SetDefault(licensed)
	defer registry.SetDefault(nil)
	writer := NewPostgresWriter(pool, zap.NewNop())
	events := []StreamEvent{{Data: map[string]string{"threadId": "thread-a", "refKey": "customer", "refValue": "a"}}, {Data: map[string]string{"threadId": "thread-b", "refKey": "customer", "refValue": "b"}}}
	profiles, err := writer.WriteThreadRefs(ctx, events)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM thread_refs`).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM entity_profile`).Scan(&count))
	require.Equal(t, 1, count)
	// Replay updates the existing profile even at capacity without creating more.
	profiles, err = writer.WriteThreadRefs(ctx, events)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM entity_profile`).Scan(&count))
	require.Equal(t, 1, count)
}
