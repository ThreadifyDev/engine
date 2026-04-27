package engine

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_ThreadChain_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query($rootId: ID!) { threadChain(rootId: $rootId) { id } }",
		"variables": map[string]interface{}{"rootId": uuid.NewString()},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_ThreadChain_SingleThread(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!) { threadChain(rootId: $rootId) { id contractName } }",
		Variables: map[string]interface{}{
			"rootId": threadID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	threads, ok := gqlResp.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)

	threadObj, ok := threads[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])
	require.Equal(t, contractName, threadObj["contractName"])
}

func TestGraphQL_ThreadChain_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1)

	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!) { threadChain(rootId: $rootId) { id } }",
		Variables: map[string]interface{}{
			"rootId": threadID,
		},
	})
	require.Empty(t, respA.Errors)
	threadsA, ok := respA.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threadsA, 1)

	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!) { threadChain(rootId: $rootId) { id } }",
		Variables: map[string]interface{}{
			"rootId": threadID,
		},
	})
	require.Empty(t, respB.Errors)
	threadsB, ok := respB.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threadsB, 0)
}

func TestGraphQL_ThreadChain_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!) { threadChain(rootId: $rootId) { id } }",
		Variables: map[string]interface{}{
			"rootId": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	threads, ok := gqlResp.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 0)
}

func TestGraphQL_ThreadChain_MaxDepth(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	rootThreadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!, $maxDepth: Int) { threadChain(rootId: $rootId, maxDepth: $maxDepth) { id } }",
		Variables: map[string]interface{}{
			"rootId":   rootThreadID,
			"maxDepth": 1,
		},
	})

	require.Empty(t, gqlResp.Errors)
	threads, ok := gqlResp.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)
}

func TestGraphQL_ThreadChain_DefaultMaxDepth(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!) { threadChain(rootId: $rootId) { id } }",
		Variables: map[string]interface{}{
			"rootId": threadID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	threads, ok := gqlResp.Data["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 1)
}

func TestGraphQL_ThreadChain_ZeroMaxDepth(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!, $maxDepth: Int) { threadChain(rootId: $rootId, maxDepth: $maxDepth) { id } }",
		Variables: map[string]interface{}{
			"rootId":   threadID,
			"maxDepth": 0,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
}

func TestGraphQL_ThreadChain_NegativeMaxDepth_Clamped(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($rootId: ID!, $maxDepth: Int) { threadChain(rootId: $rootId, maxDepth: $maxDepth) { id } }",
		Variables: map[string]interface{}{
			"rootId":   threadID,
			"maxDepth": -5,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
}

func TestGraphQL_Thread_ThreadChain(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($id: ID!) { thread(id: $id) { id threadChain { id } } }",
		Variables: map[string]interface{}{
			"id": threadID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	thread, ok := gqlResp.Data["thread"].(map[string]interface{})
	require.True(t, ok)

	chain, ok := thread["threadChain"].([]interface{})
	require.True(t, ok)
	require.Len(t, chain, 1)

	threadObj, ok := chain[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, threadID, threadObj["id"])
}
