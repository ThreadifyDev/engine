package engine

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEngine_Health(t *testing.T) {
	resp := doRaw(t, http.MethodGet, "/health", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := decodeJSONBody(t, resp)
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["postgres"])
	assert.Equal(t, "ok", body["valkey"])
}
