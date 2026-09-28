package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/tests/internal/testenv"
)

func TestAccessIndexMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pg, err := testenv.StartPostgresContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		require.NoError(t, pg.Terminate(cleanupCtx))
	})
	db, err := database.NewPostgresDB(pg.ConnectionString, 4)
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.InitSchema(ctx))
	_, err = db.Pool.Exec(ctx, `CREATE INDEX idx_thread_access_thread_id ON thread_access(thread_id);
		CREATE INDEX idx_thread_access_thread_user_status ON thread_access(thread_id,user_id,status);`)
	require.NoError(t, err)
	require.NoError(t, db.InitSchema(ctx))
	var remaining int
	require.NoError(t, db.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes
		WHERE schemaname=current_schema() AND tablename='thread_access'
		AND indexname IN ('idx_thread_access_thread_id','idx_thread_access_thread_user_status')`).Scan(&remaining))
	require.Zero(t, remaining)
	var uniqueIndex string
	require.NoError(t, db.Pool.QueryRow(ctx, `SELECT indexname FROM pg_indexes
		WHERE schemaname=current_schema() AND tablename='thread_access'
		AND indexname='thread_access_thread_id_user_id_key'`).Scan(&uniqueIndex))
}
