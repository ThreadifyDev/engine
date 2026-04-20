package engine

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testContract struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	LatestVersion int    `json:"latestVersion"`
	CompanyID     string `json:"companyId"`
	OwnerID       string `json:"ownerId"`
	IsPublic      bool   `json:"isPublic"`
	IsDeleted     bool   `json:"isDeleted"`
}

type testContractVersion struct {
	ID          string `json:"id"`
	Version     int    `json:"version"`
	ContractID  string `json:"contractId"`
	Content     string `json:"content"` // add this
	YAMLContent string `json:"yamlContent"`
}

type testContractResponse struct {
	Contract        testContract        `json:"contract"`
	ContractVersion testContractVersion `json:"contractVersion"`
}

const (
	initialYAML = `
contract_name: integration_test_engine
version: 1
description: Integration test contract
parties:
  - merchant
  - payment_processor
steps:
  - id: order_placed
    owner: merchant
    type: managed
    timeout: 5m
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - notes
  - id: payment_validation
    owner: payment_processor
    type: external
    timeout: 30s
    business_context:
      required:
        - payment_method
        - amount
  - id: payment_validated
    owner: payment_processor
    type: managed
  - id: order_cancelled
    owner: merchant
    type: managed
    business_context:
      required:
        - cancellation_reason
transitions:
  - from: order_placed
    to:
      - payment_validation
    timeout: 2m
    max_retries: 3
  - from: payment_validation
    to:
      - payment_validated
      - order_cancelled
    timeout: 30s
    max_retries: 3
entry_points:
  - order_placed
terminal_steps:
  - payment_validated
  - order_cancelled
validation:
  max_duration: 1h
  allow_multiple_terminals: true
  multiple_terminals_severity: minor
versioning:
  threads_lock_to_version: true
`

	updatedYAML = `
contract_name: integration_test_engine
version: 2
description: Updated integration test contract
parties:
  - merchant
  - payment_processor
  - logistics_carrier
steps:
  - id: order_placed
    owner: merchant
    type: managed
    timeout: 5m
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - notes
  - id: payment_validation
    owner: payment_processor
    type: external
    timeout: 30s
    business_context:
      required:
        - payment_method
        - amount
  - id: payment_validated
    owner: payment_processor
    type: managed
  - id: order_cancelled
    owner: merchant
    type: managed
    business_context:
      required:
        - cancellation_reason
  - id: shipped
    owner: logistics_carrier
    type: managed
    timeout: 24h
    business_context:
      required:
        - tracking_number
        - carrier_name
      optional:
        - estimated_delivery
  - id: delivered
    owner: logistics_carrier
    type: managed
    business_context:
      required:
        - delivery_timestamp
transitions:
  - from: order_placed
    to:
      - payment_validation
    timeout: 2m
    max_retries: 3
  - from: payment_validation
    to:
      - payment_validated
      - order_cancelled
    timeout: 30s
    max_retries: 3
  - from: payment_validated
    to:
      - shipped
    timeout: 5m
  - from: shipped
    to:
      - delivered
    timeout: 72h
entry_points:
  - order_placed
terminal_steps:
  - delivered
  - order_cancelled
validation:
  max_duration: 72h
  allow_multiple_terminals: true
  multiple_terminals_severity: major
versioning:
  threads_lock_to_version: true
`

	invalidYAML = `
contract_name: broken_contract
version: 1
    description:
    - broken
`
)

