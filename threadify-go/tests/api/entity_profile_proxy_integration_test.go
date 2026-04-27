package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityProfileProxy_Unauthorized(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/entity-profiles?refKey=abc&type=customer"},
		{http.MethodGet, "/api/entity-profiles/types"},
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

func TestEntityProfileProxy_GetEntityProfile_MissingParams(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("missing_both", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})

	t.Run("missing_type", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?refKey=abc", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})

	t.Run("missing_refKey", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?type=customer", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.Equal(t, "refKey and type query params are required", body["error"])
	})
}

func TestEntityProfileProxy_EngineUnavailable(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("get_entity_profile", func(t *testing.T) {
		resp := doRawWithAuth(t, http.MethodGet, "/api/entity-profiles?refKey=abc&type=customer", nil, "", user.AccessToken)
		require.Equal(t, http.StatusBadGateway, resp.StatusCode,
			"expected 502 when engine is not running: %s", string(resp.Body))

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"])
	})

}
