package enginetest

import (
	"fmt"
	"testing"
	"threadify-go/shared/slug"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/apihelper"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/testenv"
)

const defaultTestCredits int64 = 10_000_000

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

	setupTestCompany(t, db, companyID, userID)

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

func setupTestCompany(t *testing.T, db *dbhelpers.Helpers, companyID, ownerUserID string) {
	t.Helper()
	db.CreateTestCompany(t, companyID, "Integration Test Company")
	db.AssignRole(t, ownerUserID, "user", "admin")
	db.FundCreditAccount(t, companyID, defaultTestCredits)
}

func CreateThread(t *testing.T, pool *pgxpool.Pool, user *TestUser, contractName string, contractVersion int, opts ...dbhelpers.ThreadOption) string {
	t.Helper()
	threadID := uuid.NewString()
	dbhelpers.New(pool).CreateTestThread(t, threadID, user.CompanyID, user.ServiceAccountID, contractName, contractVersion, opts...)
	return threadID
}

func CreateEntityProfileType(t *testing.T, pool *pgxpool.Pool, companyID, name string, refKeys []string) string {
	t.Helper()
	id := uuid.NewString()
	s := slug.ToSlug(name)
	dbhelpers.New(pool).CreateEntityProfileType(t, id, companyID, name, refKeys, s)
	return id
}

func CreateEntityProfile(t *testing.T, pool *pgxpool.Pool, companyID, profileTypeID, name, refKey string) string {
	t.Helper()
	id := uuid.NewString()
	dbhelpers.New(pool).CreateEntityProfile(t, id, companyID, profileTypeID, name, refKey)
	return id
}

func CreateEntityProfileWithTimestamp(t *testing.T, pool *pgxpool.Pool, companyID, profileTypeID, name, refKey string, timestamp time.Time) string {
	t.Helper()
	id := uuid.NewString()
	dbhelpers.New(pool).CreateEntityProfileWithTimestamp(t, id, companyID, profileTypeID, name, refKey, timestamp)
	return id
}