func TestContracts_Engine_Lifecycle(t *testing.T) {
	user := setupTestUser(t)
	t.Logf("test user: ID=%s CompanyID=%s", user.ID, user.CompanyID)

	t.Run("preview_valid", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPost, "/v1/contracts/preview",
			[]byte(initialYAML), "text/plain", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"preview body: %s", resp.Body)

		body := decodeJSONBody(t, resp)

		valid, ok := body["valid"].(bool)
		require.True(t, ok, "'valid' field must be a bool; body: %s", resp.Body)
		assert.True(t, valid, "contract should be valid; errors: %v", body["errors"])
	})

	t.Run("preview_invalid_yaml", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPost, "/v1/contracts/preview",
			[]byte(invalidYAML), "text/plain", user.Token)

		if resp.StatusCode == http.StatusOK {
			body := decodeJSONBody(t, resp)
			valid, _ := body["valid"].(bool)
			assert.False(t, valid,
				"invalid YAML should have valid=false; body: %s", resp.Body)
		} else {
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"invalid YAML must be rejected; body: %s", resp.Body)
		}
	})

	var contractID string

	t.Run("create", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPost, "/v1/contracts",
			[]byte(initialYAML), "text/plain", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"create body: %s", resp.Body)

		result := requireContractFromBody(t, resp)
		contractID = result.Contract.ID

		assert.Equal(t, "integration_test_engine", result.Contract.Name)
		assert.Equal(t, "Integration test contract", result.Contract.Description)
		assert.Equal(t, 1, result.Contract.LatestVersion)
		assert.Equal(t, 1, result.ContractVersion.Version)
		assert.Equal(t, contractID, result.ContractVersion.ContractID)

		content := requireParsedContent(t, result.ContractVersion.Content)
		assert.ElementsMatch(t, []string{"merchant", "payment_processor"}, content.Parties)
		assert.ElementsMatch(t, []string{"order_placed"}, content.EntryPoints)
		assert.ElementsMatch(t, []string{"payment_validated", "order_cancelled"}, content.TerminalSteps)
		assert.Len(t, content.Steps, 4)
		assert.Len(t, content.Transitions, 2)

		// Spot-check a specific step's business context.
		orderPlaced := findStep(content.Steps, "order_placed")
		require.NotNil(t, orderPlaced, "step 'order_placed' must exist")
		assert.Equal(t, "merchant", orderPlaced.Owner)
		assert.Equal(t, "managed", orderPlaced.Type)
		assert.Equal(t, "5m", orderPlaced.Timeout)
		assert.ElementsMatch(t, []string{"order_id", "customer_id"}, orderPlaced.BusinessContext.Required)
		assert.ElementsMatch(t, []string{"notes"}, orderPlaced.BusinessContext.Optional)

		// Spot-check a transition.
		transition := findTransition(content.Transitions, "order_placed")
		require.NotNil(t, transition, "transition from 'order_placed' must exist")
		assert.ElementsMatch(t, []string{"payment_validation"}, transition.To)
		assert.Equal(t, 3, transition.MaxRetries)
	})

	t.Run("read", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodGet, "/v1/contracts/"+contractID,
			nil, "", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := requireContractFromBody(t, resp)
		assert.Equal(t, contractID, result.Contract.ID)
		assert.Equal(t, "integration_test_engine", result.Contract.Name)
		assert.Equal(t, 1, result.Contract.LatestVersion)

		content := requireParsedContent(t, result.ContractVersion.Content)
		assert.Len(t, content.Steps, 4)
		assert.Len(t, content.Transitions, 2)
		assert.ElementsMatch(t, []string{"merchant", "payment_processor"}, content.Parties)
	})

	t.Run("update", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPut, "/v1/contracts/"+contractID,
			[]byte(updatedYAML), "text/plain", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"update body: %s", resp.Body)

		resp = doWithAuth(t, http.MethodGet, "/v1/contracts/"+contractID,
			nil, "", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := requireContractFromBody(t, resp)
		assert.Equal(t, 2, result.Contract.LatestVersion)
		assert.Equal(t, "Updated integration test contract", result.Contract.Description)

		// Version 2 adds logistics_carrier and two new steps.
		content := requireParsedContent(t, result.ContractVersion.Content)
		assert.ElementsMatch(t,
			[]string{"merchant", "payment_processor", "logistics_carrier"},
			content.Parties,
			"version 2 should add logistics_carrier party")
		assert.Len(t, content.Steps, 6,
			"version 2 should have 6 steps")
		assert.Len(t, content.Transitions, 4,
			"version 2 should have 4 transitions")
		assert.ElementsMatch(t, []string{"delivered", "order_cancelled"}, content.TerminalSteps)

		shipped := findStep(content.Steps, "shipped")
		require.NotNil(t, shipped, "step 'shipped' must exist in version 2")
		assert.Equal(t, "logistics_carrier", shipped.Owner)
		assert.ElementsMatch(t, []string{"tracking_number", "carrier_name"}, shipped.BusinessContext.Required)
		assert.ElementsMatch(t, []string{"estimated_delivery"}, shipped.BusinessContext.Optional)
	})

	t.Run("delete", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodDelete, "/v1/contracts/"+contractID,
			nil, "", user.Token)

		resp = doWithAuth(t, http.MethodGet, "/v1/contracts/"+contractID,
			nil, "", user.Token)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode,
			"deleted contract must return 404")

		listResp := doWithAuth(t, http.MethodGet, "/v1/contracts",
			nil, "", user.Token)
		require.Equal(t, http.StatusOK, listResp.StatusCode)
		list := requireContractList(t, listResp)
		assert.Nil(t, findContractInList(list, contractID),
			"deleted contract must not appear in list")
	})

	t.Run("create_invalid_yaml", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPost, "/v1/contracts",
			[]byte(invalidYAML), "text/plain", user.Token)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"body: %s", resp.Body)

		body := decodeJSONBody(t, resp)
		msg, _ := body["message"].(string)
		assert.NotEmpty(t, msg, "error response must include a message")
	})

	t.Run("read_not_found", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodGet,
			"/v1/contracts/nonexistent_"+uuid.NewString()[:8],
			nil, "", user.Token)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("update_not_found", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPut,
			"/v1/contracts/nonexistent_"+uuid.NewString()[:8],
			[]byte(initialYAML), "text/plain", user.Token)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("delete_not_found", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodDelete,
			"/v1/contracts/nonexistent_"+uuid.NewString()[:8],
			nil, "", user.Token)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("create_duplicate_name", func(t *testing.T) {
		resp := doWithAuth(t, http.MethodPost, "/v1/contracts",
			[]byte(initialYAML), "text/plain", user.Token)
		require.Equal(t, http.StatusOK, resp.StatusCode,
			"first create body: %s", resp.Body)
		first := requireContractFromBody(t, resp)

		capturedToken := user.Token
		t.Cleanup(func() {
			req, err := http.NewRequestWithContext(testCtx,
				http.MethodDelete,
				engineApp.BaseURL+"/v1/contracts/"+first.Contract.ID, nil)
			if err != nil {
				return
			}
			req.Header.Set("Authorization", "Bearer "+capturedToken)
			res, err := engineApp.Client.Do(req)
			if err != nil {
				return
			}
			defer res.Body.Close()
		})

		resp = doWithAuth(t, http.MethodPost, "/v1/contracts",
			[]byte(initialYAML), "text/plain", user.Token)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"duplicate contract name must be rejected; body: %s", resp.Body)

		body := decodeJSONBody(t, resp)
		msg, _ := body["message"].(string)
		assert.Contains(t, msg, "already exists",
			"error message should explain the conflict")
	})

	t.Run("unauthenticated_requests_rejected", func(t *testing.T) {
		for _, tc := range []struct {
			method string
			path   string
			body   []byte
		}{
			{http.MethodGet, "/v1/contracts", nil},
			{http.MethodPost, "/v1/contracts", []byte(initialYAML)},
			{http.MethodGet, "/v1/contracts/" + contractID, nil},
			{http.MethodPut, "/v1/contracts/" + contractID, []byte(updatedYAML)},
			{http.MethodDelete, "/v1/contracts/" + contractID, nil},
		} {
			resp := doWithAuth(t, tc.method, tc.path, tc.body, "text/plain", "")
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s should require auth", tc.method, tc.path)
		}
	})
}

