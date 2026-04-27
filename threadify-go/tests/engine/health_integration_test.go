package engine

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/tests/internal/enginetest"
)

func TestEngine_Health(t *testing.T) {
	resp := httpc.DoRaw(t, http.MethodGet, "/health", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := enginetest.DecodeJSONBody(t, resp)
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["postgres"])
	assert.Equal(t, "ok", body["valkey"])
}
