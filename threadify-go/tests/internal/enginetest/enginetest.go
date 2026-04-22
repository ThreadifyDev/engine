package enginetest

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/apihelper"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/testenv"
)

type TestUser struct {
	ID               string
	Email            string
	Token            string
	ApiKey           string
	CompanyID        string
	ServiceAccountID string
}

func SetupTestUser(t *testing.T, env *testenv.Environment, supabase *apihelper.FakeSupabase) *TestUser {
	t.Helper()

	userID := uuid.NewString()
	companyID := uuid.NewString()
	email := fmt.Sprintf("test-%s@example.com", userID[:8])

	db := dbhelpers.New(env.Postgres.Pool)

	setupTestCompanyAndCredits(t, db, companyID, userID)

	db.CreateTestUser(t, userID, email, companyID)

	serviceAccountID := uuid.NewString()
	db.CreateTestServiceAccount(t, serviceAccountID, "Test Service Account", companyID)

	apiKey := db.CreateTestAPIKey(t, serviceAccountID, companyID, "tf_")
	db.AssignRole(t, serviceAccountID, "service_account", "owner")

	token, err := supabase.MintToken(userID, companyID, email)
	require.NoError(t, err)

	return &TestUser{
		ID:               userID,
		Email:            email,
		Token:            token,
		ApiKey:           apiKey,
		CompanyID:        companyID,
		ServiceAccountID: serviceAccountID,
	}
}

func setupTestCompanyAndCredits(t *testing.T, db *dbhelpers.Helpers, companyID, ownerUserID string) {
	t.Helper()

	db.CreateTestCompany(t, companyID, "Integration Test Company")
	db.AssignRole(t, ownerUserID, "user", "owner")
	db.FundCreditAccount(t, companyID, 10_000_000)
}

// --- Common DB fixtures (to keep DB ops out of individual test files) ---

func CreateThread(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThread(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion)
	return threadID
}

func CreateThreadWithStatus(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int, status string) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThreadWithStatus(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion, status)
	return threadID
}

func CreateThreadWithRefs(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int, refs map[string]interface{}) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThreadWithRefs(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion, refs)
	return threadID
}

func CreateThreadWithRefsAndStatus(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int, refs map[string]interface{}, status string) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThreadWithRefsAndStatus(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion, refs, status)
	return threadID
}

func CreateThreadWithRefsAndTimestamp(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int, refs map[string]interface{}, startedAt time.Time) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThreadWithRefsAndTimestamp(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion, refs, startedAt)
	return threadID
}