func requireContractFromBody(t *testing.T, resp *TestResponse) testContractResponse {
	t.Helper()
	var result testContractResponse
	require.NoError(t, json.Unmarshal(resp.Body, &result),
		"failed to decode contract response; body: %s", resp.Body)
	require.NotEmpty(t, result.Contract.ID,
		"contract.id must not be empty; body: %s", resp.Body)
	return result
}

func requireContractList(t *testing.T, resp *TestResponse) []testContract {
	t.Helper()
	body := decodeJSONBody(t, resp)

	if raw, ok := body["contracts"]; ok {
		if raw == nil {
			return []testContract{}
		}
		if l, ok := raw.([]interface{}); ok {
			return decodeContractList(t, l)
		}
	}
	if raw, ok := body["data"]; ok && raw != nil {
		if l, ok := raw.([]interface{}); ok {
			return decodeContractList(t, l)
		}
	}

	var rawList []interface{}
	err := json.Unmarshal(resp.Body, &rawList)
	require.NoError(t, err,
		"list response must have a 'contracts'/'data' key or be a bare array; body: %s", resp.Body)
	return decodeContractList(t, rawList)
}

func decodeContractList(t *testing.T, rawList []interface{}) []testContract {
	t.Helper()
	contracts := make([]testContract, 0, len(rawList))
	for _, item := range rawList {
		b, err := json.Marshal(item)
		require.NoError(t, err)
		var c testContract
		require.NoError(t, json.Unmarshal(b, &c))
		contracts = append(contracts, c)
	}
	return contracts
}

func findContractInList(list []testContract, id string) *testContract {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func extractIDs(list []testContract) []string {
	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.ID
	}
	return ids
}

func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

type contractContent struct {
	ContractName  string               `json:"ContractName"`
	Version       int                  `json:"Version"`
	Description   string               `json:"Description"`
	Parties       []string             `json:"Parties"`
	EntryPoints   []string             `json:"EntryPoints"`
	TerminalSteps []string             `json:"TerminalSteps"`
	Steps         []contractStep       `json:"Steps"`
	Transitions   []contractTransition `json:"Transitions"`
}

type contractStep struct {
	ID              string              `json:"ID"`
	Owner           string              `json:"Owner"`
	Type            string              `json:"Type"`
	Timeout         string              `json:"Timeout"`
	BusinessContext *contractBizContext `json:"BusinessContext"`
}

type contractBizContext struct {
	Required []string `json:"Required"`
	Optional []string `json:"Optional"`
}

type contractTransition struct {
	From       string   `json:"From"`
	To         []string `json:"To"`
	Timeout    string   `json:"Timeout"`
	MaxRetries int      `json:"MaxRetries"`
}

func requireParsedContent(t *testing.T, raw string) contractContent {
	t.Helper()
	var c contractContent
	require.NoError(t, json.Unmarshal([]byte(raw), &c),
		"contractVersion.content must be valid JSON; raw: %s", raw)
	return c
}

func findStep(steps []contractStep, id string) *contractStep {
	for i := range steps {
		if steps[i].ID == id {
			return &steps[i]
		}
	}
	return nil
}

func findTransition(transitions []contractTransition, from string) *contractTransition {
	for i := range transitions {
		if transitions[i].From == from {
			return &transitions[i]
		}
	}
	return nil
}
