package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDeliveryOutcomeTrendPostgres(t *testing.T) {
	dsn := os.Getenv("THREADIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_TEST_DATABASE_URL to test real PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("delivery_trend_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer func() { _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.ConnConfig.RuntimeParams["search_path"] = schema
	config.ConnConfig.RuntimeParams["timezone"] = "Pacific/Auckland"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE threads (id text, company_id text, status text, created_at timestamp);
CREATE TABLE thread_refs (thread_id text, ref_key text, ref_value text);
INSERT INTO threads VALUES
('complete', 'a', 'completed', '2026-09-22T00:30:00'),
('failure', 'a', 'failed', '2026-09-21T12:00:00Z'),
('cancel', 'a', 'cancelled', '2026-09-21T13:00:00Z'),
('active', 'a', 'active', '2026-09-22T09:00:00Z'),
('other-tenant', 'b', 'failed', '2026-09-22T09:00:00Z'),
('old', 'a', 'failed', '2026-08-01T00:00:00Z'),
('future', 'a', 'failed', '2026-09-23T00:00:00Z'),
('other-ref', 'a', 'failed', '2026-09-22T09:00:00Z');
INSERT INTO thread_refs SELECT id, 'customer', 'one' FROM threads WHERE id != 'other-ref';
INSERT INTO thread_refs VALUES ('complete', 'account', 'one'), ('other-ref', 'customer', 'two');`)
	require.NoError(t, err)
	repo := NewMetricsRepository(pool, nil, zap.NewNop())
	end := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	for _, days := range []int{7, 30, 90} {
		rows, err := repo.DeliveryOutcomeTrend(ctx, "a", "one", []string{"customer", "account"}, days, end)
		require.NoError(t, err)
		require.Len(t, rows, days)
		require.Equal(t, "2026-09-22", rows[days-1]["date"])
		require.EqualValues(t, 1, rows[days-1]["completed"])
		require.EqualValues(t, 0, rows[days-1]["failed"])
		require.EqualValues(t, 2, rows[days-1]["total"])
		require.EqualValues(t, 2, rows[days-2]["failed"])
		require.EqualValues(t, 0, rows[days-3]["total"])
	}
	_, err = repo.DeliveryOutcomeTrend(ctx, "", "one", []string{"customer"}, 30, end)
	require.Error(t, err)
	_, err = repo.DeliveryOutcomeTrend(ctx, "a", "one", []string{"customer"}, 365, end)
	require.Error(t, err)
}
