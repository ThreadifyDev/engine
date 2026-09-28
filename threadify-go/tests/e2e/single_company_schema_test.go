package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/tests/internal/testenv"
)

func TestSingleCompanySchemaHasNoCompanyIDIndexes(t *testing.T) {
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
	_, err = db.Pool.Exec(ctx, `INSERT INTO companies(id,name) VALUES('company-a','A')`)
	require.NoError(t, err)
	_, err = db.Pool.Exec(ctx, `INSERT INTO companies(id,name) VALUES('company-b','B')`)
	require.Error(t, err, "an Engine database must reject a second company")
	rows, err := db.Pool.Query(ctx, `SELECT tablename,indexname FROM pg_indexes
		WHERE schemaname=current_schema() AND indexdef ~* '\mcompany_id\M'
		ORDER BY tablename,indexname`)
	require.NoError(t, err)
	defer rows.Close()
	var companyIndexes []string
	for rows.Next() {
		var table, index string
		require.NoError(t, rows.Scan(&table, &index))
		companyIndexes = append(companyIndexes, table+"."+index)
	}
	require.NoError(t, rows.Err())
	require.Empty(t, companyIndexes)
	_, err = db.Pool.Exec(ctx, `CREATE INDEX idx_threads_company_created ON threads(company_id,created_at DESC);
		CREATE INDEX idx_service_accounts_company ON service_accounts(company_id);
		CREATE UNIQUE INDEX idx_contracts_name_company_active ON contracts(name,company_id) WHERE is_deleted=false;
		ALTER TABLE credit_accounts ADD CONSTRAINT credit_accounts_company_id_billing_cycle_start_key UNIQUE(company_id,billing_cycle_start);
		ALTER TABLE entity_profile_type ADD CONSTRAINT entity_profile_type_company_id_type_key UNIQUE(company_id,type);
		ALTER TABLE entity_profile_type ADD CONSTRAINT uq_entity_profile_type_company_slug UNIQUE(company_id,slug);
		ALTER TABLE entity_profile ADD CONSTRAINT entity_profile_company_id_entity_profile_type_id_ref_key_key UNIQUE(company_id,entity_profile_type_id,ref_key);`)
	require.NoError(t, err)
	require.NoError(t, db.InitSchema(ctx))
	var remaining int
	require.NoError(t, db.Pool.QueryRow(ctx, `SELECT count(*) FROM pg_indexes
		WHERE schemaname=current_schema() AND indexdef ~* '\mcompany_id\M'`).Scan(&remaining))
	require.Zero(t, remaining, "legacy company indexes should be removed")
	_, err = db.Pool.Exec(ctx, `DROP INDEX idx_companies_singleton;
		INSERT INTO companies(id,name) VALUES('company-b','B')`)
	require.NoError(t, err)
	err = db.InitSchema(ctx)
	require.ErrorContains(t, err, "supports one company")
	var companies int
	require.NoError(t, db.Pool.QueryRow(ctx, `SELECT count(*) FROM companies`).Scan(&companies))
	require.Equal(t, 2, companies, "failed upgrade must preserve existing companies")
}
