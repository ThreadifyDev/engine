package engine

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/dbhelpers"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestGraphQL_ComputedMetrics_FullFlow(t *testing.T) {
	user := setupTestUser(t)

	profileTypeName := "Test Computed Metrics Type"
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, profileTypeName, []string{"cust_id"})

	sqlContent := `
		SELECT COUNT(*) as total_threads
		FROM thread_refs
		WHERE ref_key = 'cust_id' AND ref_value = @ref_value
	`
	templateID := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Thread Count", sqlContent)

	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID, map[string]interface{}{})

	refValue := "cust_ABC"
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Customer ABC", refValue)

	enginetest.CreateThread(t, env.Postgres.Pool, user, "contract_1", 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{"cust_id": refValue},
	})
	enginetest.CreateThread(t, env.Postgres.Pool, user, "contract_1", 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{"cust_id": refValue},
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					id
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{
			"id": profileID,
		},
	})

	require.Empty(t, gqlResp.Errors, "computedMetrics query should not return errors")
	profile := gqlResp.Data["entityProfile"].(map[string]interface{})

	rawMetrics := profile["computedMetrics"]
	require.NotNil(t, rawMetrics, "computedMetrics should not be nil when metrics templates are bound")

	metricsJSON, _ := json.MarshalIndent(rawMetrics, "", "  ")
	t.Logf("computedMetrics response: %s", string(metricsJSON))

	computedMetrics, ok := rawMetrics.(map[string]interface{})
	require.True(t, ok, "computedMetrics should be a JSON object (map), got %T", rawMetrics)
	require.NotEmpty(t, computedMetrics, "computedMetrics should not be empty")

	val, exists := computedMetrics["Thread Count: Total Threads"]
	require.True(t, exists, "expected key 'Thread Count: Total Threads' in computedMetrics, got keys: %v", keysOf(computedMetrics))

	totalThreads, ok := val.(float64)
	require.True(t, ok, "expected float64 for total_threads, got %T", val)
	require.Equal(t, 2, int(totalThreads))

	_, gqlResp2 := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{
			"id": profileID,
		},
	})
	require.Empty(t, gqlResp2.Errors)
	profile2 := gqlResp2.Data["entityProfile"].(map[string]interface{})
	require.NotNil(t, profile2["computedMetrics"])

	computedMetrics2, ok := profile2["computedMetrics"].(map[string]interface{})
	require.True(t, ok, "cached computedMetrics should also be a map")

	val2, exists2 := computedMetrics2["Thread Count: Total Threads"]
	require.True(t, exists2, "cached result should have same key")
	require.Equal(t, totalThreads, val2.(float64))
}

func TestGraphQL_ComputedMetrics_NoMetricsConfigured(t *testing.T) {
	user := setupTestUser(t)
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "No Metrics Type", []string{"nid"})
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "No Metrics User", "nid_123")

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{
			"id": profileID,
		},
	})

	require.Empty(t, gqlResp.Errors)
	profile := gqlResp.Data["entityProfile"].(map[string]interface{})
	require.Nil(t, profile["computedMetrics"])
}

