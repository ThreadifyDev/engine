package middleware

import (
	"errors"
	"net/http"
	"threadify-go/api/internal/service"
	shderrors "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
)

// AgentCreditCheckMiddleware ensures the user has sufficient credits before allowing AI operations.
func AgentCreditCheckMiddleware(agentSvc *service.AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(service.HeaderAuthorization)
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		available, err := agentSvc.CheckCredits(c.Request.Context(), authHeader)
		if err != nil {
			if errors.Is(err, shderrors.ErrPaymentRequired) {
				c.JSON(http.StatusPaymentRequired, gin.H{"error": "Insufficient credits"})
				c.Abort()
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify credits"})
			c.Abort()
			return
		}

		if !available {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": "Insufficient credits: please top up your account to continue using the AI agent"})
			c.Abort()
			return
		}

		c.Next()
	}
}
