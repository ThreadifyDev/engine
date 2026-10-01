package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func setupSingleCompanyTestUser(t *testing.T) *enginetest.TestUser {
	t.Helper()
	const companyID = "8bf9099d-2ff9-4d88-a2eb-acb114679909"
	db := dbhelpers.New(env.Postgres.Pool)
	userID := uuid.NewString()
	serviceAccountID := uuid.NewString()
	email := fmt.Sprintf("completion-%s@example.com", userID[:8])
	db.CreateTestCompany(t, companyID, "Integration Test Company")
	db.CreateTestUser(t, userID, email, companyID)
	db.CreateTestServiceAccount(t, serviceAccountID, "Completion Test Account", companyID)
	apiKey := db.CreateTestAPIKey(t, serviceAccountID, companyID, "tf_")
	db.AssignRole(t, serviceAccountID, "service_account", "owner")
	db.AssignRole(t, userID, "user", "admin")
	db.FundCreditAccount(t, companyID, 10_000_000)
	token, err := supabase.MintToken(userID, companyID, email)
	require.NoError(t, err)
	return &enginetest.TestUser{ID: userID, Email: email, Token: token, ApiKey: apiKey, CompanyID: companyID, ServiceAccountID: serviceAccountID}
}

func TestWebSocket_ContractFreeCompletion_VisibleInGraphQL(t *testing.T) {
	user := setupSingleCompanyTestUser(t)
	conn := wsConnectAndAuth(t, user.ApiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action": "startThread",
		"label":  "completion consistency test",
	})
	start := enginetest.ReadWSAction(t, conn, "startThread", 10*time.Second)
	require.Equal(t, "success", start["status"], "%v", start)
	threadID, ok := start["threadId"].(string)
	require.True(t, ok)
	require.NotEmpty(t, threadID)

	// The engine integration harness runs without its archiver, so seed the
	// archived row that a combined deployment would have written.
	dbhelpers.New(env.Postgres.Pool).CreateTestThread(t, threadID, user.CompanyID, user.ServiceAccountID, "", 0)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":   "threadEnd",
		"threadId": threadID,
		"status":   "completed",
	})
	ended := enginetest.ReadWSAction(t, conn, "threadEnd", 10*time.Second)
	require.Equal(t, "success", ended["status"], "%v", ended)

	require.Eventually(t, func() bool {
		_, response := doGraphQL(t, "", user.ApiKey, graphQLRequest{
			Query:     `query($id: ID!) { thread(id: $id) { status completedAt } }`,
			Variables: map[string]interface{}{"id": threadID},
		})
		thread, ok := response.Data["thread"].(map[string]interface{})
		return len(response.Errors) == 0 && ok && thread["status"] == "completed" && thread["completedAt"] != nil
	}, 10*time.Second, 100*time.Millisecond)

	require.Eventually(t, func() bool {
		var status string
		var completedAt *time.Time
		return env.Postgres.Pool.QueryRow(testCtx, `SELECT status, completed_at FROM threads WHERE id=$1`, threadID).Scan(&status, &completedAt) == nil && status == "completed" && completedAt != nil
	}, 10*time.Second, 100*time.Millisecond)

	_, listResponse := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `query { threads(status: "completed", limit: 100) { threads { id status completedAt } } }`,
	})
	require.Empty(t, listResponse.Errors)
	connection, ok := listResponse.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	listed, ok := connection["threads"].([]interface{})
	require.True(t, ok)
	found := false
	for _, item := range listed {
		row, ok := item.(map[string]interface{})
		if ok && row["id"] == threadID {
			require.Equal(t, "completed", row["status"])
			require.NotNil(t, row["completedAt"])
			found = true
		}
	}
	require.True(t, found, "completed thread missing from list query")
}
