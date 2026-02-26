package middleware

import (
	"net/http"
	"strings"

	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/service"
)

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

		setAPIKeyContext(c, userInfo)
		c.Next()
	}
}

func DualAuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Either X-API-Key or Authorization header required"})
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

		// Resolve application-level roles from database
		dbRoles, err := authService.GetUserRoles(c.Request.Context(), claims.UserID, "user")
		if err == nil && len(dbRoles) > 0 {
			claims.Roles = dbRoles
		}

		sharedauth.SetGinContextFromClaims(c, claims)
		c.Next()
	}
}

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
