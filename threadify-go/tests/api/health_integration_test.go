package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_OK(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/health", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "threadify-web-api", body["service"])
}
