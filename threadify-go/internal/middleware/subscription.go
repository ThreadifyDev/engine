package middleware

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/service"

	sharedauth "threadify-go/shared/auth"
)

func SubscriptionMiddleware(planSvc *service.PlanService, valkeyClient interfaces.ValkeyClient, luaScripts interfaces.LuaScriptManager, rateCfg *config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		companyIDRaw, exists := c.Get(sharedauth.CtxCompanyID)
		if !exists {
			c.Next()
			return
		}

		companyID, ok := companyIDRaw.(string)
		if !ok || companyID == "" {
			c.Next()
			return
		}

		suspendedKey := database.SuspendedPlanPrefix + companyID
		if val, err := valkeyClient.Get(c.Request.Context(), suspendedKey); err == nil && val != "" {
			c.JSON(http.StatusPaymentRequired, gin.H{
				"error":   "Account suspended",
				"message": "Your account has been suspended due to a failed payment. Please update your payment method.",
			})
			c.Abort()
			return
		}

		meter, err := planSvc.GetCurrentLimits(c.Request.Context(), companyID)
		if err != nil {
			if errors.Is(err, service.ErrSubscriptionExpired) {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error":   "Subscription expired",
					"message": "Your subscription has expired. Please renew to continue using this service.",
				})
				c.Abort()
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Subscription check failed",
				"message": "Unable to verify subscription. Please try again.",
			})
			c.Abort()
			return
		}
		if meter == nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Subscription required",
				"message": "An active subscription is required to use this service.",
			})
			c.Abort()
			return
		}

		if meter.MaxRateLimit > 0 && luaScripts != nil {
			windowSeconds := rateCfg.WindowSeconds
			if windowSeconds <= 0 {
				windowSeconds = 60
			}
			requestsPerWindow := meter.MaxRateLimit * windowSeconds

			redisTimeout := time.Duration(rateCfg.RedisTimeoutMs) * time.Millisecond
			if redisTimeout <= 0 {
				redisTimeout = 5 * time.Millisecond
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), redisTimeout)
			allowed, luaErr := luaScripts.CheckCompanyRateLimit(ctx, companyID, requestsPerWindow, windowSeconds)
			cancel()

			if luaErr == nil && !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Rate limit exceeded",
					"message": "Too many requests for your plan. Please upgrade or try again later.",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
