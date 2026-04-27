package engine

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_ThreadsByRef_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query($refValue: String!) { threadsByRef(refValue: $refValue, limit: 1) { totalCount } }",
		"variables": map[string]interface{}{"refValue": "test_value"},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_ThreadsByRef_ByKeyAndValue(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "order_id"
	refValue := "order_12345"

	// Create thread with reference
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			refKey: refValue,
		},
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!) { threadsByRef(refKey: $refKey, refValue: $refValue, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refKey":   refKey,
			"refValue": refValue,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
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
}

func TestGraphQL_ThreadsByRef_ByValueOnly(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "customer_id"
	refValue := "customer_67890"

	// Create thread with reference
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			refKey: refValue,
		},
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refValue: String!) { threadsByRef(refValue: $refValue, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refValue": refValue,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
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
}

func TestGraphQL_ThreadsByRef_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "order_id"
	refValue := "order_54321"

	_ = enginetest.CreateThread(t, env.Postgres.Pool, userA, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			refKey: refValue,
		},
	})

	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($refValue: String!) { threadsByRef(refValue: $refValue, limit: 10) { totalCount } }",
		Variables: map[string]interface{}{
			"refValue": refValue,
		},
	})
	require.Empty(t, respA.Errors)
	connA, ok := respA.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	countA, ok := connA["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(countA))

	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($refValue: String!) { threadsByRef(refValue: $refValue, limit: 10) { totalCount } }",
		Variables: map[string]interface{}{
			"refValue": refValue,
		},
	})
	require.Empty(t, respB.Errors)
	connB, ok := respB.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	countB, ok := connB["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(countB))
}

func TestGraphQL_ThreadsByRef_MultipleThreadsSameRef(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "batch_id"
	refValue := "batch_999"

	threadIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		threadIDs[i] = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
			Refs: map[string]interface{}{
				refKey: refValue,
			},
		})
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!) { threadsByRef(refKey: $refKey, refValue: $refValue, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refKey":   refKey,
			"refValue": refValue,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 3, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 3)
}

func TestGraphQL_ThreadsByRef_MultipleRefsSameThread(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]

	// Create thread with multiple references
	refs := map[string]interface{}{
		"order_id":    "order_111",
		"customer_id": "customer_222",
		"product_id":  "product_333",
	}
	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: refs,
	})

	testCases := []struct {
		refKey   string
		refValue string
	}{
		{"order_id", "order_111"},
		{"customer_id", "customer_222"},
		{"product_id", "product_333"},
	}

	for _, tc := range testCases {
		_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
			Query: "query($refKey: String, $refValue: String!) { threadsByRef(refKey: $refKey, refValue: $refValue, limit: 10) { totalCount threads { id } } }",
			Variables: map[string]interface{}{
				"refKey":   tc.refKey,
				"refValue": tc.refValue,
			},
		})

		require.Empty(t, gqlResp.Errors)
		conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
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
	}
}

func TestGraphQL_ThreadsByRef_WithStatusFilter(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "order_id"
	refValue := "order_777"
	status := "completed"

	threadID := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			refKey: refValue,
		},
		Status: status,
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!, $status: String) { threadsByRef(refKey: $refKey, refValue: $refValue, status: $status, limit: 10) { totalCount threads { id status } } }",
		Variables: map[string]interface{}{
			"refKey":   refKey,
			"refValue": refValue,
			"status":   status,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
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

func TestGraphQL_ThreadsByRef_WithDateFilters(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "order_id"
	refValue := "order_666"

	startedAt := time.Now().Add(-1 * time.Hour)
	_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			refKey: refValue,
		},
		StartedAt: &startedAt,
	})

	twoHoursAgo := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)
	now := time.Now().Format(time.RFC3339)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!, $startedAfter: String, $startedBefore: String) { threadsByRef(refKey: $refKey, refValue: $refValue, startedAfter: $startedAfter, startedBefore: $startedBefore, limit: 10) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refKey":        refKey,
			"refValue":      refValue,
			"startedAfter":  twoHoursAgo,
			"startedBefore": now,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 1, int(count))
}

func TestGraphQL_ThreadsByRef_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refValue: String!) { threadsByRef(refValue: $refValue, limit: 10) { totalCount } }",
		Variables: map[string]interface{}{
			"refValue": "nonexistent_value",
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(count))
}

func TestGraphQL_ThreadsByRef_Pagination(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "batch_id"
	refValue := "batch_pagination_test"

	for i := 0; i < 5; i++ {
		_ = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
			Refs: map[string]interface{}{
				refKey: refValue,
			},
		})
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!, $limit: Int) { threadsByRef(refKey: $refKey, refValue: $refValue, limit: $limit) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refKey":   refKey,
			"refValue": refValue,
			"limit":    3,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 5, int(count)) // Total count should be 5

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 3) // But only 3 returned due to limit
}

func TestGraphQL_ThreadsByRef_Offset(t *testing.T) {
	user := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	refKey := "batch_id"
	refValue := "batch_offset_test"

	threadIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		threadIDs[i] = enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
			Refs: map[string]interface{}{
				refKey: refValue,
			},
		})
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String, $refValue: String!, $offset: Int, $limit: Int) { threadsByRef(refKey: $refKey, refValue: $refValue, offset: $offset, limit: $limit) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"refKey":   refKey,
			"refValue": refValue,
			"offset":   2,
			"limit":    2,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 5, int(count))

	threads, ok := conn["threads"].([]interface{})
	require.True(t, ok)
	require.Len(t, threads, 2)
}

func TestGraphQL_ThreadsByRef_NegativeOffset_ClampedToZero(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refValue: String!, $offset: Int) { threadsByRef(refValue: $refValue, offset: $offset, limit: 10) { totalCount } }",
		Variables: map[string]interface{}{
			"refValue": "test_value",
			"offset":   -10,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["threadsByRef"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 0, int(count))
}
