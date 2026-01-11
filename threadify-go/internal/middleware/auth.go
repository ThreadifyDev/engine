package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/service"
)

func AuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		claims, err := authService.VerifyToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("userID", claims["sub"])
		c.Set("claims", claims)
		c.Next()
	}
}

// GraphQLAuthMiddleware validates API key for GraphQL requests
func GraphQLAuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "X-API-Key header required"})
			c.Abort()
			return
		}

		userInfo, err := authService.ValidateApiKey(apiKey)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
			c.Abort()
			return
		}

		// Store user info in Gin context
		c.Set("ownerID", userInfo.OwnerID)
		c.Set("companyID", userInfo.CompanyID)
		c.Set("role", userInfo.Role)
		c.Next()
	}
}
