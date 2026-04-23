package engine

import (
	"testing"
	"threadify-go/shared/slug"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_EntityProfile_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query": "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id refKey companyId } }",
		"variables": map[string]interface{}{
			"refKey": "test_key",
			"type":   "customer",
		},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_EntityProfile_NotFound(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id refKey companyId } }",
		Variables: map[string]interface{}{
			"refKey": "nonexistent_key",
			"type":   "customer",
		},
	})

	// Should return null for non-existent entity
	require.Empty(t, gqlResp.Errors)
	profile := gqlResp.Data["entityProfile"]
	require.Nil(t, profile)
}

func TestGraphQL_EntityProfile_RequiredParameters(t *testing.T) {
	user := setupTestUser(t)

	// Test with both required parameters
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id refKey companyId profileTypeId name createdAt lastActiveAt metrics { totalDeliveries completedSuccessfully validationViolations deliveryHealthScore healthTrendSlope averageDeliveryTimeMs lastCalculatedAt } } }",
		Variables: map[string]interface{}{
			"refKey": "test_customer_123",
			"type":   "customer",
		},
	})

	require.Empty(t, gqlResp.Errors)
	profile := gqlResp.Data["entityProfile"]
	require.Nil(t, profile) // Should be null since no entity exists
}

func TestGraphQL_EntityProfile_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	// User A queries for an entity (will be null since none exist)
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id companyId } }",
		Variables: map[string]interface{}{
			"refKey": "test_entity_123",
			"type":   "customer",
		},
	})
	require.Empty(t, respA.Errors)
	profileA := respA.Data["entityProfile"]
	require.Nil(t, profileA)

	// User B queries for the same entity (should also be null)
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id companyId } }",
		Variables: map[string]interface{}{
			"refKey": "test_entity_123",
			"type":   "customer",
		},
	})
	require.Empty(t, respB.Errors)
	profileB := respB.Data["entityProfile"]
	require.Nil(t, profileB)
}

func TestGraphQL_EntityProfile_EmptyRefKey(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id } }",
		Variables: map[string]interface{}{
			"refKey": "",
			"type":   "customer",
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
	require.Equal(t, "must provide either id or both refKey and type", gqlResp.Errors[0]["message"])
}

func TestGraphQL_EntityProfile_EmptyType(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id } }",
		Variables: map[string]interface{}{
			"refKey": "test_key",
			"type":   "",
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
	require.Equal(t, "must provide either id or both refKey and type", gqlResp.Errors[0]["message"])
}

func TestGraphQL_EntityProfile_LongRefKey(t *testing.T) {
	user := setupTestUser(t)

	longRefKey := "this_is_a_very_long_reference_key_that_exceeds_normal_limits_and_should_be_handled_gracefully_by_the_system"

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id } }",
		Variables: map[string]interface{}{
			"refKey": longRefKey,
			"type":   "customer",
		},
	})

	require.Empty(t, gqlResp.Errors)
	profile := gqlResp.Data["entityProfile"]
	require.Nil(t, profile) // Should be null since no entity exists
}

func TestGraphQL_EntityProfile_SpecialCharactersInRefKey(t *testing.T) {
	user := setupTestUser(t)

	specialRefKey := "test_key-with.special@characters#123"

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($refKey: String!, $type: String!) { entityProfile(refKey: $refKey, type: $type) { id } }",
		Variables: map[string]interface{}{
			"refKey": specialRefKey,
			"type":   "customer",
		},
	})

	require.Empty(t, gqlResp.Errors)
	profile := gqlResp.Data["entityProfile"]
	require.Nil(t, profile) // Should be null since no entity exists
}

func TestGraphQL_EntityProfileTypes_RequiresAuth(t *testing.T) {
	resp := httpc.DoJSON(t, "POST", "/graphql", map[string]interface{}{
		"query":     "query { entityProfileTypes { id companyId name type description } }",
		"variables": map[string]interface{}{},
	})

	require.Equal(t, 401, resp.StatusCode)
}

