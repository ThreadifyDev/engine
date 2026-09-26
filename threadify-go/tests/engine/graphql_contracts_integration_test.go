package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type graphQLResponse struct {
	Data   map[string]interface{}   `json:"data"`
	Errors []map[string]interface{} `json:"errors"`
}

func doGraphQL(t *testing.T, token, apiKey string, reqBody graphQLRequest) (*enginetest.TestResponse, graphQLResponse) {
	t.Helper()

	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, httpc.BaseURL+"/graphql", bytes.NewReader(bodyBytes))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := httpc.Client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	tr := &enginetest.TestResponse{StatusCode: resp.StatusCode, Body: respBody}

	var gqlResp graphQLResponse
	_ = json.Unmarshal(respBody, &gqlResp)

	return tr, gqlResp
}

func createContractVersion(t *testing.T, token, contractName string, version int, terminalSteps []string) (contractID string) {
	t.Helper()

	source := fmt.Sprintf(`Feature: %s
Version: %d
Description: graphql test contract

Rule: First step
  When step "stepA" is submitted
  Then owner must be "actor1"
  And this step is an entry point
  And next step must be one of "stepB"

Rule: Second step
  When step "stepB" is submitted
  Then owner must be "actor1"
`, contractName, version)
	if version >= 2 {
		source += `  And next step must be one of "stepC"

Rule: Third step
  When step "stepC" is submitted
  Then owner must be "actor1"
`
	}
	for _, terminal := range terminalSteps {
		marker := fmt.Sprintf("  When step %q is submitted\n  Then owner must be \"actor1\"\n", terminal)
		source = strings.Replace(source, marker, marker+"  And this step is terminal\n", 1)
	}

	if version == 1 {
		resp := httpc.DoWithAuth(t, http.MethodPost, "/v1/contracts", []byte(source), "text/plain", token)
		require.Equal(t, http.StatusOK, resp.StatusCode, "body: %s", resp.Body)
		result := requireContractFromBody(t, resp)
		return result.Contract.ID
	}

	listResp := httpc.DoWithAuth(t, http.MethodGet, "/v1/contracts", nil, "", token)
	require.Equal(t, http.StatusOK, listResp.StatusCode, "body: %s", listResp.Body)
	list := requireContractList(t, listResp)
	var id string
	for _, c := range list {
		if c.Name == contractName {
			id = c.ID
			break
		}
	}
	require.NotEmpty(t, id)

	updateResp := httpc.DoWithAuth(t, http.MethodPut, "/v1/contracts/"+id, []byte(source), "text/plain", token)
	require.Equal(t, http.StatusOK, updateResp.StatusCode, "body: %s", updateResp.Body)
	return id
}

func TestGraphQL_ContractGraph_RequiresAuth(t *testing.T) {
	contractName := "gql_contract_" + uuid.NewString()[:8]

	resp := httpc.DoJSON(t, http.MethodPost, "/graphql", map[string]interface{}{
		"query":     "query($name: String!) { contractGraph(name: $name) { parties } }",
		"variables": map[string]interface{}{"name": contractName},
	})

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGraphQL_ContractGraph_LatestVersion_Default(t *testing.T) {
	user := setupTestUser(t)
	contractName := "gql_contract_" + uuid.NewString()[:8]

	_ = createContractVersion(t, user.Token, contractName, 1, []string{"stepB"})
	_ = createContractVersion(t, user.Token, contractName, 2, []string{"stepC"})

	_, gqlResp := doGraphQL(t, user.Token, "", graphQLRequest{
		Query: "query($name: String!) { contractGraph(name: $name) { graph { terminalSteps entryPoints nodes { id } } } }",
		Variables: map[string]interface{}{
			"name": contractName,
		},
	})

	require.Empty(t, gqlResp.Errors, "errors: %v", gqlResp.Errors)
	require.NotNil(t, gqlResp.Data)

	cg, ok := gqlResp.Data["contractGraph"].(map[string]interface{})
	require.True(t, ok)
	graph, ok := cg["graph"].(map[string]interface{})
	require.True(t, ok)
	terminalSteps, ok := graph["terminalSteps"].([]interface{})
	require.True(t, ok)
	require.GreaterOrEqual(t, len(terminalSteps), 1)
	require.Equal(t, "stepC", terminalSteps[0].(string))
}

func TestGraphQL_ContractGraph_SpecificVersion(t *testing.T) {
	user := setupTestUser(t)
	contractName := "gql_contract_" + uuid.NewString()[:8]

	_ = createContractVersion(t, user.Token, contractName, 1, []string{"stepB"})
	_ = createContractVersion(t, user.Token, contractName, 2, []string{"stepC"})

	_, gqlResp := doGraphQL(t, user.Token, "", graphQLRequest{
		Query: "query($name: String!, $version: Int) { contractGraph(name: $name, version: $version) { graph { terminalSteps } } }",
		Variables: map[string]interface{}{
			"name":    contractName,
			"version": 1,
		},
	})

	require.Empty(t, gqlResp.Errors, "errors: %v", gqlResp.Errors)
	cg, ok := gqlResp.Data["contractGraph"].(map[string]interface{})
	require.True(t, ok)
	graph, ok := cg["graph"].(map[string]interface{})
	require.True(t, ok)
	terminalSteps, ok := graph["terminalSteps"].([]interface{})
	require.True(t, ok)
	require.GreaterOrEqual(t, len(terminalSteps), 1)
	require.Equal(t, "stepB", terminalSteps[0].(string))
}

func TestGraphQL_ContractGraph_NotFound(t *testing.T) {
	user := setupTestUser(t)
	contractName := "gql_contract_missing_" + uuid.NewString()[:8]

	_, gqlResp := doGraphQL(t, user.Token, "", graphQLRequest{
		Query: "query($name: String!) { contractGraph(name: $name) { parties } }",
		Variables: map[string]interface{}{
			"name": contractName,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
}

func TestGraphQL_ContractGraph_CrossCompanyDenied(t *testing.T) {
	userA := setupTestUser(t)
	userB := setupTestUser(t)

	contractName := "gql_contract_" + uuid.NewString()[:8]
	_ = createContractVersion(t, userA.Token, contractName, 1, []string{"stepB"})

	_, gqlResp := doGraphQL(t, userB.Token, "", graphQLRequest{
		Query: "query($name: String!) { contractGraph(name: $name) { parties } }",
		Variables: map[string]interface{}{
			"name": contractName,
		},
	})

	require.NotEmpty(t, gqlResp.Errors)
}
