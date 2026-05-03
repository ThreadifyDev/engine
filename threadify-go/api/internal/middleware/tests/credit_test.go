package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"threadify-go/api/internal/middleware"
	agent_mocks "threadify-go/api/internal/service/mocks/service/agent"
	shderrors "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestAgentCreditCheckMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentSvc := agent_mocks.NewMockAgentService(ctrl)
		authHeader := "Bearer valid-token"

		mockAgentSvc.EXPECT().
			CheckCredits(gomock.Any(), authHeader).
			Return(true, nil)

		r := gin.New()
		r.Use(middleware.AgentCreditCheckMiddleware(mockAgentSvc))
		r.GET("/ai", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ai", nil)
		req.Header.Set("Authorization", authHeader)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("InsufficientCredits", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentSvc := agent_mocks.NewMockAgentService(ctrl)
		authHeader := "Bearer empty-token"

		mockAgentSvc.EXPECT().
			CheckCredits(gomock.Any(), authHeader).
			Return(false, nil)

		r := gin.New()
		r.Use(middleware.AgentCreditCheckMiddleware(mockAgentSvc))
		r.GET("/ai", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ai", nil)
		req.Header.Set("Authorization", authHeader)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusPaymentRequired, w.Code)
		assert.Contains(t, w.Body.String(), "Insufficient credits")
	})

	t.Run("PaymentRequiredError", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockAgentSvc := agent_mocks.NewMockAgentService(ctrl)
		authHeader := "Bearer pay-token"

		mockAgentSvc.EXPECT().
			CheckCredits(gomock.Any(), authHeader).
			Return(false, shderrors.ErrPaymentRequired)

		r := gin.New()
		r.Use(middleware.AgentCreditCheckMiddleware(mockAgentSvc))
		r.GET("/ai", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ai", nil)
		req.Header.Set("Authorization", authHeader)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusPaymentRequired, w.Code)
	})
}
