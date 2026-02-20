package middleware

import (
	"fmt"
	"net/http"

	"threadify-go/shared/jwt"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
	jwtgo "github.com/golang-jwt/jwt/v5"
)

// ContractJWTMiddleware validates JWT tokens for contract endpoints
func ContractJWTMiddleware(jwtValidator *jwt.Validator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		claims, err := jwtValidator.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Set user context
		c.Set("userID", claims.UserID)
		c.Set("companyID", claims.CompanyID)
		c.Set("email", claims.Email)
		c.Set("roles", claims.Roles)

		// Also set claims map for contract handler compatibility
		claimsMap := jwtgo.MapClaims{
			"ownerId":   claims.UserID,
			"companyId": claims.CompanyID,
			"email":     claims.Email,
			"roles":     claims.Roles,
		}
		c.Set("claims", claimsMap)
		c.Next()
	}
}

// ContractRBACMiddleware checks RBAC permissions for contract operations
// The permission parameter can include :id placeholder which will be replaced with the actual contract ID from the URL
func ContractRBACMiddleware(rbacLoader *rbac.Loader, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get("userID")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		roles, exists := c.Get("roles")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "No roles assigned"})
			c.Abort()
			return
		}

		roleList, ok := roles.([]string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid roles format"})
			c.Abort()
			return
		}

		// Replace :id placeholder with actual contract ID from URL if present
		actualPermission := permission
		if contractID := c.Param("id"); contractID != "" {
			// For resource-specific permissions like contract.read.<contract_id>
			// Replace wildcard with actual ID
			actualPermission = fmt.Sprintf("contract.%s.%s",
				getOperationFromPath(c.Request.Method),
				contractID)
		}

		// Check if user has permission through any of their roles
		hasPermission := false
		for _, roleName := range roleList {
			// Get role from app_level (user roles)
			appLevelRoles := rbacLoader.GetRolesByLevel("app_level")
			if role, exists := appLevelRoles[roleName]; exists {
				// Check if role has the required permission
				if rbacLoader.CheckPermission(role.Permissions, actualPermission) {
					hasPermission = true
					break
				}
				// Also check against the wildcard permission
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

// Helper function to extract operation from HTTP method
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
