package engine

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_ValidationResults_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query": "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		"variables": map[string]interface{}{
			"threadId":       uuid.NewString(),
			"stepName":       "testStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_ValidationResults_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	stepName := "testStep"
	idempotencyKey := uuid.NewString()

	// Create thread for User A
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1)

	// User A should be able to query validation results (even if empty)
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       stepName,
			"idempotencyKey": idempotencyKey,
		},
	})
	require.Empty(t, respA.Errors)
	resultsA, ok := respA.Data["validationResults"].([]interface{})
	require.True(t, ok)
	// Should return empty array, not error
	require.Len(t, resultsA, 0)

	// User B should get empty result for User A's thread
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       stepName,
			"idempotencyKey": idempotencyKey,
		},
	})
	require.Empty(t, respB.Errors)
	resultsB, ok := respB.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, resultsB, 0)
}

func TestGraphQL_ValidationResults_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       uuid.NewString(),
			"stepName":       "nonexistentStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}

func TestGraphQL_ValidationResults_RequiredParameters(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test with all required parameters
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId stepName idempotencyKey } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       "testStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0) // Should be empty since no validation results exist
}

func TestGraphQL_ValidationResults_ResponseStructure(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId threadId stepId stepName idempotencyKey timestamp overallStatus hasCriticalViolation criticalCount warningCount minorCount infoCount totalValidations validations { type message field expected actual rule } } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       "testStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0) // Should be empty since no validation results exist
}

func TestGraphQL_ValidationResults_EmptyResults(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test with non-existent step
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       "nonexistentStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}

func TestGraphQL_ValidationResults_InvalidThreadId(t *testing.T) {
	user := setupTestUser(t)

	// Test with invalid UUID format
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       "invalid-uuid-format",
			"stepName":       "testStep",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}

func TestGraphQL_ValidationResults_EmptyStepName(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test with empty step name
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       "",
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}

func TestGraphQL_ValidationResults_EmptyIdempotencyKey(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test with empty idempotency key
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       "testStep",
			"idempotencyKey": "",
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}

func TestGraphQL_ValidationResults_LongStepName(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1)

	// Test with very long step name
	longStepName := "this_is_a_very_long_step_name_that_exceeds_normal_limits_and_should_be_handled_gracefully_by_the_system"

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($threadId: String!, $stepName: String!, $idempotencyKey: String!) { validationResults(threadId: $threadId, stepName: $stepName, idempotencyKey: $idempotencyKey) { validationId } }",
		Variables: map[string]interface{}{
			"threadId":       threadID,
			"stepName":       longStepName,
			"idempotencyKey": uuid.NewString(),
		},
	})

	require.Empty(t, gqlResp.Errors)
	results, ok := gqlResp.Data["validationResults"].([]interface{})
	require.True(t, ok)
	require.Len(t, results, 0)
}
