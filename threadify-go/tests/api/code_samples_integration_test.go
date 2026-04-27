package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeSamples_DefaultType(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/code-samples", nil, "")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(resp.Body))

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "basic_instrumentation", body["code_type"])

	samples, ok := body["samples"].(map[string]any)
	require.True(t, ok)
	_, hasGo := samples["go"].(string)
	assert.True(t, hasGo)
}

func TestCodeSamples_UnknownType(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/api/code-samples?codeType=does_not_exist", nil, "")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "Code type not found", body["error"])
}
