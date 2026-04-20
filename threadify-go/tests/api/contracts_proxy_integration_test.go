package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContracts_Proxy_Unauthorized(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/contracts"},
		{http.MethodPost, "/api/contracts"},
		{http.MethodPost, "/api/contracts/preview"},
		{http.MethodGet, "/api/contracts/some-id"},
		{http.MethodPut, "/api/contracts/some-id"},
		{http.MethodDelete, "/api/contracts/some-id"},
		{http.MethodGet, "/api/contracts/some-id/versions"},
		{http.MethodGet, "/api/contracts/some-id/versions/1"},
		{http.MethodDelete, "/api/contracts/some-id/versions/1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+"_"+ep.path, func(t *testing.T) {
			resp := doRaw(t, ep.method, ep.path, nil, "")
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
				"expected 401 for unauthenticated %s %s, got %d: %s",
				ep.method, ep.path, resp.StatusCode, string(resp.Body))

			body := decodeJSONBody(t, resp)
			assert.NotEmpty(t, body["error"], "unauthorized response must carry an error message")
		})
	}
}

func TestContracts_Proxy_RouteMounted(t *testing.T) {
	user := setupAuthenticatedUser(t)

	endpoints := []struct {
		name   string
		method string
		path   string
		body   []byte
		ct     string
	}{
		{"list_all", http.MethodGet, "/api/contracts", nil, ""},
		{"create", http.MethodPost, "/api/contracts", []byte("name: test\nsteps: []"), "text/plain"},
		{"get_by_id", http.MethodGet, "/api/contracts/nonexistent-id", nil, ""},
		{"update", http.MethodPut, "/api/contracts/nonexistent-id", []byte("name: updated"), "text/plain"},
		{"delete", http.MethodDelete, "/api/contracts/nonexistent-id", nil, ""},
		{"list_versions", http.MethodGet, "/api/contracts/nonexistent-id/versions", nil, ""},
		{"get_version", http.MethodGet, "/api/contracts/nonexistent-id/versions/1", nil, ""},
		{"delete_version", http.MethodDelete, "/api/contracts/nonexistent-id/versions/1", nil, ""},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			resp := doRawWithAuth(t, ep.method, ep.path, ep.body, ep.ct, user.AccessToken)
			require.Equal(t, http.StatusBadGateway, resp.StatusCode,
				"expected 502 (engine not running) for authenticated %s %s, got %d: %s",
				ep.method, ep.path, resp.StatusCode, string(resp.Body))

			body := decodeJSONBody(t, resp)
			assert.Equal(t, "Failed to connect to ThreadifyEngine", body["error"])
		})
	}
}

func TestContracts_Proxy_PreviewBadBody(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("nil_body", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/contracts/preview", nil, "application/json", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Invalid request body", body["error"])
	})

	t.Run("malformed_json", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/contracts/preview", []byte(`{not valid json`), "application/json", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Invalid request body", body["error"])
	})

	t.Run("wrong_content_type", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodPost, "/api/contracts/preview", []byte(`{"key": "value"}`), "text/plain", user.AccessToken)
		require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "Content-Type must be application/json", body["error"])
	})
}
