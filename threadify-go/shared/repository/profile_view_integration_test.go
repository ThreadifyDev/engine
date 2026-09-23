package repository

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	shareddb "threadify-go/shared/database"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
)

func TestProfileViewPostgres(t *testing.T) {
	dsn := os.Getenv("THREADIFY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_TEST_DATABASE_URL for isolated-schema PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "profile_view_" + uuid.NewString()[:8]
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE entity_profile_type(id text PRIMARY KEY, company_id text NOT NULL, name text, archived_at timestamptz);
 INSERT INTO entity_profile_type(id,company_id,name) VALUES('customers','company-a','Customers'),('other','company-b','Other');`)
	require.NoError(t, err)
	// The actual startup migration must work against existing rows and be repeatable.
	for i := 0; i < 2; i++ {
		_, err = pool.Exec(ctx, shareddb.ProfileViewSchema)
		require.NoError(t, err)
	}
	repo := NewProfileViewRepository(pool)
	initial, err := repo.Get(ctx, "company-a", "customers")
	require.NoError(t, err)
	require.Nil(t, initial.Definition)
	require.Zero(t, initial.Revision)
	input := domain.ProfileViewDefinition{SchemaVersion: 1, Title: "Health", Range: "30d", Columns: 2, Blocks: []domain.ProfileViewBlock{{ID: "health", Source: "deliveryHealth", Title: "Health"}}}
	saved, err := repo.Save(ctx, "company-a", "customers", "editor-a", 0, input)
	require.NoError(t, err)
	require.EqualValues(t, 1, saved.Revision)
	require.Equal(t, "editor-a", saved.UpdatedBy)
	require.NotNil(t, saved.UpdatedAt)
	read, err := NewProfileViewRepository(pool).Get(ctx, "company-a", "customers")
	require.NoError(t, err)
	require.Equal(t, saved, read)
	require.Equal(t, "$entity.refKey", read.Requests[0].Parameters["refKey"])
	_, err = repo.Get(ctx, "company-b", "customers")
	require.ErrorIs(t, err, serror.ErrEntityProfileTypeNotFound)
	_, err = repo.Save(ctx, "company-b", "customers", "editor-b", 1, input)
	require.ErrorIs(t, err, serror.ErrEntityProfileTypeNotFound)
	_, err = repo.Save(ctx, "company-a", "customers", "editor-a", 0, input)
	require.ErrorIs(t, err, domain.ErrProfileViewConflict)
	// Exactly one writer can commit against the same revision.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, actor := range []string{"editor-a", "editor-b"} {
		wg.Add(1)
		go func(actor string) {
			defer wg.Done()
			_, err := repo.Save(ctx, "company-a", "customers", actor, 1, input)
			results <- err
		}(actor)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, domain.ErrProfileViewConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)
	read, err = repo.Get(ctx, "company-a", "customers")
	require.NoError(t, err)
	require.EqualValues(t, 2, read.Revision)
	_, err = pool.Exec(ctx, `CREATE TABLE entity_profile_type_metrics(id uuid PRIMARY KEY, entity_profile_type_id text);
INSERT INTO entity_profile_type_metrics VALUES ('11111111-1111-4111-8111-111111111111', 'customers'), ('22222222-2222-4222-8222-222222222222', 'other');`)
	require.NoError(t, err)
	metricView := domain.ProfileViewDefinition{SchemaVersion: 1, Title: "Metric", Range: "30d", Columns: 1, Blocks: []domain.ProfileViewBlock{{ID: "configured", Source: "configuredMetric", Title: "Count", MetricID: "22222222-2222-4222-8222-222222222222", Display: "card"}}}
	_, err = repo.Save(ctx, "company-a", "customers", "editor-a", 2, metricView)
	require.ErrorIs(t, err, domain.ErrProfileMetricBinding)
	metricView.Blocks[0].MetricID = "11111111-1111-4111-8111-111111111111"
	bound, err := repo.Save(ctx, "company-a", "customers", "editor-a", 2, metricView)
	require.NoError(t, err)
	require.Equal(t, metricView.Blocks[0].MetricID, bound.Requests[0].Parameters["metricId"])
	_, err = pool.Exec(ctx, `UPDATE entity_profile_type SET name='Renamed' WHERE id='customers'`)
	require.NoError(t, err)
	read, err = repo.Get(ctx, "company-a", "customers")
	require.NoError(t, err)
	require.EqualValues(t, 3, read.Revision)
	_, err = pool.Exec(ctx, `UPDATE entity_profile_type SET archived_at=NOW() WHERE id='customers'`)
	require.NoError(t, err)
	_, err = repo.Get(ctx, "company-a", "customers")
	require.ErrorIs(t, err, serror.ErrEntityProfileTypeNotFound)
	_, err = repo.Save(ctx, "company-a", "customers", "editor-a", 2, input)
	require.ErrorIs(t, err, serror.ErrEntityProfileTypeNotFound)
}
