package middleware

import (
	"net/http"
	"threadify-go/api/internal/ports"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
)

func AuthAccessTokenAuth(authService ports.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}
		token, err := sharedauth.ExtractBearerToken(authHeader)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		claims, err := authService.VerifyToken(c.Request.Context(), token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		dbRoles, err := authService.GetUserRoles(c.Request.Context(), claims.UserID, "user")
		if err == nil && len(dbRoles) > 0 {
			claims.Roles = dbRoles
		}

		sharedauth.SetGinContextFromClaims(c, claims)

		c.Next()
	}
}
