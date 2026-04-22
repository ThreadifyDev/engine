package dbhelpers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"threadify-go/shared/models"
	"threadify-go/shared/repository"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type Helpers struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Helpers {
	return &Helpers{pool: pool}
}

func (db *Helpers) CreateTestCompany(t *testing.T, companyID, name string) {
	_, err := db.pool.Exec(context.Background(),
		"INSERT INTO companies (id, name, external_customer_id) VALUES ($1, $2, $3) ON CONFLICT (id) DO NOTHING",
		companyID, name, "cus_"+uuid.NewString()[:8])
	require.NoError(t, err)
}

func (db *Helpers) CreateTestUser(t *testing.T, userID, email, companyID string) {
	_, err := db.pool.Exec(context.Background(),
		"INSERT INTO users (id, email, company_id) VALUES ($1, $2, $3) ON CONFLICT (email) DO NOTHING",
		userID, email, companyID)
	require.NoError(t, err)
}

func (db *Helpers) CreateTestThread(t *testing.T, threadID, companyID, ownerID, contractName string, contractVersion int) {
	t.Helper()
	_, err := db.pool.Exec(context.Background(), `
		INSERT INTO threads (
			id, contract_name, contract_version, owner_id, company_id, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,'active',NOW(),NOW())
		ON CONFLICT (id) DO NOTHING`,
		threadID, contractName, contractVersion, ownerID, companyID,
	)
	require.NoError(t, err)
}

func (db *Helpers) CreateTestServiceAccount(t *testing.T, saID, name, companyID string) {
	_, err := db.pool.Exec(context.Background(),
		"INSERT INTO service_accounts (id, company_id, name, is_active) VALUES ($1, $2, $3, $4)",
		saID, companyID, name, true)
	require.NoError(t, err)
}

func (db *Helpers) CreateTestAPIKey(t *testing.T, saID, companyID, apiKeyPrefix string) string {
	apiKey := apiKeyPrefix + uuid.NewString()
	hash := sha256.Sum256([]byte(apiKey))

	_, err := db.pool.Exec(context.Background(),
		"INSERT INTO api_keys (id, service_account_id, company_id, key_hash, key_prefix, is_active) VALUES ($1, $2, $3, $4, $5, $6)",
		uuid.NewString(), saID, companyID, hex.EncodeToString(hash[:]), apiKeyPrefix, true)
	require.NoError(t, err)

	return apiKey
}

func (db *Helpers) AssignRole(t *testing.T, principalID, principalType, roleName string) {
	_, err := db.pool.Exec(context.Background(),
		"INSERT INTO user_roles (principal_id, principal_type, role_name, assigned_by) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING",
		principalID, principalType, roleName, "system")
	require.NoError(t, err)
}

func (db *Helpers) FundCreditAccount(t *testing.T, companyID string, amountMillicents int64) {
	err := repository.NewPlanRepo(db.pool).CreateCreditAccount(context.Background(), &models.CreditAccount{
		ID:                      uuid.NewString(),
		CompanyID:               companyID,
		CreditBalanceMillicents: amountMillicents,
		BillingCycleStart:       time.Now().UTC().Truncate(24 * time.Hour),
		RateLimitTPS:            100,
		PayloadLimitBytes:       1024 * 1024,
	})
	if err != nil {
		fmt.Printf("Note: credit account creation error (ignoring if exists): %v\n", err)
	}
}

func (db *Helpers) GetExternalCustomerID(t *testing.T, companyID string) string {
	t.Helper()
	planRepo := repository.NewPlanRepo(db.pool)
	externalID, err := planRepo.GetExternalCustomerID(context.Background(), companyID)
	require.NoError(t, err)
	require.NotEmpty(t, externalID, "company %s must have an external_customer_id", companyID)
	return externalID
}

func (db *Helpers) GetCreditBalance(t *testing.T, companyID string) int64 {
	t.Helper()
	account, err := repository.NewPlanRepo(db.pool).GetCreditAccount(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, account, "credit account must exist for company %s", companyID)
	return account.CreditBalanceMillicents
}

