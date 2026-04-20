package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
)

func TestContractProxyHandler(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer backend.Close()

	newRouter := func() *gin.Engine {
		h := handlers.NewContractProxyHandler(backend.URL)
		r := common.SetupTestRouter()
		r.Use(func(c *gin.Context) {
			c.Request.Header.Set("Authorization", "Bearer valid-token")
		})
		r.GET("/v1/contracts", h.GetAllContracts)
		r.POST("/v1/contracts", h.CreateContract)
		r.POST("/v1/contracts/preview", h.PreviewContract)
		r.GET("/v1/contracts/:id", h.GetContract)
		r.PUT("/v1/contracts/:id", h.UpdateContract)
		r.DELETE("/v1/contracts/:id", h.DeleteContract)
		r.GET("/v1/contracts/:id/versions", h.GetAllContractVersions)
		r.GET("/v1/contracts/:id/versions/:version", h.GetContractVersion)
		r.DELETE("/v1/contracts/:id/versions/:version", h.DeleteContractVersion)
		return r
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"GetAllContracts", "GET", "/v1/contracts", nil},
		{"CreateContract", "POST", "/v1/contracts", "raw body text"},
		{"PreviewContract", "POST", "/v1/contracts/preview", map[string]string{"foo": "bar"}},
		{"GetContract", "GET", "/v1/contracts/c1", nil},
		{"UpdateContract", "PUT", "/v1/contracts/c1", "raw update body"},
		{"DeleteContract", "DELETE", "/v1/contracts/c1", nil},
		{"GetAllContractVersions", "GET", "/v1/contracts/c1/versions", nil},
		{"GetContractVersion", "GET", "/v1/contracts/c1/versions/v1", nil},
		{"DeleteContractVersion", "DELETE", "/v1/contracts/c1/versions/v1", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := common.DoRequest(t, newRouter(), tt.method, tt.path, tt.body)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}

func TestContractProxyHandler_MissingAuth(t *testing.T) {
	newRouter := func() *gin.Engine {
		h := handlers.NewContractProxyHandler("http://dummy")
		r := common.SetupTestRouter()
		r.GET("/v1/contracts", h.GetAllContracts)
		return r
	}

	w := common.DoRequest(t, newRouter(), "GET", "/v1/contracts", nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
