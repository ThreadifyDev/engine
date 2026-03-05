package middleware

import (
	"net/http"
	"strings"

	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/service"
)

// AuthMode controls which credential types are accepted by AuthMiddleware.
type AuthMode int

const (
	// AuthAPIKey accepts X-API-Key only.
	AuthAPIKey AuthMode = 1 << iota
	// AuthJWT accepts Bearer JWT only.
	AuthJWT

	// AuthDual accepts either X-API-Key or Bearer JWT (most common).
	AuthDual = AuthAPIKey | AuthJWT
)

// AuthMiddleware is the single, unified authentication middleware for the Engine.
//
// Usage:
//
//	middleware.AuthMiddleware(authSvc, middleware.AuthAPIKey)   // API key only (MCP)
//	middleware.AuthMiddleware(authSvc, middleware.AuthJWT)      // JWT only
//	middleware.AuthMiddleware(authSvc, middleware.AuthDual)     // either (GraphQL, contracts)
func AuthMiddleware(authService *service.AuthService, mode AuthMode) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── API Key path ──────────────────────────────────────────────────────
		if mode&AuthAPIKey != 0 {
			if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
				userInfo, err := authService.ValidateApiKey(apiKey)
				if err != nil {
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid API key"})
					c.Abort()
					return
				}
				setAPIKeyContext(c, userInfo)
				c.Next()
				return
			}
		}

		// ── JWT path ──────────────────────────────────────────────────────────
		if mode&AuthJWT != 0 {
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" {
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

				// Resolve application-level roles from database, capped to JWT lifetime
				if dbRoles, err := authService.GetUserRoles(c.Request.Context(), claims.UserID, "user", claims.ExpiresAt); err == nil && len(dbRoles) > 0 {
					claims.Roles = dbRoles
				}

				sharedauth.SetGinContextFromClaims(c, claims)
				c.Next()
				return
			}
		}

		// ── Nothing matched — build a clear error message ─────────────────────
		c.JSON(http.StatusUnauthorized, gin.H{"error": unauthorizedMessage(mode)})
		c.Abort()
	}
}

func unauthorizedMessage(mode AuthMode) string {
	switch mode {
	case AuthAPIKey:
		return "X-API-Key header required"
	case AuthJWT:
		return "Authorization header required"
	default:
		return "Either X-API-Key or Authorization header required"
	}
}

// setAPIKeyContext populates the Gin context with identity fields from a
// validated API key, matching the same keys used by the JWT path.
func setAPIKeyContext(c *gin.Context, userInfo *service.UserInfo) {
	roles := []string{}
	if userInfo.Role != "" {
		roles = []string{userInfo.Role}
	}

	c.Set(sharedauth.CtxUserID, userInfo.OwnerID)
	c.Set(sharedauth.CtxCompanyID, userInfo.CompanyID)
	c.Set(sharedauth.CtxRoles, roles)
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
