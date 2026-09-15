package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	sharedconfig "threadify-go/shared/config"

	"github.com/threadify/engine/internal/config"
)

func TestPricingHandler_DoesNotAdvertiseLegacyCreditPrices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{}
	cfg.Subscription = sharedconfig.SubscriptionConfig{
		Credit: sharedconfig.CreditConfig{
			IngressCostMillicents:  10,
			EgressCostMillicents:   20,
			ContractCostMillicents: 30,
			RateLimitTPS:           40,
			PayloadLimitBytes:      50,
		},
	}

	r := gin.New()
	r.GET("/v1/pricing", pricingHandler(cfg))

	req := httptest.NewRequest(http.MethodGet, "/v1/pricing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"billing_source":"registry"`)
	require.NotContains(t, w.Body.String(), "millicents")
	require.NotContains(t, w.Body.String(), "payload_limit_bytes")
}
