package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUser_Profile_Flow(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("get_minimal_profile", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/user/profile?minimal=true", nil, "", user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)

		// User block must be present with at least an ID
		u, ok := body["user"].(map[string]any)
		require.True(t, ok, "response must include a user object")
		assert.NotEmpty(t, u["id"], "user must have an id")

		// Company block must be present and report setup incomplete
		company, ok := body["company"].(map[string]any)
		require.True(t, ok, "response must include a company object")
		assert.Equal(t, false, company["details_completed"])

		// Minimal profile must not leak heavy fields
		assert.Empty(t, company["industry"], "minimal profile must not include industry")
		assert.Empty(t, company["company_size"], "minimal profile must not include company_size")
		assert.Empty(t, company["use_case"], "minimal profile must not include use_case")
	})

	t.Run("first_time_update_requires_company_details", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/user/profile", marshalJSON(t, map[string]any{
			"full_name": "Test User",
			"job_role":  "Engineer",
		}), "application/json", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Company details are required for first-time setup", body["error"])

		// Verify state was not mutated — profile should still show setup incomplete
		profileResp := doRawWithAuth(t, http.MethodGet, "/api/user/profile?minimal=true", nil, "", user.AccessToken)
		require.Equal(t, http.StatusOK, profileResp.StatusCode)
		profileBody := decodeJSONBody(t, profileResp)
		company := profileBody["company"].(map[string]any)
		assert.Equal(t, false, company["details_completed"], "failed update must not mutate company setup state")
	})
	t.Run("first_time_update_with_company_details_succeeds", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/user/profile", marshalJSON(t, map[string]any{
			"full_name":    "Test User",
			"job_role":     "Engineer",
			"industry":     "software",
			"company_size": "small",
			"use_case":     "observability",
		}), "application/json", user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Profile updated successfully", body["message"])

		u, ok := body["user"].(map[string]any)
		require.True(t, ok, "response must include a user object")
		assert.Equal(t, "Test User", u["full_name"])
		assert.Equal(t, "Engineer", u["job_role"])
	})

	t.Run("user_fields_can_be_updated_later", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/user/profile", marshalJSON(t, map[string]any{
			"full_name": "Updated Name",
			"job_role":  "Staff Engineer",
		}), "application/json", user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)

		u, ok := body["user"].(map[string]any)
		require.True(t, ok, "response must include a user object")
		assert.Equal(t, "Updated Name", u["full_name"])
		assert.Equal(t, "Staff Engineer", u["job_role"])
	})

	t.Run("get_full_profile_reflects_all_updates", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/user/profile", nil, "", user.AccessToken)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)

		// User fields must reflect the latest update
		u, ok := body["user"].(map[string]any)
		require.True(t, ok, "response must include a user object")
		assert.Equal(t, "Updated Name", u["full_name"])
		assert.Equal(t, "Staff Engineer", u["job_role"])

		// Company fields must reflect the initial setup and nothing from the rejected attempt
		company, ok := body["company"].(map[string]any)
		require.True(t, ok, "response must include a company object")
		assert.Equal(t, true, company["details_completed"])
		assert.Equal(t, "software", company["industry"])
		assert.Equal(t, "small", company["company_size"])
		assert.Equal(t, "observability", company["use_case"])
	})
}

func TestUser_MarkInstrumentationDone(t *testing.T) {
	user := setupAuthenticatedUser(t)

	resp := doRawWithAuth(t, http.MethodPost, "/api/user/mark-instrumentation-done", []byte(`{}`), "application/json", user.AccessToken)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Instrumentation status updated", body["message"])

	profileResp := doRawWithAuth(t, http.MethodGet, "/api/user/profile", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, profileResp.StatusCode, string(profileResp.Body))

	profileBody := decodeJSONBody(t, profileResp)
	u, ok := profileBody["user"].(map[string]any)
	require.True(t, ok, "profile response must include a user object")
	assert.Equal(t, true, u["first_instrumentation_done"], "first_instrumentation_done must be true after marking done")
}

func TestUser_Profile_Unauthorized(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/user/profile", nil, "")
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.NotEmpty(t, body["error"], "unauthorized response must carry an error message")
}
