package engine

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_EntityProfileHistory_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query": "query($profileID: ID!) { entityProfileHistory(profileID: $profileID) { totalCount } }",
		"variables": map[string]interface{}{
			"profileID": uuid.NewString(),
		},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_EntityProfileHistory_Success(t *testing.T) {
	user := setupTestUser(t)

	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "Customer", []string{"customer_id"})

	refValue := "cust_12345"
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Test Customer", refValue)

	contractName := "history_contract_" + uuid.NewString()[:8]
	refs := map[string]interface{}{
		"customer_id": refValue,
		"other_ref":   "random",
	}

	threadID1 := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: refs,
	})
	threadID2 := enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: refs,
	})

	enginetest.CreateThread(t, env.Postgres.Pool, user, contractName, 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			"customer_id": "different_cust",
		},
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($profileID: ID!) { entityProfileHistory(profileID: $profileID) { totalCount threads { id } } }",
		Variables: map[string]interface{}{
			"profileID": profileID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	conn, ok := gqlResp.Data["entityProfileHistory"].(map[string]interface{})
	require.True(t, ok)

	totalCount := int(conn["totalCount"].(float64))
	require.Equal(t, 2, totalCount)

	threads := conn["threads"].([]interface{})
	require.Len(t, threads, 2)

	found1 := false
	found2 := false
	for _, it := range threads {
		tObj := it.(map[string]interface{})
		id := tObj["id"].(string)
		if id == threadID1 {
			found1 = true
		}
		if id == threadID2 {
			found2 = true
		}
	}
	require.True(t, found1)
	require.True(t, found2)
}

func TestGraphQL_EntityProfileHistory_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, userA.CompanyID, "User A Type", []string{"uid"})
	refValue := "uid_999"
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, userA.CompanyID, profileTypeID, "User A Profile", refValue)

	enginetest.CreateThread(t, env.Postgres.Pool, userA, "contractA", 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{
			"uid": refValue,
		},
	})

	_, gqlResp := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($profileID: ID!) { entityProfileHistory(profileID: $profileID) { totalCount } }",
		Variables: map[string]interface{}{
			"profileID": profileID,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
	require.Contains(t, gqlResp.Errors[0]["message"], "failed to get entity profile")
}
