package engine

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func setupTestUser(t *testing.T) *TestUser {
	userId := uuid.NewString()
	companyId := uuid.NewString()
	email := fmt.Sprintf("test-%s@example.com", userId[:8])

	db := newDBHelpers(env.Postgres.Pool)

	setupTestCompanyAndCredits(t, companyId, userId, db)

	db.CreateTestUser(t, userId, email, companyId)

	saId := uuid.NewString()
	saName := "Test Service Account"
	db.CreateTestServiceAccount(t, saId, saName, companyId)

	apiKey := db.CreateTestAPIKey(t, saId, companyId, "tf_")

	db.AssignRole(t, saId, "service_account", "owner")

	token, err := supabase.MintToken(userId, companyId, email)
	require.NoError(t, err)

	return &TestUser{
		ID:        userId,
		Email:     email,
		Token:     token,
		ApiKey:    apiKey,
		CompanyID: companyId,
	}
}

func setupTestCompanyAndCredits(t *testing.T, companyID, ownerID string, db *dbHelpers) {
	db.CreateTestCompany(t, companyID, "Integration Test Company")

	db.AssignRole(t, ownerID, "user", "owner")
	db.FundCreditAccount(t, companyID, 10000000)
}
