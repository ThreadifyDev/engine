package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/middleware"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	shareddomain "threadify-go/shared/domain"
)

func TestCreditUsageMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	t.Run("MissingCompanyID", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockPlanSvc := enginemocks.NewMockPlanService(ctrl)

		r := gin.New()
		r.Use(middleware.CreditUsageMiddleware(mockPlanSvc, logger))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Unauthorized")
	})

	t.Run("InsufficientBalance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockPlanSvc := enginemocks.NewMockPlanService(ctrl)

		companyID := "comp-123"
		mockPlanSvc.EXPECT().
			CheckBalancePositive(gomock.Any(), companyID).
			Return(nil, service.ErrInsufficientCredit)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(sharedauth.CtxCompanyID, companyID)
			c.Next()
		})
		r.Use(middleware.CreditUsageMiddleware(mockPlanSvc, logger))
		r.GET("/test", func(c *gin.Context) {
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusPaymentRequired, w.Code)
		assert.Contains(t, w.Body.String(), "CREDIT_BALANCE_EXHAUSTED")
	})

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockPlanSvc := enginemocks.NewMockPlanService(ctrl)

		companyID := "comp-123"
		account := &shareddomain.CreditAccount{CompanyID: companyID}

		mockPlanSvc.EXPECT().
			CheckBalancePositive(gomock.Any(), companyID).
			Return(account, nil)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(sharedauth.CtxCompanyID, companyID)
			c.Next()
		})
		r.Use(middleware.CreditUsageMiddleware(mockPlanSvc, logger))
		r.GET("/test", func(c *gin.Context) {
			val, _ := c.Get(sharedauth.CtxCreditAccount)
			assert.Equal(t, account, val)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

}

func TestEgressMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockPlanSvc := enginemocks.NewMockPlanService(ctrl)

		companyID := "comp-123"
		account := &shareddomain.CreditAccount{CompanyID: companyID}

		mockPlanSvc.EXPECT().
			DecrementEgress(gomock.Any(), companyID, gomock.Any()).
			Return(nil)

		r := gin.New()
		r.Use(func(c *gin.Context) {
			c.Set(sharedauth.CtxCreditAccount, account)
			c.Next()
		})
		r.Use(middleware.EgressMiddleware(mockPlanSvc, logger))
		r.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "hello world") // 11 bytes
		})

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}
