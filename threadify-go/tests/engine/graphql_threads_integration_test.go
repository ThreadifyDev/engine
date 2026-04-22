package engine

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func startThreadViaWS(t *testing.T, apiKey, contractName, role string) string {
	t.Helper()
	conn := wsConnectAndAuth(t, apiKey)

	enginetest.SendWSJSON(t, conn, map[string]interface{}{
		"action":       "startThread",
		"contractName": contractName,
		"role":         role,
	})

	resp := enginetest.ReadWSWithTimeout(t, conn, 5*time.Second)
	require.Equal(t, "success", resp["status"], "startThread resp: %v", resp)
	threadID, _ := resp["threadId"].(string)
	require.NotEmpty(t, threadID)
	return threadID
}

func TestGraphQL_Thread_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query($id: ID!) { thread(id: $id) { id } }",
		"variables": map[string]interface{}{"id": uuid.NewString()},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_Thread_CrossCompanyDenied(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := wsSetupContract(t, userA.Token,
		[]string{"actor1"},
		[]map[string]interface{}{{"id": "stepA", "owner": "actor1", "type": "managed"}},
		[]map[string]interface{}{},
		[]string{"stepA"},
		[]string{"stepA"},
	)

	threadID := startThreadViaWS(t, userA.ApiKey, contractName, "actor1")

	_, gqlResp := doGraphQL(t, userB.Token, "", graphQLRequest{
		Query: "query($id: ID!) { thread(id: $id) { id companyId contractName } }",
		Variables: map[string]interface{}{
			"id": threadID,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
}

func TestGraphQL_ThreadsByContract_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1)

	// Query as the same company (service account via API key).
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($name: String!) { threadsByContract(contractName: $name, limit: 10, offset: 0) { totalCount threads { id contractName } } }",
		Variables: map[string]interface{}{
			"name": contractName,
		},
	})
	require.Empty(t, respA.Errors)

	connA, ok := respA.Data["threadsByContract"].(map[string]interface{})
	require.True(t, ok)
	countA, ok := connA["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(countA))

	threadsA, ok := connA["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threadsA, 1)
	threadObj, ok := threadsA[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])

	// User B (different company) should not see User A's threads.
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($name: String!) { threadsByContract(contractName: $name, limit: 10, offset: 0) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"name": contractName,
		},
	})
	require.Empty(t, respB.Errors)

	connB, ok := respB.Data["threadsByContract"].(map[string]interface{})
	require.True(t, ok)
	countB, ok := connB["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(countB))
}

func TestGraphQL_ThreadsByContract_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query($name: String!) { threadsByContract(contractName: $name, limit: 1, offset: 0) { totalCount } }",
		"variables": map[string]interface{}{"name": "nope"},
	})
	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_ThreadsByContract_NegativeOffset_ClampedToZero(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($name: String!, $offset: Int!) { threadsByContract(contractName: $name, limit: 10, offset: $offset) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"name":   contractName,
			"offset": -10,
		},
	})
	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByContract"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(count))
}

func TestGraphQL_Thread_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, user.Token, "", graphQLRequest{
		Query: "query($id: ID!) { thread(id: $id) { id } }",
		Variables: map[string]interface{}{
			"id": uuid.NewString(),
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
	if gqlResp.Data != nil {
		_, ok := gqlResp.Data["thread"]
		require.True(t, ok)
		require.Nil(t, gqlResp.Data["thread"])
	}
}

func TestGraphQL_RecordLLMUsage_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "mutation($tokens: Int!) { recordLLMUsage(tokens: $tokens) }",
		"variables": map[string]interface{}{"tokens": 10},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_RecordLLMUsage_Success(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, user.Token, "", graphQLRequest{
		Query: "mutation($tokens: Int!) { recordLLMUsage(tokens: $tokens) }",
		Variables: map[string]interface{}{
			"tokens": 10,
		},
	})

	require.Empty(t, gqlResp.Errors)
	v, ok := gqlResp.Data["recordLLMUsage"].(bool)
	require.True(t, ok)
	require.True(t, v)
}
