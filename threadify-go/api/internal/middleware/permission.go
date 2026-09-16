package middleware

import (
	"net/http"
	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/ports"

	"github.com/gin-gonic/gin"
)

// RequirePermission checks if the authenticated user/service account has the required permission
// Uses JSON-based permission loader for fast lookups
func RequirePermission(
	permLoader ports.PermissionLoader,
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
	required string, // e.g., "contract.read.*" or "thread.write"
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		serviceAccountID := c.GetString("serviceAccountID")

		// Check authentication
		if userID == "" && serviceAccountID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		var permissions []string

		if userID != "" {
			// User authentication - load roles from DB using internal userID
			roleNames, err := userRoleRepo.GetUserRoles(c.Request.Context(), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user roles"})
				c.Abort()
				return
			}

			// Default to standard role if no roles assigned
			if len(roleNames) == 0 {
				roleNames = []string{"standard"}
			}

			// Get permissions from JSON config
			permissions = permLoader.GetPermissionsForRoles(roleNames, "app_level")
		} else {
			// Service account authentication
			sa, err := serviceAccountRepo.FindByID(c.Request.Context(), serviceAccountID)
			if err != nil || sa == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Service account not found"})
				c.Abort()
				return
			}

			if !sa.IsActive {
				c.JSON(http.StatusForbidden, gin.H{"error": "Service account is inactive"})
				c.Abort()
				return
			}

			// Load roles from DB and get permissions from JSON
			roleNames, err := userRoleRepo.GetServiceAccountRoles(c.Request.Context(), serviceAccountID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load service account roles"})
				c.Abort()
				return
			}

			// Get permissions from JSON config
			permissions = permLoader.GetPermissionsForRoles(roleNames, "runtime_level")

			// TODO: Move last_used_at tracking to NATS for async processing
			// Currently commented out - spawns unbounded goroutines and data not actively used
			// go serviceAccountRepo.UpdateLastUsed(serviceAccountID)
		}

		// Check permission with wildcard matching
		hasPermission := permLoader.CheckPermission(permissions, required)
		if !hasPermission {
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
// For example: "contract.read" with resourceParam="id" will check:
// 1. contract.read.* (wildcard)
// 2. contract.read.<actual_id_from_url> (specific resource)
func RequireResourcePermission(
	permLoader ports.PermissionLoader,
	serviceAccountRepo domain.ServiceAccountRepository,
	userRoleRepo domain.UserRoleRepository,
	action string, // e.g., "contract.read", "contract.update"
	resourceParam string, // URL parameter name, e.g., "id"
) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		serviceAccountID := c.GetString("serviceAccountID")

		// Check authentication
		if userID == "" && serviceAccountID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			c.Abort()
			return
		}

		var permissions []string

		if userID != "" {
			// User authentication - load roles from DB using internal userID
			roleNames, err := userRoleRepo.GetUserRoles(c.Request.Context(), userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load user roles"})
				c.Abort()
				return
			}

			if len(roleNames) == 0 {
				roleNames = []string{"standard"}
			}

			permissions = permLoader.GetPermissionsForRoles(roleNames, "app_level")
		} else {
			// Service account authentication
			sa, err := serviceAccountRepo.FindByID(c.Request.Context(), serviceAccountID)
			if err != nil || sa == nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Service account not found"})
				c.Abort()
				return
			}

			if !sa.IsActive {
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

			permissions = permLoader.GetPermissionsForRoles(roleNames, "runtime_level")
			// TODO: Move last_used_at tracking to NATS for async processing
			// Currently commented out - spawns unbounded goroutines and data not actively used
			// go serviceAccountRepo.UpdateLastUsed(serviceAccountID)
		}

		// Check wildcard permission first (e.g., "contract.read.*")
		wildcardPerm := action + ".*"
		if permLoader.CheckPermission(permissions, wildcardPerm) {
			c.Next()
			return
		}

		// Check resource-specific permission (e.g., "contract.read.<contract_id>")
		resourceID := c.Param(resourceParam)
		if resourceID != "" {
			specificPerm := action + "." + resourceID
			if permLoader.CheckPermission(permissions, specificPerm) {
				c.Next()
				return
			}
		}

		// No permission found
		c.JSON(http.StatusForbidden, gin.H{
			"error":    "Insufficient permissions",
			"required": action + ".*" + " or " + action + ".<resource_id>",
		})
		c.Abort()
	}
}
