package middleware

import (
	"net/http"
	"threadify-go/api/internal/ports"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
)

// AuthAccessTokenAuth accepts a Registry-backed browser session or Threadify API key in the
// same Bearer header. This lets service accounts use the same management API as
// the web UI while preserving RBAC on the routes that follow.
func AuthAccessTokenAuth(authService ports.AuthService, apiKeyService ports.APIKeyService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}
		token, err := sharedauth.ExtractBearerToken(authHeader)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header"})
			c.Abort()
			return
		}

		claims, err := authService.VerifyToken(c.Request.Context(), token)
		if err != nil {
			apiKey, keyErr := apiKeyService.ValidateAPIKey(c.Request.Context(), token)
			if keyErr != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
				c.Abort()
				return
			}
			c.Set(sharedauth.CtxCompanyID, apiKey.CompanyID)
			if apiKey.ServiceAccountID != nil {
				c.Set("serviceAccountID", *apiKey.ServiceAccountID)
			} else if apiKey.UserID != nil {
				c.Set(sharedauth.CtxUserID, *apiKey.UserID)
			}
			c.Next()
			return
		}

		// Use internal UserID for RBAC role lookups (user_roles table stores by internal ID)
		// Legacy verifier fixtures also resolve roles through the local user ID.
		if !sharedauth.BrowserSessionsEnabled() {
			dbRoles, err := authService.GetUserRoles(c.Request.Context(), claims.UserID, "user")
			if err == nil && len(dbRoles) > 0 {
				claims.Roles = dbRoles
			}
		}

		sharedauth.SetGinContextFromClaims(c, claims)

		c.Next()
	}
}
