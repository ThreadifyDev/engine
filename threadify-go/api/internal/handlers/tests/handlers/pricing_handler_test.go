package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPricingHandler_GetPricing_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v1/pricing", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"credit":{"ingress_cost_millicents":1}}`))
	}))
	defer engine.Close()

	h := handlers.NewPricingHandler(engine.URL)

	r := gin.New()
	r.GET("/api/pricing", h.GetPricing)

	req := httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.JSONEq(t, `{"credit":{"ingress_cost_millicents":1}}`, w.Body.String())
}

func TestPricingHandler_GetPricing_BadGateway_WhenEngineUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := handlers.NewPricingHandler("http://127.0.0.1:1")

	r := gin.New()
	r.GET("/api/pricing", h.GetPricing)

	req := httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "Failed to connect to ThreadifyEngine")
}
