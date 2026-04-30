package api

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAccounts_GetPermissions_APILevel(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/scopes/api_level/permissions", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "api_level", body["scope"],
		"response must echo back the requested scope")

	roles, ok := body["roles"].(map[string]any)
	require.True(t, ok, "roles must be an object")
	assert.NotEmpty(t, roles, "api_level must have at least one role defined")

	_, hasReader := roles["reader"]
	assert.True(t, hasReader, "api_level must include the reader role")

	_, hasStandardService := roles["standard_service"]
	assert.True(t, hasStandardService, "api_level must include the standard_service role")
}

func TestServiceAccounts_GetPermissions_InvalidScope(t *testing.T) {
	user := setupAuthenticatedUser(t)

	for _, scope := range []string{"runtime_level", "admin_level", "unknown", ""} {
		t.Run("scope_"+scope, func(t *testing.T) {
			path := "/api/service-accounts/scopes/" + scope + "/permissions"
			resp := doRawWithAuth(t, http.MethodGet, path, nil, "", user.AccessToken)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)

			body := decodeJSONBody(t, resp)
			assert.Equal(t, "Service accounts must use 'api_level' scope", body["error"])
			assert.Nil(t, body["roles"],
				"invalid scope must not return any roles")
		})
	}
}

func TestServiceAccounts_GetPermissions_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/service-accounts/scopes/api_level/permissions", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["roles"])
}

func TestServiceAccounts_Create_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "my-service-account",
		"role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Service account created successfully", body["message"])

	sa, ok := body["service_account"].(map[string]any)
	require.True(t, ok, "response must include a service_account object")
	assert.NotEmpty(t, sa["id"])
	assert.Equal(t, "my-service-account", sa["name"])
	assert.Equal(t, user.CompanyID, sa["company_id"],
		"service account must belong to the authenticated user's company")
	assert.Equal(t, true, sa["is_active"],
		"newly created service account must be active")
	assert.NotEmpty(t, sa["created_at"])
}

func TestServiceAccounts_Create_WithDescription(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name":        "sa-with-desc",
		"role":        "standard_service",
		"description": "used for automated testing",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	sa := body["service_account"].(map[string]any)
	assert.Equal(t, "used for automated testing", sa["description"],
		"description must be stored and returned")
}

func TestServiceAccounts_Create_InvalidRole(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "sa-bad-role",
		"role": "not-a-valid-role",
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "invalid role: must be 'standard_service' or 'reader'", body["error"])
	assert.Nil(t, body["service_account"],
		"error response must not include a service account")
}

func TestServiceAccounts_Create_MissingName(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"])
}

func TestServiceAccounts_Create_MissingRole(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "sa-no-role",
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"])
}

func TestServiceAccounts_Create_MissingBody(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/service-accounts", nil, "application/json", user.AccessToken)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"])
}

func TestServiceAccounts_Create_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodPost, "/api/service-accounts",
		marshalJSON(t, map[string]any{"name": "sa", "role": "reader"}),
		"application/json")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceAccounts_List_ReturnsOwnCompanyOnly(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	// user1 creates two service accounts
	for _, name := range []string{"sa-a", "sa-b"} {
		r := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
			"name": name, "role": "reader",
		}, user1.AccessToken)
		require.Equal(t, http.StatusCreated, r.StatusCode)
	}

	// user2 creates one
	r := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "sa-c", "role": "reader",
	}, user2.AccessToken)
	require.Equal(t, http.StatusCreated, r.StatusCode)

	listResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts", nil, "", user1.AccessToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	listBody := decodeJSONBody(t, listResp)
	accounts, ok := listBody["service_accounts"].([]any)
	require.True(t, ok, "service_accounts must be an array")

	assert.GreaterOrEqual(t, len(accounts), 2,
		"user1 must see at least their two explicitly created service accounts")

	for _, a := range accounts {
		sa := a.(map[string]any)
		assert.Equal(t, user1.CompanyID, sa["company_id"],
			"listed service account must belong to user1's company, not user2's")
	}
}

func TestServiceAccounts_List_EmptyForNewCompany(t *testing.T) {
	user := setupAuthenticatedUser(t)

	listResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	listBody := decodeJSONBody(t, listResp)
	_, exists := listBody["service_accounts"]
	assert.True(t, exists, "service_accounts key must always be present")
}

func TestServiceAccounts_List_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/service-accounts", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceAccounts_Get_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "sa-to-get",
		"role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	body := decodeJSONBody(t, getResp)
	sa, ok := body["service_account"].(map[string]any)
	require.True(t, ok, "response must include a service_account object")
	assert.Equal(t, saID, sa["id"])
	assert.Equal(t, "sa-to-get", sa["name"])
	assert.Equal(t, user.CompanyID, sa["company_id"])
}

func TestServiceAccounts_Get_NonexistentID(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+uuid.NewString(), nil, "", user.AccessToken)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "service account not found", body["error"])
	assert.Nil(t, body["service_account"])
}

