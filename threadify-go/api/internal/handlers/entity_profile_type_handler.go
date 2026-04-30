package handlers

import (
	"net/http"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/dto"
	"threadify-go/api/internal/ports"
	"threadify-go/api/internal/validation"
	sharedauth "threadify-go/shared/auth"

	"github.com/gin-gonic/gin"
)

type EntityProfileTypeHandler struct {
	entityProfileTypeService ports.EntityProfileTypeService
}

func NewEntityProfileTypeHandler(entityProfileTypeService ports.EntityProfileTypeService) *EntityProfileTypeHandler {
	return &EntityProfileTypeHandler{
		entityProfileTypeService: entityProfileTypeService,
	}
}

func (h *EntityProfileTypeHandler) CreateEntityProfileType(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	var req dto.CreateEntityProfileTypeRequest

	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateCreateEntityProfileTypeRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	profileType, err := h.entityProfileTypeService.CreateEntityProfileType(c.Request.Context(), compID, &domain.CreateEntityProfileTypeCmd{
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Metrics:     req.Metrics,
	})
	if err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, err.Error())
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Entity profile type created successfully.",
		"data":    profileType,
	})
}

func (h *EntityProfileTypeHandler) ListEntityProfileTypes(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	entityProfileTypes, err := h.entityProfileTypeService.ListEntityProfileTypes(c.Request.Context(), compID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile types fetched successfully.",
		"data":    entityProfileTypes,
	})
}

func (h *EntityProfileTypeHandler) UpdateEntityProfileType(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)

	id := c.Param("id")

	var req dto.UpdateEntityProfileTypeRequest
	if !bindJSON(c, &req) {
		return
	}

	if err := validation.ValidateUpdateEntityProfileTypeRequest(&req); err != nil {
		respondValidationError(c, err)
		return
	}

	profileType, err := h.entityProfileTypeService.UpdateEntityProfileType(c.Request.Context(), compID, id, &domain.UpdateEntityProfileTypeCmd{
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Metrics:     req.Metrics,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile type updated successfully.",
		"data":    profileType,
	})
}

func (h *EntityProfileTypeHandler) ArchiveEntityProfileType(c *gin.Context) {
	companyID, exists := c.Get(sharedauth.CtxCompanyID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	compID := companyID.(string)
	id := c.Param("id")

	err := h.entityProfileTypeService.ArchiveEntityProfileType(c.Request.Context(), compID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile type archived safely.",
	})
}

func (h *EntityProfileTypeHandler) ListMetricsTemplates(c *gin.Context) {
	templates, err := h.entityProfileTypeService.ListMetricsTemplates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Metrics templates fetched successfully.",
		"data":    templates,
	})
}
