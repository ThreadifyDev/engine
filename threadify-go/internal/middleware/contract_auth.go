package middleware

import (
	"fmt"
	"net/http"

	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/threadify/engine/internal/service"
)

const (
	userRole = "user"
)

func ContractDualAuthMiddleware(authService *service.AuthService) gin.HandlerFunc {
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

		dbRoles, err := authService.GetUserRoles(c.Request.Context(), claims.UserID, userRole)
		if err != nil {
		}
		if len(dbRoles) > 0 {
			claims.Roles = dbRoles
		}

		sharedauth.SetGinContextFromClaims(c, claims)
		claimsMap := jwt.MapClaims{
			sharedauth.OwnerID:      claims.UserID,
			sharedauth.CtxCompanyID: claims.CompanyID,
			sharedauth.CtxEmail:     claims.Email,
			sharedauth.CtxRoles:     claims.Roles,
		}
		c.Set(sharedauth.CtxClaims, claimsMap)
		c.Next()
	}
}

func ContractRBACMiddleware(rbacLoader *rbac.Loader, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get(sharedauth.CtxUserID)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		rolesRaw, exists := c.Get(sharedauth.CtxRoles)
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "No roles assigned"})
			c.Abort()
			return
		}

		roleList, ok := rolesRaw.([]string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid roles format"})
			c.Abort()
			return
		}

		// Replace :id placeholder with actual contract ID from URL if present
		actualPermission := permission
		if contractID := c.Param("id"); contractID != "" {
			actualPermission = fmt.Sprintf("contract.%s.%s",
				getOperationFromPath(c.Request.Method),
				contractID)
		}

		hasPermission := false
		for _, roleName := range roleList {
			appLevelRoles := rbacLoader.GetRolesByLevel("app_level")
			if role, exists := appLevelRoles[roleName]; exists {
				if rbacLoader.CheckPermission(role.Permissions, actualPermission) {
					hasPermission = true
					break
				}
				if rbacLoader.CheckPermission(role.Permissions, permission) {
					hasPermission = true
					break
				}
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient permissions",
				"required": permission,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func getOperationFromPath(method string) string {
	switch method {
	case "GET":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "read"
	}
}