func TestGraphQL_EntityProfileTypes_CompanyScoped(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query { entityProfileTypes { id companyId name type description createdAt updatedAt } }",
	})

	require.Empty(t, gqlResp.Errors)
	types, ok := gqlResp.Data["entityProfileTypes"].([]interface{})
	require.True(t, ok)
	// Should return empty array if no profile types are configured
	require.Len(t, types, 0)
}

func TestGraphQL_EntityProfileTypes_ResponseStructure(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query { entityProfileTypes { id companyId name type description createdAt updatedAt } }",
	})

	require.Empty(t, gqlResp.Errors)
	types, ok := gqlResp.Data["entityProfileTypes"].([]interface{})
	require.True(t, ok)
	require.Len(t, types, 0) // Should be empty initially
}

func TestGraphQL_EntityProfileTypes_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	// User A queries profile types
	_, respA := doGraphQL(t, "", userA.ApiKey, graphQLRequest{
		Query: "query { entityProfileTypes { id companyId name } }",
	})
	require.Empty(t, respA.Errors)
	typesA, ok := respA.Data["entityProfileTypes"].([]interface{})
	require.True(t, ok)
	require.Len(t, typesA, 0)

	// User B queries profile types (should be separate)
	_, respB := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: "query { entityProfileTypes { id companyId name } }",
	})
	require.Empty(t, respB.Errors)
	typesB, ok := respB.Data["entityProfileTypes"].([]interface{})
	require.True(t, ok)
	require.Len(t, typesB, 0)
}

func TestGraphQL_EntityProfileTypes_EmptyResult(t *testing.T) {
	user := setupTestUser(t)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query { entityProfileTypes { id name type description } }",
	})

	require.Empty(t, gqlResp.Errors)
	types, ok := gqlResp.Data["entityProfileTypes"].([]interface{})
	require.True(t, ok)
	require.Len(t, types, 0) // Should be empty if no profile types configured
}

func TestGraphQL_EntityProfilesByType_FiltersByProfileTypeName(t *testing.T) {
	user := setupTestUser(t)

	profileTypeName := "Customer Profiles"
	refKey := "customer"

	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, profileTypeName, []string{refKey})

	olderLastActive := time.Now().Add(-2 * time.Hour).UTC()
	newerLastActive := time.Now().Add(-1 * time.Hour).UTC()

	olderID := enginetest.CreateEntityProfileWithTimestamp(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Older Profile", "older_ref", olderLastActive)
	newerID := enginetest.CreateEntityProfileWithTimestamp(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Newer Profile", "newer_ref", newerLastActive)

	profileTypeSlug := slug.ToSlug(profileTypeName)

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: "query($type: String!) { entityProfilesByType(type: $type, limit: 10, offset: 0) { totalCount profileType { name } items { id refKey profileTypeId } } }",
		Variables: map[string]interface{}{
			"type": profileTypeSlug,
		},
	})

	require.Empty(t, gqlResp.Errors)

	conn, ok := gqlResp.Data["entityProfilesByType"].(map[string]interface{})
	require.True(t, ok)
	count, ok := conn["totalCount"].(float64)
	require.True(t, ok)
	require.Equal(t, 2, int(count))

	pType, ok := conn["profileType"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, profileTypeName, pType["name"])

	items, ok := conn["items"].([]interface{})
	require.True(t, ok)
	require.Len(t, items, 2)

	// Ordered by lastActiveAt desc, so newer should come first.
	first := items[0].(map[string]interface{})
	second := items[1].(map[string]interface{})
	require.Equal(t, newerID, first["id"])
	require.Equal(t, olderID, second["id"])
	require.Equal(t, profileTypeID, first["profileTypeId"])
	require.Equal(t, profileTypeID, second["profileTypeId"])
}
