package handlers

import (
	"net/http"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
)

type RoleHandler struct {
	rbacLoader *rbac.Loader
}

func NewRoleHandler(rbacLoader *rbac.Loader) *RoleHandler {
	return &RoleHandler{
		rbacLoader: rbacLoader,
	}
}

// GetRoles returns all available roles from roles.json
func (h *RoleHandler) GetRoles(c *gin.Context) {
	roles := h.rbacLoader.GetAllRoles()

	c.JSON(http.StatusOK, gin.H{
		"roles": roles,
	})
}

// GetRolesByLevel returns roles for a specific level (app_level, api_level, or runtime_level)
func (h *RoleHandler) GetRolesByLevel(c *gin.Context) {
	level := c.Param("level")

	if level != "app_level" && level != "api_level" && level != "runtime_level" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid level. Must be 'app_level', 'api_level', or 'runtime_level'",
		})
		return
	}

	roles := h.rbacLoader.GetRolesByLevel(level)

	c.JSON(http.StatusOK, gin.H{
		"level": level,
		"roles": roles,
	})
}
