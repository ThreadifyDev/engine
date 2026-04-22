package rbac

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// UserRoleGetter interface for getting user roles from database
type UserRoleGetter interface {
	GetUserRoles(ctx context.Context, userID string) ([]string, error)
	GetServiceAccountRoles(ctx context.Context, serviceAccountID string) ([]string, error)
}

// ServiceAccountChecker interface for checking service account status
type ServiceAccountChecker interface {
	IsServiceAccountActive(ctx context.Context, id string) (bool, error)
	UpdateLastUsed(ctx context.Context, id string) error
}

// RequirePermission checks if the authenticated user/service account has the required permission
func RequirePermission(
	loader *Loader,
	userRoleRepo UserRoleGetter,
	serviceAccountChecker ServiceAccountChecker,
	required string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		serviceAccountID := c.GetString("serviceAccountID")

		if userID == "" && serviceAccountID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		var permissions []string

		if userID != "" {
			roleNames, err := userRoleRepo.GetUserRoles(c.Request.Context(), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user roles"})
				c.Abort()
				return
			}

			if len(roleNames) == 0 {
				roleNames = []string{"member"}
			}

			permissions = loader.GetPermissionsForRoles(roleNames, "app_level")
		} else {
			isActive, err := serviceAccountChecker.IsServiceAccountActive(c.Request.Context(), serviceAccountID)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Service account not found"})
				c.Abort()
				return
			}

			if !isActive {
				c.JSON(http.StatusForbidden, gin.H{"error": "Service account is inactive"})
				c.Abort()
				return
			}

			roleNames, err := userRoleRepo.GetServiceAccountRoles(c.Request.Context(), serviceAccountID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load service account roles"})
				c.Abort()
				return
			}

			permissions = loader.GetPermissionsForRoles(roleNames, "app_level")
			// TODO: Move last_used_at tracking to NATS for async processing
			// Currently commented out - spawns unbounded goroutines and data not actively used
			// go serviceAccountChecker.UpdateLastUsed(serviceAccountID)
		}

		if !loader.CheckPermission(permissions, required) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient permissions",
				"required": required,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireResourcePermission checks permission with support for dynamic resource IDs
func RequireResourcePermission(
	loader *Loader,
	userRoleRepo UserRoleGetter,
	serviceAccountChecker ServiceAccountChecker,
	action string,
	resourceParam string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		serviceAccountID := c.GetString("serviceAccountID")

		if userID == "" && serviceAccountID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		var permissions []string

		if userID != "" {
			roleNames, err := userRoleRepo.GetUserRoles(c.Request.Context(), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user roles"})
				c.Abort()
				return
			}

			if len(roleNames) == 0 {
				roleNames = []string{"member"}
			}

			permissions = loader.GetPermissionsForRoles(roleNames, "app_level")
		} else {
			isActive, err := serviceAccountChecker.IsServiceAccountActive(c.Request.Context(), serviceAccountID)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Service account not found"})
				c.Abort()
				return
			}

			if !isActive {
				c.JSON(http.StatusForbidden, gin.H{"error": "Service account is inactive"})
				c.Abort()
				return
			}

			roleNames, err := userRoleRepo.GetServiceAccountRoles(c.Request.Context(), serviceAccountID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load service account roles"})
				c.Abort()
				return
			}

			permissions = loader.GetPermissionsForRoles(roleNames, "app_level")
			// TODO: Move last_used_at tracking to NATS for async processing
			// Currently commented out - spawns unbounded goroutines and data not actively used
			// go serviceAccountChecker.UpdateLastUsed(serviceAccountID)
		}

		// Check wildcard permission first
		wildcardPerm := action + ".*"
		if loader.CheckPermission(permissions, wildcardPerm) {
			c.Next()
			return
		}

		// Check resource-specific permission
		resourceID := c.Param(resourceParam)
		if resourceID != "" {
			specificPerm := action + "." + resourceID
			if loader.CheckPermission(permissions, specificPerm) {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error":    "Insufficient permissions",
			"required": action + ".*" + " or " + action + ".<resource_id>",
		})
		c.Abort()
	}
}
