package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/billing"
)

const (
	egressTimeout = 5 * time.Second
)

func CreditUsageMiddleware(planSvc *service.PlanService, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		companyIDRaw, exists := c.Get(sharedauth.CtxCompanyID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "You are not authorized to perform this action",
			})
			c.Abort()
			return
		}

		companyID, ok := companyIDRaw.(string)
		if !ok || companyID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "You are not authorized to perform this action",
			})
			c.Abort()
			return
		}

		account, err := planSvc.CheckBalancePositive(c.Request.Context(), companyID)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrNoAccount):
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "NO_BILLING_ACCOUNT",
					"message": "No billing account found. Please set up billing to continue.",
				})
			case errors.Is(err, service.ErrInsufficientCredit):
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "CREDIT_BALANCE_EXHAUSTED",
					"message": "Credit balance exhausted. Please purchase credits to continue.",
				})
			default:
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "CREDIT_BALANCE_CHECK_FAILED",
					"message": "Unable to verify credit balance. Please try again.",
				})
			}
			c.Abort()
			return
		}

		if account == nil {
			logger.Error("credit usage middleware: CheckBalancePositive returned nil account with no error",
				zap.String("company_id", companyID),
			)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "CREDIT_BALANCE_CHECK_FAILED",
				"message": "Unable to verify credit balance. Please try again.",
			})
			c.Abort()
			return
		}

		c.Set(sharedauth.CtxCreditAccount, account)

		allowed, err := planSvc.CheckRateLimit(c.Request.Context(), account)
		if err != nil {
			logger.Warn("credit usage middleware: rate limit check failed, failing open",
				zap.Error(err),
				zap.String("company_id", companyID),
			)
		} else if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "RATE_LIMIT_EXCEEDED",
				"message": "Company rate limit exceeded. Please upgrade your plan for higher TPS.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func EgressMiddleware(planSvc *service.PlanService, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		size := int64(c.Writer.Size())
		if size <= 0 {
			return
		}

		var companyID string
		if accountRaw, exists := c.Get(sharedauth.CtxCreditAccount); exists {
			if account, ok := accountRaw.(*billing.CreditAccount); ok && account != nil {
				companyID = account.CompanyID
			}
		}

		if companyID == "" {
			if idRaw, exists := c.Get(sharedauth.CtxCompanyID); exists {
				if id, ok := idRaw.(string); ok {
					companyID = id
				}
			}
		}

		if companyID == "" {
			logger.Warn("egress middleware: company_id not found in context, skipping egress decrement")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), egressTimeout)
		defer cancel()

		if err := planSvc.DecrementEgress(ctx, companyID, size); err != nil {
			if errors.Is(err, service.ErrInsufficientCredit) {
				logger.Warn("egress middleware: insufficient credits",
					zap.String("company_id", companyID),
					zap.Int64("size", size),
				)
			} else {
				logger.Error("egress middleware: egress decrement failed",
					zap.Error(err),
					zap.String("company_id", companyID),
					zap.Int64("size", size),
				)
			}
		}
	}
}