func TestGraphQL_ComputedMetrics_InvalidRange(t *testing.T) {
	user := setupTestUser(t)
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "Range Test", []string{"rid"})

	templateID := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Dummy", "SELECT 1 as val")
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID, map[string]interface{}{})

	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Range User", "rid_123")

	// Invalid range should produce an error
	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "invalid")
				}
			}
		`,
		Variables: map[string]interface{}{
			"id": profileID,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
	require.Contains(t, gqlResp.Errors[0]["message"], "unsupported range format")
}

func TestGraphQL_ComputedMetrics_MultipleTemplates(t *testing.T) {
	user := setupTestUser(t)
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "Multi Metrics Type", []string{"multi_id"})

	// Template 1
	sqlContent1 := `SELECT COUNT(*) as count_one FROM thread_refs WHERE ref_key = 'multi_id' AND ref_value = @ref_value`
	templateID1 := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Metric One", sqlContent1)
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID1, map[string]interface{}{"p1": "v1"})

	// Template 2
	sqlContent2 := `SELECT COUNT(*) as count_two FROM thread_refs WHERE ref_key = 'multi_id' AND ref_value = @ref_value`
	templateID2 := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Metric Two", sqlContent2)
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID2, map[string]interface{}{})

	refValue := "multi_123"
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Multi User", refValue)

	// Create 1 thread to give count 1
	enginetest.CreateThread(t, env.Postgres.Pool, user, "contract_1", 1, dbhelpers.ThreadOption{
		Refs: map[string]interface{}{"multi_id": refValue},
	})

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{"id": profileID},
	})

	require.Empty(t, gqlResp.Errors)
	computedMetrics, ok := gqlResp.Data["entityProfile"].(map[string]interface{})["computedMetrics"].(map[string]interface{})
	require.True(t, ok)

	// Check keys are properly formatted with names and params suffix
	require.Contains(t, computedMetrics, "Metric One: Count One (p1: v1)")
	require.Contains(t, computedMetrics, "Metric Two: Count Two")
	require.Equal(t, float64(1), computedMetrics["Metric One: Count One (p1: v1)"])
	require.Equal(t, float64(1), computedMetrics["Metric Two: Count Two"])
}

func TestGraphQL_ComputedMetrics_CompanyIsolation(t *testing.T) {
	userA := setupTestUser(t) // Company A
	userB := setupTestUser(t) // Company B

	// Profile Type belonging to Company A
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, userA.CompanyID, "Company A Type", []string{"iso_id"})
	templateID := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Dummy", "SELECT 1 as val")
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID, map[string]interface{}{})

	// Try querying the metrics for a profile type that belongs to company A using user B's profile
	// Simulate user B having a profile but trying to use company A's type
	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, userB.CompanyID, profileTypeID, "Company B Profile", "iso_B")

	_, gqlResp := doGraphQL(t, "", userB.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{"id": profileID},
	})

	require.Empty(t, gqlResp.Errors)
	// Because of company isolation, the profile type shouldn't be matched or metrics shouldn't be evaluated
	require.Nil(t, gqlResp.Data["entityProfile"].(map[string]interface{})["computedMetrics"])
}

func TestGraphQL_ComputedMetrics_ZeroActivity(t *testing.T) {
	user := setupTestUser(t)
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "Zero Type", []string{"zid"})

	// A query that returns zero rows if there are no threads
	sqlContent := `
		SELECT COUNT(*) as activity_count
		FROM thread_refs
		WHERE ref_key = 'zid' AND ref_value = @ref_value
		HAVING COUNT(*) > 0
	`
	templateID := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "Activity", sqlContent)
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID, map[string]interface{}{})

	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "Zero User", "zid_000")

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{"id": profileID},
	})

	require.Empty(t, gqlResp.Errors)

	// Expect computedMetrics to be null when NO queries returned results
	require.Nil(t, gqlResp.Data["entityProfile"].(map[string]interface{})["computedMetrics"])
}

func TestGraphQL_ComputedMetrics_MultiRowResults(t *testing.T) {
	user := setupTestUser(t)
	profileTypeID := enginetest.CreateEntityProfileType(t, env.Postgres.Pool, user.CompanyID, "MultiRow Type", []string{"mrid"})

	// A dummy query that returns 2 rows
	sqlContent := `
		SELECT 'foo' as type, 1 as val
		UNION ALL
		SELECT 'bar' as type, 2 as val
	`
	templateID := enginetest.CreateMetricsTemplate(t, env.Postgres.Pool, "MultiRow Metric", sqlContent)
	enginetest.BindMetricsTemplate(t, env.Postgres.Pool, profileTypeID, templateID, map[string]interface{}{})

	profileID := enginetest.CreateEntityProfile(t, env.Postgres.Pool, user.CompanyID, profileTypeID, "MultiRow User", "mrid_123")

	_, gqlResp := doGraphQL(t, "", user.ApiKey, graphQLRequest{
		Query: `
			query($id: String!) {
				entityProfile(id: $id) {
					computedMetrics(range: "30d")
				}
			}
		`,
		Variables: map[string]interface{}{"id": profileID},
	})

	require.Empty(t, gqlResp.Errors)

	computedMetrics, ok := gqlResp.Data["entityProfile"].(map[string]interface{})["computedMetrics"].(map[string]interface{})
	require.True(t, ok)

	// We expect the result to be under the metric name directly as a JSON array
	val, exists := computedMetrics["MultiRow Metric"]
	require.True(t, exists, "Expected MultiRow Metric key in response")

	list, ok := val.([]interface{})
	require.True(t, ok, "Expected value to be an array")
	require.Equal(t, 2, len(list))

	firstRow := list[0].(map[string]interface{})
	require.Equal(t, "foo", firstRow["type"])
	require.Equal(t, float64(1), firstRow["val"])
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
