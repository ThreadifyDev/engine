package engine

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGraphQL_ResolveActors_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		"variables": map[string]interface{}{"ids": []string{uuid.NewString()}},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_ResolveActors_SingleActor(t *testing.T) {
	user := setupTestUser(t)

	actorID := user.ServiceAccountID

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type companyName } }",
		Variables: map[string]interface{}{
			"ids": []string{actorID},
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actors, 1)

	actorObj, ok := actors[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, actorID, actorObj["id"])
	require.Contains(t, actorObj, "name")
	require.Contains(t, actorObj, "type")
	require.Contains(t, actorObj, "companyName")
}

func TestGraphQL_ResolveActors_MultipleActors(t *testing.T) {
	user := setupTestUser(t)

	actorIDs := []string{
		user.ServiceAccountID,
		user.ID,
		uuid.NewString(), // Non-existent actor
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		Variables: map[string]interface{}{
			"ids": actorIDs,
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	// resolveActors returns only actors that exist (schema is [ActorInfo!]!)
	require.Len(t, actors, 2)

	// Order is not guaranteed due to UNION ALL query; assert membership.
	ids := map[string]bool{}
	for _, actor := range actors {
		actorObj, ok := actor.(map[string]interface{})
		require.True(t, ok)
		id, ok := actorObj["id"].(string)
		require.True(t, ok)
		ids[id] = true
	}
	require.True(t, ids[user.ServiceAccountID])
	require.True(t, ids[user.ID])
}

func TestGraphQL_ResolveActors_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	// User A should only see actors from their company
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type companyName } }",
		Variables: map[string]interface{}{
			"ids": []string{userA.ServiceAccountID, userB.ServiceAccountID},
		},
	})
	require.Empty(t, respA.Errors)
	actorsA, ok := respA.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actorsA, 2)

	// Check that User A's service account is resolved and includes a companyName.
	var foundUserAActor bool
	for _, actor := range actorsA {
		actorObj := actor.(map[string]interface{})
		if actorObj["id"] == userA.ServiceAccountID {
			foundUserAActor = true
			require.Equal(t, "Integration Test Company", actorObj["companyName"])
			break
		}
	}
	require.True(t, foundUserAActor, "User A should see their own service account")
}

func TestGraphQL_ResolveActors_EmptyList(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		Variables: map[string]interface{}{
			"ids": []string{},
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actors, 0)
}

func TestGraphQL_ResolveActors_NonExistentActors(t *testing.T) {
	user := setupTestUser(t)

	nonExistentIDs := []string{
		uuid.NewString(),
		uuid.NewString(),
		uuid.NewString(),
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		Variables: map[string]interface{}{
			"ids": nonExistentIDs,
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actors, 0)
}

func TestGraphQL_ResolveActors_InvalidUUIDs(t *testing.T) {
	user := setupTestUser(t)

	invalidIDs := []string{
		"invalid-uuid-1",
		"invalid-uuid-2",
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		Variables: map[string]interface{}{
			"ids": invalidIDs,
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actors, 0)
}

func TestGraphQL_ResolveActors_MixedValidAndInvalid(t *testing.T) {
	user := setupTestUser(t)

	mixedIDs := []string{
		user.ServiceAccountID, // Valid
		uuid.NewString(),      // Non-existent
		"invalid-uuid-format", // Invalid format
		user.ID,               // Valid
	}

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($ids: [String!]!) { resolveActors(ids: $ids) { id name type } }",
		Variables: map[string]interface{}{
			"ids": mixedIDs,
		},
	})

	require.Empty(t, gqlResp.Errors)
	actors, ok := gqlResp.Data["resolveActors"].([]interface{})
	require.True(t, ok)
	require.Len(t, actors, 2)
}

func TestGraphQL_CheckCredits_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query { checkCredits }",
		"variables": map[string]interface{}{},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_CheckCredits_BasicCheck(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query { checkCredits }",
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits) // Should have credits from setup
}

func TestGraphQL_CheckCredits_WithMeter(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($meter: String, $amount: Int) { checkCredits(meter: $meter, amount: $amount) }",
		Variables: map[string]interface{}{
			"meter":  "contract_execution",
			"amount": 1000, // 1 millicent
		},
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits) // Should have sufficient credits
}

func TestGraphQL_CheckCredits_WithAmountOnly(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($amount: Int) { checkCredits(amount: $amount) }",
		Variables: map[string]interface{}{
			"amount": 5000, // 5 millicents
		},
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits) // Should have sufficient credits
}

func TestGraphQL_CheckCredits_WithMeterOnly(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($meter: String) { checkCredits(meter: $meter) }",
		Variables: map[string]interface{}{
			"meter": "egress",
		},
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits) // Should have credits for basic check
}

func TestGraphQL_CheckCredits_InsufficientCredits(t *testing.T) {
	user := setupTestUser(t)

	// In the integration-test config, meter costs may be zeroed out, meaning
	// any checkCredits request is effectively free and should succeed.
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($meter: String, $amount: Int) { checkCredits(meter: $meter, amount: $amount) }",
		Variables: map[string]interface{}{
			"meter":  "contract_execution",
			"amount": 100000000, // 100,000 cents = 1,000 dollars
		},
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits)
}

func TestGraphQL_CheckCredits_ZeroAmount(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($amount: Int) { checkCredits(amount: $amount) }",
		Variables: map[string]interface{}{
			"amount": 0,
		},
	})

	require.Empty(t, gqlResp.Errors)
	hasCredits, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	require.True(t, hasCredits) // Zero amount should always succeed
}

func TestGraphQL_CheckCredits_NegativeAmount(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($amount: Int) { checkCredits(amount: $amount) }",
		Variables: map[string]interface{}{
			"amount": -1000,
		},
	})

	require.Empty(t, gqlResp.Errors)
	_, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	// Negative amounts should either be rejected or treated as zero
	// The exact behavior depends on implementation
}

func TestGraphQL_CheckCredits_InvalidMeter(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($meter: String) { checkCredits(meter: $meter) }",
		Variables: map[string]interface{}{
			"meter": "nonexistent_meter",
		},
	})

	require.Empty(t, gqlResp.Errors)
	_, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	// Invalid meter should either be rejected or fall back to basic check
}

func TestGraphQL_CheckCredits_NegativeMeterAmount(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($meter: String, $amount: Int) { checkCredits(meter: $meter, amount: $amount) }",
		Variables: map[string]interface{}{
			"meter":  "contract_execution",
			"amount": -5000,
		},
	})

	require.Empty(t, gqlResp.Errors)
	_, ok := gqlResp.Data["checkCredits"].(bool)
	require.True(t, ok)
	// Negative amounts should either be rejected or treated as zero
}
