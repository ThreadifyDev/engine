package middleware

import (
	"net/http"

	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/domain"
)

type AuthMode int

const (
	AuthAPIKey AuthMode = 1 << iota
	AuthJWT
	AuthDual = AuthAPIKey | AuthJWT
)

func (m AuthMode) has(flag AuthMode) bool { return m&flag != 0 }

func AuthMiddleware(authSvc domain.AuthService, mode AuthMode) gin.HandlerFunc {
	return func(c *gin.Context) {
		if mode.has(AuthAPIKey) {
			if apiKey := c.GetHeader("X-API-Key"); apiKey != "" {
				userInfo, err := authSvc.ValidateApiKey(apiKey)
				if err != nil {
					abort(c, "invalid API key")
					return
				}
				setAPIKeyContext(c, userInfo)
				c.Next()
				return
			}
		}

		if mode.has(AuthJWT) {
			if authHeader := c.GetHeader("Authorization"); authHeader != "" {
				token, err := sharedauth.ExtractBearerToken(authHeader)
				if err != nil {
					abort(c, err.Error())
					return
				}

				claims, err := authSvc.VerifyToken(c.Request.Context(), token)
				if err != nil {
					abort(c, err.Error())
					return
				}

				if dbRoles, err := authSvc.GetUserRoles(c.Request.Context(), claims.UserID, "user", claims.ExpiresAt); err == nil && len(dbRoles) > 0 {
					claims.Roles = dbRoles
				}

				sharedauth.SetGinContextFromClaims(c, claims)
				c.Next()
				return
			}
		}

		abort(c, unauthorizedMessage(mode))
	}
}

func abort(c *gin.Context, msg string) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": msg})
	c.Abort()
}

func unauthorizedMessage(mode AuthMode) string {
	switch mode {
	case AuthAPIKey:
		return "X-API-Key header required"
	case AuthJWT:
		return "Authorization header required"
	default:
		return "either X-API-Key or Authorization header required"
	}
}

func setAPIKeyContext(c *gin.Context, userInfo *domain.UserInfo) {
	roles := []string{}
	if userInfo.Role != "" {
		roles = []string{userInfo.Role}
	}
	c.Set(sharedauth.CtxUserID, userInfo.OwnerID)
	c.Set(sharedauth.CtxCompanyID, userInfo.CompanyID)
	c.Set(sharedauth.CtxRoles, roles)
}
