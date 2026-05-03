package engine

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_Threads_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query { threads(limit: 1) { totalCount } }",
		"variables": map[string]interface{}{},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_Threads_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	_ = enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1)

	// Query as the same company (service account via API key).
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query { threads(limit: 10, offset: 0) { totalCount threads { id contractName } } }",
	})

	require.Empty(t, respA.Errors)
	connA, ok := respA.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	countA, ok := connA["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(countA))

	// User B (different company) should not see User A's threads.
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query { threads(limit: 10, offset: 0) { totalCount } }",
	})
	require.Empty(t, respB.Errors)

	connB, ok := respB.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	countB, ok := connB["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(countB))
}

func TestGraphQL_Threads_ByContractName(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($contractName: String) { threads(contractName: $contractName, limit: 10) { totalCount threads { id contractName } } }",
		Variables: map[string]interface{}{
			"contractName": contractName,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)
	threadObj, ok := threads[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])
	require.Equal(t, contractName, threadObj["contractName"])
}

func TestGraphQL_Threads_ByActor(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($actor: String) { threads(actor: $actor, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"actor": user.ServiceAccountID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))
}

func TestGraphQL_Threads_ByStatus(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	status := "running"
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Status: "running",
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($status: String) { threads(status: $status, limit: 10) { totalCount threads { id status } } }",
		Variables: map[string]interface{}{
			"status": status,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)
	threadObj, ok := threads[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])
	require.Equal(t, status, threadObj["status"])
}

func TestGraphQL_Threads_ByDateRange(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Query for threads started in the last hour (using created_at since started_at doesn't exist)
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour).Format(time.RFC3339)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($startedAfter: String, $startedBefore: String) { threads(startedAfter: $startedAfter, startedBefore: $startedBefore, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"startedAfter":  twoHoursAgo,
			"startedBefore": now.Add(time.Minute).Format(time.RFC3339),
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))
}

func TestGraphQL_Threads_MultipleFilters(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	status := "running"
	_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Status: "running",
	})
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($contractName: String, $status: String) { threads(contractName: $contractName, status: $status, limit: 10) { totalCount threads { id contractName status } } }",
		Variables: map[string]interface{}{
			"contractName": contractName,
			"status":       status,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))
}

func TestGraphQL_Threads_Pagination(t *testing.T) {
	user := setupTestUser(t)

	// Create multiple threads
	contractName := "gql_contract_" + uuid.NewString()[:8]
	for i := 0; i < 5; i++ {
		_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)
	}

	// Test limit
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($limit: Int) { threads(limit: $limit) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"limit": 3,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 5, int(count)) // Total count should be 5

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 3) // But only 3 returned due to limit
}

func TestGraphQL_Threads_Offset(t *testing.T) {
	user := setupTestUser(t)

	// Create multiple threads
	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		threadIDs[i] = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)
	}

	// Test offset
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($offset: Int, $limit: Int) { threads(offset: $offset, limit: $limit) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"offset": 2,
			"limit":  2,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 5, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 2)
}

func TestGraphQL_Threads_NegativeOffset_ClampedToZero(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($offset: Int) { threads(offset: $offset, limit: 10) { totalCount } }",
		Variables: map[string]interface{}{
			"offset": -10,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(count))
}

func TestGraphQL_Threads_DefaultPagination(t *testing.T) {
	user := setupTestUser(t)

	// Test default values (limit: 50, offset: 0)
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query { threads { totalCount threads { id } } }",
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	// Should have default limit of 50
	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(threads), 50)
}

func TestGraphQL_Threads_ContractVersion(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	contractVersion := 2
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, contractVersion)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($contractName: String, $contractVersion: Int) { threads(contractName: $contractName, contractVersion: $contractVersion, limit: 10) { totalCount threads { id contractVersion } } }",
		Variables: map[string]interface{}{
			"contractName":    contractName,
			"contractVersion": contractVersion,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threads"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)
	threadObj, ok := threads[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])
	require.Equal(t, float64(contractVersion), threadObj["contractVersion"])
}
