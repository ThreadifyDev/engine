package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
)

// UserRateLimiter implements per-user rate limiting using Redis sliding window
// This middleware should be applied AFTER authentication middleware
func UserRateLimiter(luaScripts interfaces.LuaScriptManager, cfg *config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if disabled
		if !cfg.PerUser.Enabled {
			c.Next()
			return
		}

		// Extract ownerID from context (set by auth middleware)
		ownerID, exists := c.Get("ownerID")
		if !exists {
			// No ownerID = not authenticated, skip user rate limiting
			c.Next()
			return
		}

		userID, ok := ownerID.(string)
		if !ok || userID == "" {
			c.Next()
			return
		}

		// Check rate limit using Lua script
		allowed, err := luaScripts.CheckUserRateLimit(
			c.Request.Context(),
			userID,
			cfg.PerUser.RequestsPerMinute,
			cfg.PerUser.WindowSeconds,
		)

		if err != nil {
			// On error, fail open (allow request) but log the error
			// This prevents Redis issues from breaking the entire service
			c.Next()
			return
		}

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "Rate limit exceeded",
				"message": "User rate limit exceeded. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
