package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"

	sharedauth "threadify-go/shared/auth"
)

const (
	egressTimeout = 5 * time.Second
)

func CreditUsageMiddleware(planSvc *service.PlanService, valkeyClient interfaces.ValkeyClient, logger *zap.Logger) gin.HandlerFunc {
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

		account, err := planSvc.GetCurrentLimits(c.Request.Context(), companyID)
		if err != nil {
			if errors.Is(err, service.ErrNoAccount) {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "CREDIT_BALANCE_EXHAUSTED",
					"message": err.Error(),
				})
				c.Abort()
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "CREDIT_BALANCE_CHECK_FAILED",
				"message": "Unable to verify credit balance. Please try again.",
			})
			c.Abort()
			return
		}

		if account == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "CREDIT_BALANCE_REQUIRED",
				"message": "Credit balance required. Please purchase credits to continue.",
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

		companyIDRaw, exists := c.Get(sharedauth.CtxCompanyID)
		if !exists {
			logger.Warn("credit usage middleware: account not found in context in egress middleware")
			return
		}

		companyID, ok := companyIDRaw.(string)
		if !ok || companyID == "" {
			logger.Warn("credit usage middleware: account not found in context in egress middleware")
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
				)
			}
		}
	}
}
