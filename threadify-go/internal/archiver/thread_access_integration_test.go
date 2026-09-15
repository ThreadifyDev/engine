package archiver

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Uses a private schema and the real PostgreSQL ON CONFLICT behavior that mocks
// cannot reproduce. Rejoins must not poison a whole persistence batch.
func TestWriteThreadAccess_RepeatedJoins(t *testing.T) {
	dsn := os.Getenv("THREADIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires explicitly supplied test PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "join_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE threads(id text PRIMARY KEY);
 CREATE TABLE thread_access(thread_id text,user_id text,roles jsonb,runtime_role text,permissions text[],granted_by text,granted_at timestamptz,status text, UNIQUE(thread_id,user_id));
 INSERT INTO threads VALUES('thread-a'),('thread-b');`)
	require.NoError(t, err)
	event := func(thread, user, role, status string) StreamEvent {
		return StreamEvent{Data: map[string]string{"threadId": thread, "userId": user, "roles": "[\"warehouse\"]", "runtime_role": role, "permissions": "[]", "grantedBy": "orders", "grantedAt": time.Now().UTC().Format(time.RFC3339Nano), "status": status}}
	}
	events := []StreamEvent{event("thread-a", "orders", "owner", "active"), event("thread-a", "warehouse", "observer", "active"), event("thread-a", "warehouse", "participant", "active"), event("thread-b", "warehouse", "observer", "active"), event("thread-a", "orders", "owner", "revoked")}
	writer := NewPostgresWriter(pool, zap.NewNop())
	require.NoError(t, writer.WriteThreadAccess(ctx, events))
	// Retrying an acknowledged-late batch must also be idempotent.
	require.NoError(t, writer.WriteThreadAccess(ctx, events))
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM thread_access").Scan(&count))
	require.Equal(t, 3, count)
	for _, want := range []struct{ thread, user, role, status string }{{"thread-a", "orders", "owner", "revoked"}, {"thread-a", "warehouse", "participant", "active"}, {"thread-b", "warehouse", "observer", "active"}} {
		var role, status string
		require.NoError(t, pool.QueryRow(ctx, "SELECT runtime_role,status FROM thread_access WHERE thread_id=$1 AND user_id=$2", want.thread, want.user).Scan(&role, &status))
		require.Equal(t, want.role, role)
		require.Equal(t, want.status, status)
	}
}
