package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"

	sharedauth "threadify-go/shared/auth"
)

const (
	ctxUsageMeter       = "usageMeter"
	ctxSubscriptionTier = "subscriptionTier"
)

// GetUsageMeter retrieves the usage meter from the gin context.
// Returns nil if no meter is set (unauthenticated or no subscription).
func GetUsageMeter(c *gin.Context) *models.UsageMeter {
	raw, exists := c.Get(ctxUsageMeter)
	if !exists {
		return nil
	}
	meter, ok := raw.(*models.UsageMeter)
	if !ok {
		return nil
	}
	return meter
}

// SubscriptionMiddleware loads the company's UsageMeter, enforces tenant rate limiting,
// and sets the meter in the gin context for downstream middleware/handlers.
func SubscriptionMiddleware(planSvc *service.PlanService, luaScripts interfaces.LuaScriptManager) gin.HandlerFunc {
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

		// Load the full usage meter from PlanService (cached in Valkey)
		meter, err := planSvc.GetCurrentLimits(c.Request.Context(), companyID)
		if err != nil {
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

		// Enforce tenant rate limit using the plan's MaxRateLimit (TPS)
		if meter.MaxRateLimit > 0 && luaScripts != nil {
			requestsPerMinute := meter.MaxRateLimit * 60
			windowSeconds := 60

			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Millisecond)
			allowed, luaErr := luaScripts.CheckUserRateLimit(ctx, companyID, requestsPerMinute, windowSeconds)
			cancel()

			if luaErr == nil && !allowed {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "Rate limit exceeded",
					"message": "Too many requests for your plan. Please upgrade or try again later.",
				})
				c.Abort()
				return
			}
			// On Lua/Redis error, fail open — don't block traffic
		}

		// Set meter and tier in context for downstream use
		plan, _ := planSvc.GetCompanyPlan(c.Request.Context(), companyID)
		if plan != nil {
			c.Set(ctxSubscriptionTier, string(plan.SubscriptionTier))
		}
		c.Set(ctxUsageMeter, meter)

		c.Next()
	}
}

// IngressMiddleware decrements bandwidth ingress on every thread access (create + read).
// Apply to thread/WS routes only.
func IngressMiddleware(planSvc *service.PlanService) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		if err := planSvc.DecrementIngress(c.Request.Context(), companyID, 1); err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Ingress limit reached",
				"message": err.Error(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// ContractQuotaMiddleware checks the company's contract limit before contract creation.
// It uses ContractService to count existing contracts and the usage meter for the limit.
func ContractQuotaMiddleware(planSvc *service.PlanService, contractSvc *service.ContractService) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		currentCount, err := contractSvc.CountContractsByCompany(c.Request.Context(), companyID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Quota check failed",
				"message": "Unable to verify contract quota.",
			})
			c.Abort()
			return
		}

		if quotaErr := planSvc.CheckContractQuota(c.Request.Context(), companyID, currentCount); quotaErr != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Contract limit reached",
				"message": quotaErr.Error(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// PayloadSizeMiddleware checks Content-Length against the plan's MaxPayloadBytes.
// Reads the limit from the usage meter already set in context by SubscriptionMiddleware.
func PayloadSizeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		meter := GetUsageMeter(c)
		if meter == nil {
			c.Next()
			return
		}

		if c.Request.ContentLength > 0 && c.Request.ContentLength > meter.MaxPayloadBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":   "Payload too large",
				"message": "Request body exceeds your plan's maximum payload size. Please upgrade your plan.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
