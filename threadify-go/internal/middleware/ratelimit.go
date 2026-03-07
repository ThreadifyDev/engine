package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
)

func IPRateLimitMiddleware(luaScripts interfaces.LuaScriptManager, rateCfg *config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for health and metrics
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		if !rateCfg.Enabled || !rateCfg.IPRateLimitEnabled {
			c.Next()
			return
		}

		ip := c.ClientIP()
		if ip == "" {
			c.Next()
			return
		}

		windowSeconds := rateCfg.WindowSeconds
		if windowSeconds <= 0 {
			windowSeconds = 60
		}
		requestsPerWindow := rateCfg.IPRequestsPerWindow
		if requestsPerWindow <= 0 {
			requestsPerWindow = 100
		}

		redisTimeout := time.Duration(rateCfg.RedisTimeoutMs) * time.Millisecond
		if redisTimeout <= 0 {
			redisTimeout = 5 * time.Millisecond
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), redisTimeout)
		defer cancel()

		allowed, err := luaScripts.CheckIPRateLimit(ctx, ip, requestsPerWindow, windowSeconds)
		if err != nil {
			// On error, fail open to avoid blocking legitimate traffic
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Too many requests",
				"message": "Rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
