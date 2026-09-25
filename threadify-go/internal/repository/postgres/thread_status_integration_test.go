package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestUpdateThreadStatus(t *testing.T) {
	dsn := os.Getenv("THREADIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_TEST_DATABASE_URL to test real PostgreSQL")
	}

	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()

	schema := fmt.Sprintf("thread_status_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()

	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer pool.Close()

	_, err = pool.Exec(ctx, `
CREATE TABLE threads (
	id text PRIMARY KEY,
	status varchar(50) DEFAULT 'active',
	updated_at timestamp NOT NULL DEFAULT now(),
	completed_at timestamp,
	closed_at timestamp
);
INSERT INTO threads (id, status) VALUES ('thread-1', 'active');`)
	require.NoError(t, err)

	repo := NewThreadRepository(pool)
	timestamp := time.Date(2026, 9, 25, 0, 49, 1, 970000000, time.UTC)

	for _, test := range []struct {
		name          string
		status        string
		wantCompleted *time.Time
		wantClosed    *time.Time
	}{
		{name: "completed", status: "completed", wantCompleted: &timestamp},
		{name: "closed", status: "closed", wantClosed: &timestamp},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, `UPDATE threads SET status='active', updated_at=now(), completed_at=NULL, closed_at=NULL WHERE id='thread-1'`)
			require.NoError(t, err)

			err = repo.UpdateThreadStatus(ctx, "thread-1", test.status, timestamp)
			require.NoError(t, err)

			var status string
			var updatedAt time.Time
			var completedAt, closedAt *time.Time
			err = pool.QueryRow(ctx, `SELECT status, updated_at, completed_at, closed_at FROM threads WHERE id='thread-1'`).Scan(&status, &updatedAt, &completedAt, &closedAt)
			require.NoError(t, err)
			require.Equal(t, test.status, status)
			require.True(t, timestamp.Equal(updatedAt))
			if test.wantCompleted == nil {
				require.Nil(t, completedAt)
			} else {
				require.NotNil(t, completedAt)
				require.True(t, test.wantCompleted.Equal(*completedAt))
			}
			if test.wantClosed == nil {
				require.Nil(t, closedAt)
			} else {
				require.NotNil(t, closedAt)
				require.True(t, test.wantClosed.Equal(*closedAt))
			}
		})
	}
}
