package engine

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestEngine_Pricing(t *testing.T) {
	resp := httpc.DoRaw(t, http.MethodGet, "/v1/pricing", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := enginetest.DecodeJSONBody(t, resp)
	credit, ok := body["credit"].(map[string]interface{})
	require.True(t, ok, "expected 'credit' to be an object, got %T", body["credit"])

	assert.NotNil(t, credit)
}
