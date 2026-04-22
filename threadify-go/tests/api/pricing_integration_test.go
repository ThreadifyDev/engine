package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricing_Endpoint_Mounted(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/pricing", nil, "")
	require.Equal(t, http.StatusBadGateway, resp.StatusCode, "expected 502 when engine is not running")

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Failed to connect to ThreadifyEngine", body["error"])
}
