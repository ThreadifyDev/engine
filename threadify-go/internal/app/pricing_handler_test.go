package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/threadify/engine/internal/config"
	sharedconfig "threadify-go/shared/config"
)

func TestPricingHandler_ReturnsCreditConfig(t *testing.T) {
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
	require.JSONEq(t, `{"credit":{"ingress_cost_millicents":10,"egress_cost_millicents":20,"seat_cost_millicents":0,"contract_cost_millicents":30,"llm_token_cost_millicents":0,"rate_limit_tps":40,"payload_limit_bytes":50}}`, w.Body.String())
}
