package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/rbac"
)

type ProfileViewStore interface {
	Get(context.Context, string, string) (*domain.SavedProfileView, error)
	Save(context.Context, string, string, string, int64, domain.ProfileViewDefinition) (*domain.SavedProfileView, error)
}
type ProfileViewHandler struct {
	store ProfileViewStore
	roles *rbac.Loader
}

func NewProfileViewHandler(store ProfileViewStore, roles *rbac.Loader) *ProfileViewHandler {
	return &ProfileViewHandler{store: store, roles: roles}
}
func (h *ProfileViewHandler) permitted(c *gin.Context, permission string) bool {
	if h.roles == nil {
		return false
	}
	raw, _ := c.Get(sharedauth.CtxRoles)
	assigned, _ := raw.([]string)
	for _, level := range []string{"app_level", "api_level"} {
		available := h.roles.GetRolesByLevel(level)
		for _, name := range assigned {
			if role, ok := available[name]; ok && h.roles.CheckPermission(role.Permissions, permission) {
				return true
			}
		}
	}
	return false
}
func (h *ProfileViewHandler) authorize(c *gin.Context, permission string) bool {
	c.Header("Cache-Control", "no-store")
	if c.GetString(sharedauth.CtxCompanyID) == "" || c.GetString(sharedauth.CtxUserID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return false
	}
	if !h.permitted(c, permission) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions", "required": permission})
		return false
	}
	if _, err := uuid.Parse(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid profile type ID required"})
		return false
	}
	return true
}
func profileViewError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrProfileMetricBinding):
		c.JSON(400, gin.H{"error": "A referenced metric is no longer configured for this profile type. Update the presentation before saving."})
	case errors.Is(err, serror.ErrEntityProfileTypeNotFound):
		c.JSON(404, gin.H{"error": "profile type not found"})
	case errors.Is(err, domain.ErrProfileViewConflict):
		c.JSON(409, gin.H{"error": "This profile view changed since you opened it. Load the latest shared view before saving.", "code": "PROFILE_VIEW_CONFLICT"})
	default:
		c.JSON(500, gin.H{"error": "Could not access the profile view. Please try again."})
	}
}
func (h *ProfileViewHandler) Get(c *gin.Context) {
	if !h.authorize(c, "entity_profile_type.read") {
		return
	}
	view, err := h.store.Get(c.Request.Context(), c.GetString(sharedauth.CtxCompanyID), c.Param("id"))
	if err != nil {
		profileViewError(c, err)
		return
	}
	c.JSON(200, gin.H{"data": view, "can_manage": h.permitted(c, "entity_profile_type.update")})
}
func (h *ProfileViewHandler) Save(c *gin.Context) {
	if !h.authorize(c, "entity_profile_type.update") {
		return
	}
	var input struct {
		ExpectedRevision *int64                        `json:"expected_revision"`
		Definition       *domain.ProfileViewDefinition `json:"definition"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		c.JSON(400, gin.H{"error": "Expected a profile view definition and expected_revision, without extra fields (maximum 32 KB)."})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(400, gin.H{"error": "Expected one JSON object."})
		return
	}
	if input.ExpectedRevision == nil || *input.ExpectedRevision < 0 || *input.ExpectedRevision >= 9007199254740991 {
		c.JSON(400, gin.H{"error": "expected_revision must be a nonnegative safe integer"})
		return
	}
	if err := input.Definition.Validate(); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	view, err := h.store.Save(c.Request.Context(), c.GetString(sharedauth.CtxCompanyID), c.Param("id"), c.GetString(sharedauth.CtxUserID), *input.ExpectedRevision, *input.Definition)
	if err != nil {
		profileViewError(c, err)
		return
	}
	c.JSON(200, gin.H{"data": view, "can_manage": true})
}