func TestServiceAccounts_Get_WrongCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "user1-sa", "role": "reader",
	}, user1.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	resp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user2.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"],
		"cross-company get must not return service account data")
}

func TestServiceAccounts_Get_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/service-accounts/"+uuid.NewString(), nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceAccounts_Update_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "original-name", "role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	updateResp := doJSONWithAuth(t, http.MethodPut, "/api/service-accounts/"+saID, map[string]any{
		"name":        "updated-name",
		"description": "updated description",
		"is_active":   false,
	}, user.AccessToken)
	require.Equal(t, http.StatusOK, updateResp.StatusCode, string(updateResp.Body))

	body := decodeJSONBody(t, updateResp)
	assert.Equal(t, "Service account updated successfully", body["message"])

	sa, ok := body["service_account"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, saID, sa["id"],
		"updated service account must have the same id")
	assert.Equal(t, "updated-name", sa["name"],
		"name must be updated")
	assert.Equal(t, "updated description", sa["description"],
		"description must be fully replaced")
	assert.Equal(t, false, sa["is_active"],
		"is_active must be updated to false")
	assert.Equal(t, user.CompanyID, sa["company_id"],
		"company_id must not change on update")

	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	getBody := decodeJSONBody(t, getResp)
	getSA := getBody["service_account"].(map[string]any)
	assert.Equal(t, "updated-name", getSA["name"],
		"update must persist — GET must reflect the new name")
	assert.Equal(t, "updated description", getSA["description"],
		"update must persist — GET must reflect the new description")
	assert.Equal(t, false, getSA["is_active"],
		"update must persist — GET must reflect is_active=false")
}

func TestServiceAccounts_Update_RejectsPartialPayload(t *testing.T) {
	user := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "partial-sa", "role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	updateResp := doJSONWithAuth(t, http.MethodPut, "/api/service-accounts/"+saID, map[string]any{
		"name": "partial-updated",
	}, user.AccessToken)
	require.Equal(t, http.StatusBadRequest, updateResp.StatusCode)

	body := decodeJSONBody(t, updateResp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"])

	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	sa := decodeJSONBody(t, getResp)["service_account"].(map[string]any)
	assert.Equal(t, "partial-sa", sa["name"])
	assert.Equal(t, true, sa["is_active"])
}

func TestServiceAccounts_Update_NonexistentID(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doJSONWithAuth(t, http.MethodPut, "/api/service-accounts/"+uuid.NewString(), map[string]any{
		"name":        "ghost",
		"description": "",
		"is_active":   true,
	}, user.AccessToken)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "service account not found", body["error"])
	assert.Nil(t, body["service_account"])
}

func TestServiceAccounts_Update_WrongCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "user1-sa", "role": "reader",
	}, user1.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	resp := doJSONWithAuth(t, http.MethodPut, "/api/service-accounts/"+saID, map[string]any{
		"name":        "hijacked",
		"description": "",
		"is_active":   false,
	}, user2.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])
	assert.Nil(t, body["service_account"])

	// Verify user1's SA was not modified
	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user1.AccessToken)
	getSA := decodeJSONBody(t, getResp)["service_account"].(map[string]any)
	assert.Equal(t, "user1-sa", getSA["name"],
		"cross-company update must not modify the service account")
}

func TestServiceAccounts_Update_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodPut, "/api/service-accounts/"+uuid.NewString(),
		marshalJSON(t, map[string]any{"name": "x"}), "application/json")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServiceAccounts_Delete_Success(t *testing.T) {
	user := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "sa-to-delete", "role": "reader",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	deleteResp := doRawWithAuth(t, http.MethodDelete, "/api/service-accounts/"+saID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)

	body := decodeJSONBody(t, deleteResp)
	assert.Equal(t, "Service account deleted successfully", body["message"])

	// Verify it's actually gone
	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusNotFound, getResp.StatusCode,
		"deleted service account must return 404 on subsequent GET")
}

func TestServiceAccounts_Delete_NonexistentID(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodDelete, "/api/service-accounts/"+uuid.NewString(), nil, "", user.AccessToken)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "service account not found", body["error"])
}

func TestServiceAccounts_Delete_WrongCompany(t *testing.T) {
	user1 := setupAuthenticatedUser(t)
	user2 := setupAuthenticatedUser(t)

	createResp := doJSONWithAuth(t, http.MethodPost, "/api/service-accounts", map[string]any{
		"name": "user1-sa", "role": "reader",
	}, user1.AccessToken)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)
	saID := decodeJSONBody(t, createResp)["service_account"].(map[string]any)["id"].(string)

	resp := doRawWithAuth(t, http.MethodDelete, "/api/service-accounts/"+saID, nil, "", user2.AccessToken)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"])

	// Verify SA still exists for user1
	getResp := doRawWithAuth(t, http.MethodGet, "/api/service-accounts/"+saID, nil, "", user1.AccessToken)
	require.Equal(t, http.StatusOK, getResp.StatusCode,
		"cross-company delete must not remove the service account")
}

func TestServiceAccounts_Delete_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodDelete, "/api/service-accounts/"+uuid.NewString(), nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
