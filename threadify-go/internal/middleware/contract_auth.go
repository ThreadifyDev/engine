package middleware

import (
	"fmt"
	"net/http"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/domain"
)

func ContractRBACMiddleware(rbacLoader domain.RBACLoader, permission string) gin.HandlerFunc {
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
