package middleware

import (
	"net/http"
	"threadify-go/shared/registry"

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
				if err := registry.Default().CheckCompany(userInfo.CompanyID); err != nil {
					abort(c, err.Error())
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

				if !sharedauth.BrowserSessionsEnabled() {
					if dbRoles, err := authSvc.GetUserRoles(c.Request.Context(), claims.UserID, "user", claims.ExpiresAt); err == nil && len(dbRoles) > 0 {
						claims.Roles = dbRoles
					}
				}

				if claims.OAuthAccess {
					path := c.FullPath()
					if c.Request.Method != http.MethodGet || (path != "/v1/contracts" && path != "/v1/contracts/:id") {
						abort(c, "OAuth scope does not permit this route")
						return
					}
					permission := "contract.read.*"
					if path == "/v1/contracts/:id" {
						permission = "contract.read." + c.Param("id")
					}
					if !sharedauth.OAuthCanUse(c.Request.Context(), claims, permission) {
						abort(c, "OAuth scope denied")
						return
					}
				}

				if err := registry.Default().CheckCompany(claims.CompanyID); err != nil {
					abort(c, err.Error())
					return
				}
				sharedauth.SetGinContextFromClaims(c, claims)
				// Engine thread operations address both human and service principals by owner ID.
				c.Set(sharedauth.CtxUserID, claims.UserID)
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
	roles := userInfo.Roles
	if len(roles) == 0 && userInfo.Role != "" {
		roles = []string{userInfo.Role}
	}
	c.Set(sharedauth.CtxUserID, userInfo.OwnerID)
	c.Set(sharedauth.CtxCompanyID, userInfo.CompanyID)
	c.Set(sharedauth.CtxRoles, roles)
}
