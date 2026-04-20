package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoles_GetRoles(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/roles", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	roles, ok := body["roles"].(map[string]any)
	require.True(t, ok, "response must include a roles object")

	appLevel, ok := roles["app_level"].(map[string]any)
	require.True(t, ok, "roles must include app_level")

	apiLevel, ok := roles["api_level"].(map[string]any)
	require.True(t, ok, "roles must include api_level")

	owner, ok := appLevel["owner"].(map[string]any)
	require.True(t, ok, "app_level must include an owner role")

	perms, ok := owner["permissions"].([]any)
	require.True(t, ok, "owner must have a permissions array")
	assert.NotEmpty(t, perms, "owner must have at least one permission")
	assert.Contains(t, perms, "apikey.create")

	reader, ok := apiLevel["reader"].(map[string]any)
	require.True(t, ok, "api_level must include a reader role")

	readerPerms, ok := reader["permissions"].([]any)
	require.True(t, ok, "reader must have a permissions array")
	assert.NotEmpty(t, readerPerms, "reader must have at least one permission")

	for levelName, level := range map[string]map[string]any{
		"app_level": appLevel,
		"api_level": apiLevel,
	} {
		for roleName, roleDef := range level {
			m, ok := roleDef.(map[string]any)
			require.True(t, ok, "%s.%s must be an object", levelName, roleName)
			p, ok := m["permissions"].([]any)
			require.True(t, ok, "%s.%s must have a permissions array", levelName, roleName)
			assert.NotEmpty(t, p, "%s.%s must have at least one permission", levelName, roleName)
		}
	}
}

func TestRoles_GetRolesByLevel(t *testing.T) {
	t.Run("app_level_ok", func(t *testing.T) {
		resp := doRaw(t, http.MethodGet, "/api/roles/app_level", nil, "")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "app_level", body["level"])

		roles, ok := body["roles"].(map[string]any)
		require.True(t, ok, "response must include a roles object")
		assert.NotEmpty(t, roles, "app_level must have at least one role")

		// owner is load-bearing — must always be present
		owner, ok := roles["owner"].(map[string]any)
		require.True(t, ok, "app_level must include an owner role")
		perms, ok := owner["permissions"].([]any)
		require.True(t, ok, "owner must have a permissions array")
		assert.NotEmpty(t, perms, "owner must have at least one permission")

		// Every role present must be well-formed
		for roleName, roleDef := range roles {
			m, ok := roleDef.(map[string]any)
			require.True(t, ok, "app_level.%s must be an object", roleName)
			p, ok := m["permissions"].([]any)
			require.True(t, ok, "app_level.%s must have a permissions array", roleName)
			assert.NotEmpty(t, p, "app_level.%s must have at least one permission", roleName)
		}
	})

	t.Run("api_level_ok", func(t *testing.T) {
		resp := doRaw(t, http.MethodGet, "/api/roles/api_level", nil, "")
		require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "api_level", body["level"])

		roles, ok := body["roles"].(map[string]any)
		require.True(t, ok, "response must include a roles object")
		assert.NotEmpty(t, roles, "api_level must have at least one role")

		// reader is load-bearing — must always be present
		reader, ok := roles["reader"].(map[string]any)
		require.True(t, ok, "api_level must include a reader role")
		perms, ok := reader["permissions"].([]any)
		require.True(t, ok, "reader must have a permissions array")
		assert.NotEmpty(t, perms, "reader must have at least one permission")

		// Every role present must be well-formed
		for roleName, roleDef := range roles {
			m, ok := roleDef.(map[string]any)
			require.True(t, ok, "api_level.%s must be an object", roleName)
			p, ok := m["permissions"].([]any)
			require.True(t, ok, "api_level.%s must have a permissions array", roleName)
			assert.NotEmpty(t, p, "api_level.%s must have at least one permission", roleName)
		}
	})

	t.Run("invalid_level", func(t *testing.T) {
		resp := doRaw(t, http.MethodGet, "/api/roles/not-a-level", nil, "")
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(resp.Body))

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"], "bad request must carry an error message")
		assert.Contains(t, body["error"], "Invalid level")
	})
}
