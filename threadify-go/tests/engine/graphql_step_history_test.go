package engine

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_StepHistory_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query": "query($threadId: String!, $stepName: String!) { stepHistory(threadId: $threadId, stepName: $stepName) { attempt } }",
		"variables": map[string]interface{}{
			"threadId": uuid.NewString(),
			"stepName": "testStep",
		},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_StepHistory_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	// Create thread for User A
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1)

	// User A should be able to query step history (even if empty)
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!) { stepHistory(threadId: $threadId, stepName: $stepName) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
		},
	})
	require.Empty(t, respA.Errors)
	historyA, ok := respA.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	// Should return empty array, not error
	require.Len(t, historyA, 0)

	// User B should get empty result for User A's thread
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!) { stepHistory(threadId: $threadId, stepName: $stepName) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
		},
	})
	require.Empty(t, respB.Errors)
	historyB, ok := respB.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.Len(t, historyB, 0)
}

func TestGraphQL_StepHistory_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!) { stepHistory(threadId: $threadId, stepName: $stepName) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": uuid.NewString(),
			"stepName": "nonexistentStep",
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.Len(t, history, 0)
}

func TestGraphQL_StepHistory_DefaultPagination(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test default values (limit: 100, offset: 0)
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!) { stepHistory(threadId: $threadId, stepName: $stepName) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_Pagination(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test limit
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $limit: Int) { stepHistory(threadId: $threadId, stepName: $stepName, limit: $limit) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"limit":    5,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 5)
}

func TestGraphQL_StepHistory_Offset(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test offset
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $offset: Int) { stepHistory(threadId: $threadId, stepName: $stepName, offset: $offset) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"offset":   10,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_NegativeOffset_ClampedToZero(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $offset: Int) { stepHistory(threadId: $threadId, stepName: $stepName, offset: $offset) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"offset":   -10,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_WithIdempotencyKey(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"
	idempotencyKey := uuid.NewString()

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String) { stepHistory(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { attempt timestamp } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       stepName,
			"idempotencyKey": idempotencyKey,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.Len(t, history, 0) // Should be empty since no step history exists
}

func TestGraphQL_StepHistory_DateFilters(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test date range filters
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour).Format(time.RFC3339)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $startAt: String, $endAt: String) { stepHistory(threadId: $threadId, stepName: $stepName, startAt: $startAt, endAt: $endAt) { attempt timestamp } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"startAt":  twoHoursAgo,
			"endAt":    now.Format(time.RFC3339),
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_ActivityTypeFilter(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $activityType: String) { stepHistory(threadId: $threadId, stepName: $stepName, activityType: $activityType) { attempt timestamp } }",
		Variables: map[string]interface{}{
			"threadId":     threadID,
			"stepName":     stepName,
			"activityType": "execution",
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_ActorFilter(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $actor: String) { stepHistory(threadId: $threadId, stepName: $stepName, actor: $actor) { attempt actor } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"actor":    user.ServiceAccountID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 100)
}

func TestGraphQL_StepHistory_MaxLimit(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test maximum limit (1000)
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $limit: Int) { stepHistory(threadId: $threadId, stepName: $stepName, limit: $limit) { attempt } }",
		Variables: map[string]interface{}{
			"threadId": threadID,
			"stepName": stepName,
			"limit":    1000,
		},
	})

	require.Empty(t, gqlResp.Errors)
	history, ok := gqlResp.Data["stepHistory"].([]interface{})
	require.True(t, ok)
	require.LessOrEqual(t, len(history), 1000)
}
