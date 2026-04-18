package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
)

func TestEntityProfileProxyHandler(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": {"profiles": []}}`))
	}))
	defer backend.Close()

	newRouter := func() *gin.Engine {
		h := handlers.NewEntityProfileProxyHandler(backend.URL, zap.NewNop())
		r := common.SetupTestRouter()
		r.Use(func(c *gin.Context) {
			c.Request.Header.Set("Authorization", "Bearer valid-token")
		})
		r.GET("/v1/entity-profiles/:id", h.GetEntityProfile)
		r.GET("/v1/entity-profile-types", h.ListEntityProfileTypes)
		return r
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"GetEntityProfile", "GET", "/v1/entity-profiles/ep1?refKey=abc&type=client", nil},
		{"ListEntityProfileTypes", "GET", "/v1/entity-profile-types", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := common.DoRequest(t, newRouter(), tt.method, tt.path, tt.body)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestEntityProfileProxyHandler_MissingAuth(t *testing.T) {
	newRouter := func() *gin.Engine {
		h := handlers.NewEntityProfileProxyHandler("http://dummy", zap.NewNop())
		r := common.SetupTestRouter()
		r.GET("/v1/entity-profile-types", h.ListEntityProfileTypes)
		return r
	}

	w := common.DoRequest(t, newRouter(), "GET", "/v1/entity-profile-types", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
