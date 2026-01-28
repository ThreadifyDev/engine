package middleware

import (
	"net/http"
	"strings"
	"threadify-go/shared/jwt"

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

// DualAuthMiddleware validates either API key (X-API-Key) or JWT Bearer token for GraphQL requests
// Priority: API Key is checked first, then JWT if API Key is not present
func DualAuthMiddleware(authService *service.AuthService, jwtValidator *jwt.Validator) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try API Key authentication first
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			userInfo, err := authService.ValidateApiKey(apiKey)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
				c.Abort()
				return
			}

			// Store user info in Gin context (from API key)
			c.Set("ownerID", userInfo.OwnerID)
			c.Set("companyID", userInfo.CompanyID)
			c.Set("role", userInfo.Role)
			c.Next()
			return
		}

		// Try JWT authentication if no API Key
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Either X-API-Key or Authorization header required"})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format. Expected: Bearer <token>"})
			c.Abort()
			return
		}

		token := parts[1]

		// Validate JWT token
		claims, err := jwtValidator.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired JWT token"})
			c.Abort()
			return
		}

		// Store user info in Gin context (from JWT)
		// Map JWT claims to the same context keys used by API Key auth
		c.Set("ownerID", claims.UserID)
		c.Set("companyID", claims.CompanyID)
		c.Set("role", strings.Join(claims.Roles, ",")) // Convert roles array to comma-separated string
		c.Next()
	}
}