func (db *Helpers) CreateTestInvoice(t *testing.T, companyID, externalInvoiceID string, totalCents int64) {
	t.Helper()
	now := time.Now()
	_, err := db.pool.Exec(context.Background(),
		`INSERT INTO billing_snapshots
			(id, company_id, external_invoice_id, total_cents, payment_status, reason, period_start, period_end)
		 VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7)`,
		uuid.NewString(),
		companyID,
		externalInvoiceID,
		totalCents,
		string(models.SnapshotReasonCreditTopup),
		now,
		now.Add(30*24*time.Hour),
	)
	require.NoError(t, err, "must be able to insert test invoice")
}

func (db *Helpers) GetInvoiceStatus(t *testing.T, externalInvoiceID string) string {
	t.Helper()
	var status string
	err := db.pool.QueryRow(context.Background(),
		"SELECT payment_status FROM billing_snapshots WHERE external_invoice_id = $1",
		externalInvoiceID,
	).Scan(&status)
	require.NoError(t, err, "invoice %s must exist", externalInvoiceID)
	return status
}

func (db *Helpers) CreateTestThreadWithRefs(t *testing.T, threadID, companyID, ownerID, contractName string, contractVersion int, refs map[string]interface{}) {
	t.Helper()

	// Create the thread first
	db.CreateTestThread(t, threadID, companyID, ownerID, contractName, contractVersion)

	// Insert refs into the thread_refs table
	for refKey, refValue := range refs {
		refValueStr := fmt.Sprintf("%v", refValue)
		_, err := db.pool.Exec(context.Background(), `
			INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (thread_id, ref_key) DO UPDATE SET
				ref_value = EXCLUDED.ref_value,
				updated_at = NOW()`,
			threadID, refKey, refValueStr,
		)
		require.NoError(t, err)
	}
}

func (db *Helpers) CreateTestThreadWithStatus(t *testing.T, threadID, companyID, ownerID, contractName string, contractVersion int, status string) {
	t.Helper()
	_, err := db.pool.Exec(context.Background(), `
		INSERT INTO threads (
			id, contract_name, contract_version, owner_id, company_id, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())
		ON CONFLICT (id) DO NOTHING`,
		threadID, contractName, contractVersion, ownerID, companyID, status,
	)
	require.NoError(t, err)
}

func (db *Helpers) CreateTestThreadWithRefsAndStatus(t *testing.T, threadID, companyID, ownerID, contractName string, contractVersion int, refs map[string]interface{}, status string) {
	t.Helper()
	// Create the thread first (threads.refs column no longer exists; refs live in thread_refs)
	db.CreateTestThreadWithStatus(t, threadID, companyID, ownerID, contractName, contractVersion, status)

	// Insert refs into the thread_refs table
	for refKey, refValue := range refs {
		refValueStr := fmt.Sprintf("%v", refValue)
		_, err := db.pool.Exec(context.Background(), `
			INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (thread_id, ref_key) DO UPDATE SET
				ref_value = EXCLUDED.ref_value,
				updated_at = NOW()`,
			threadID, refKey, refValueStr,
		)
		require.NoError(t, err)
	}
}

func (db *Helpers) CreateTestThreadWithRefsAndTimestamp(t *testing.T, threadID, companyID, ownerID, contractName string, contractVersion int, refs map[string]interface{}, startedAt time.Time) {
	t.Helper()
	// Create thread with a deterministic created_at for date-range filter tests.
	_, err := db.pool.Exec(context.Background(), `
		INSERT INTO threads (
			id, contract_name, contract_version, owner_id, company_id, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,'active',$6,$6)
		ON CONFLICT (id) DO NOTHING`,
		threadID, contractName, contractVersion, ownerID, companyID, startedAt,
	)
	require.NoError(t, err)

	// Insert refs into the thread_refs table
	for refKey, refValue := range refs {
		refValueStr := fmt.Sprintf("%v", refValue)
		_, err := db.pool.Exec(context.Background(), `
			INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
			ON CONFLICT (thread_id, ref_key) DO UPDATE SET
				ref_value = EXCLUDED.ref_value,
				updated_at = NOW()`,
			threadID, refKey, refValueStr,
		)
		require.NoError(t, err)
	}
}
