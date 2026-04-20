package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeys_Create_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name": "my-key",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)

	key, ok := body["key"].(string)
	require.True(t, ok, "key must be a string")
	assert.NotEmpty(t, key)
	assert.Contains(t, key, body["key_prefix"].(string),
		"returned key must start with the key_prefix")

	// key_prefix is stored and used for display
	keyPrefix, ok := body["key_prefix"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, keyPrefix)

	// api_key object must have all expected fields
	apiKey, ok := body["api_key"].(map[string]any)
	require.True(t, ok, "api_key must be an object")
	assert.NotEmpty(t, apiKey["id"])
	assert.Equal(t, "my-key", apiKey["name"])
	assert.Equal(t, keyPrefix, apiKey["key_prefix"])
	assert.Equal(t, user.CompanyID, apiKey["company_id"],
		"api_key must belong to the authenticated user's company")
	assert.Nil(t, apiKey["expires_at"],
		"key created without expires_in must have no expiry")
	assert.Nil(t, apiKey["revoked_at"],
		"newly created key must not be revoked")
}

func TestAPIKeys_Create_WithExpiry(t *testing.T) {
	user := setupAuthenticatedUser(t)

	expiresIn := 30
	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name":       "expiring-key",
		"expires_in": expiresIn,
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	apiKey := body["api_key"].(map[string]any)

	expiresAtRaw, ok := apiKey["expires_at"].(string)
	require.True(t, ok, "expires_at must be set when expires_in is provided")

	expiresAt, err := time.Parse(time.RFC3339, expiresAtRaw)
	require.NoError(t, err, "expires_at must be a valid RFC3339 timestamp")

	expectedExpiry := time.Now().AddDate(0, 0, expiresIn)
	assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second,
		"expires_at must be approximately now + expires_in days")
}

func TestAPIKeys_Create_MissingName(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Invalid request body", body["error"],
		"missing name must return binding error before service validation")
}

func TestAPIKeys_Create_EmptyName(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name": "",
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
}

func TestAPIKeys_Create_MissingBody(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/api-keys", nil, "application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Invalid request body", body["error"])
}

func TestAPIKeys_Create_WithServiceAccount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	saResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "test-sa",
		"role": "standard_service",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, saResp.StatusCode, string(saResp.Body))
	saBody := decodeJSONBody(t, saResp)
	saID := saBody["service_account"].(map[string]any)["id"].(string)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name":               "sa-key",
		"service_account_id": saID,
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	apiKey := body["api_key"].(map[string]any)
	assert.Equal(t, saID, apiKey["service_account_id"],
		"key must be linked to the specified service account")
	assert.Nil(t, apiKey["user_id"],
		"key linked to service account must not have user_id set")
}

func TestAPIKeys_Create_NonexistentServiceAccount(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name":               "key",
		"service_account_id": "sa_doesnotexist_123",
	}, user.AccessToken)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "service account not found", body["error"])
}

func TestAPIKeys_Create_ServiceAccountWrongCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	// user2 creates a service account in their company
	saResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "other-company-sa",
		"role": "standard_service",
	}, user2.AccessToken)
	require.Equal(t, http.StatusCreated, saResp.StatusCode, string(saResp.Body))
	saBody := decodeJSONBody(t, saResp)
	saID := saBody["service_account"].(map[string]any)["id"].(string)

	// user1 tries to create a key linked to user2's service account
	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name":               "cross-company-key",
		"service_account_id": saID,
	}, user1.AccessToken)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "unauthorized: service account belongs to different company", body["error"])
}

func TestAPIKeys_Create_InvalidServiceAccountRole(t *testing.T) {
	user := setupAuthenticatedUser(t)

	invalidRole := "nonexistent_role"
	resp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{
		"name":                   "key",
		"create_service_account": true,
		"service_account_role":   invalidRole,
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid service account role: must be from api_level", body["error"])
}

func TestAPIKeys_List_IsolatedByCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t) // different company

	// user1 creates two keys
	for _, name := range []string{"key-a", "key-b"} {
		r := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{"name": name}, user1.AccessToken)
		require.Equal(t, http.StatusCreated, r.StatusCode)
	}

	// user2 creates one key
	r := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{"name": "key-c"}, user2.AccessToken)
	require.Equal(t, http.StatusCreated, r.StatusCode)

	// user1 should only see their own two keys
	listResp := doRawWithAuth(t, http.MethodGet, "/api/api-keys", nil, "", user1.AccessToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	listBody := decodeJSONBody(t, listResp)
	keys := listBody["api_keys"].([]any)
	assert.Len(t, keys, 2, "user1 must only see their own company's keys")

	// Verify none of user2's keys leaked into user1's list
	for _, k := range keys {
		key := k.(map[string]any)
		assert.Equal(t, user1.CompanyID, key["company_id"],
			"listed key must belong to user1's company")
	}
}

func TestAPIKeys_List_NeverExposesKeyHash(t *testing.T) {
	user := setupAuthenticatedUser(t)

	doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{"name": "key"}, user.AccessToken)

	listResp := doRawWithAuth(t, http.MethodGet, "/api/api-keys", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	listBody := decodeJSONBody(t, listResp)
	keys := listBody["api_keys"].([]any)
	require.Len(t, keys, 1)

	key := keys[0].(map[string]any)
	_, hasHash := key["key_hash"]
	assert.False(t, hasHash, "key_hash must never be returned in list response")
	_, hasKey := key["key"]
	assert.False(t, hasKey, "raw key must never be returned after creation")
}

func TestAPIKeys_Revoke_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{"name": "to-revoke"}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	createBody := decodeJSONBody(t, createResp)
	keyID := createBody["api_key"].(map[string]any)["id"].(string)

	revokeResp := doRawWithAuth(t, http.MethodDelete, "/api/api-keys/"+keyID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, revokeResp.StatusCode)
	revokeBody := decodeJSONBody(t, revokeResp)
	assert.Equal(t, "API key revoked successfully", revokeBody["message"])

	// Key must no longer appear in list
	listResp := doRawWithAuth(t, http.MethodGet, "/api/api-keys", nil, "", user.AccessToken)
	listBody := decodeJSONBody(t, listResp)
	assertEmptyOrNilList(t, listBody, "api_keys")
}

func TestAPIKeys_Revoke_NonexistentKey(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodDelete, "/api/api-keys/nonexistent_id", nil, "", user.AccessToken)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "API key not found", body["error"])
}

func TestAPIKeys_Revoke_WrongCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	// user1 creates a key
	createResp := doJSONWithAuth(t, http.MethodPost, "/api/api-keys", map[string]any{"name": "user1-key"}, user1.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	keyID := decodeJSONBody(t, createResp)["api_key"].(map[string]any)["id"].(string)

	// user2 tries to revoke user1's key
	resp := doRawWithAuth(t, http.MethodDelete, "/api/api-keys/"+keyID, nil, "", user2.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "unauthorized", body["error"])

	// user1's key must still be intact
	listResp := doRawWithAuth(t, http.MethodGet, "/api/api-keys", nil, "", user1.AccessToken)
	listBody := decodeJSONBody(t, listResp)
	keys := listBody["api_keys"].([]any)
	assert.Len(t, keys, 1, "user1's key must not have been revoked by user2")
}

func TestAPIKeys_Unauthorized(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/api-keys"},
		{http.MethodPost, "/api/api-keys"},
		{http.MethodDelete, "/api/api-keys/some-id"},
	} {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			resp := doRaw(t, tc.method, tc.path, nil, "")
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"%s %s must require authentication", tc.method, tc.path)
		})
	}
}
