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

	metrics := make([]domain.EntityTypeMetric, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		metrics = append(metrics, domain.EntityTypeMetric{
			ID:               m.ID,
			TemplateID:       m.TemplateID,
			Name:             m.Name,
			Parameters:       m.Parameters,
			CustomDefinition: toDomainMetricDefinition(m.CustomDefinition),
		})
	}

	profileType, err := h.entityProfileTypeService.CreateEntityProfileType(c.Request.Context(), compID, &domain.CreateEntityProfileTypeCmd{
		Name:        req.Name,
		Type:        req.Type,
		Description: req.Description,
		Metrics:     metrics,
	})
	if err != nil {
		if respondValidationError(c, err) {
			return
		}
		statusCode, message := authErrorResponse(err, http.StatusInternalServerError, "An internal error occurred.")
		c.JSON(statusCode, gin.H{"error": message})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Entity profile type created successfully.",
		"data":    toEntityProfileTypeDTO(profileType),
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

	out := make([]*dto.EntityProfileType, 0, len(entityProfileTypes))
	for _, t := range entityProfileTypes {
		out = append(out, toEntityProfileTypeDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile types fetched successfully.",
		"data":    out,
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

	metrics := make([]domain.EntityTypeMetric, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		metrics = append(metrics, domain.EntityTypeMetric{
			ID:               m.ID,
			TemplateID:       m.TemplateID,
			Name:             m.Name,
			Parameters:       m.Parameters,
			CustomDefinition: toDomainMetricDefinition(m.CustomDefinition),
		})
	}

	profileType, err := h.entityProfileTypeService.UpdateEntityProfileType(c.Request.Context(), compID, id, &domain.UpdateEntityProfileTypeCmd{
		Name:              req.Name,
		Type:              req.Type,
		Description:       req.Description,
		Metrics:           metrics,
		MarkedForDeletion: req.MarkedForDeletion,
		ModifiedMetricIDs: req.ModifiedMetricIDs,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "An internal error occurred."})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Entity profile type updated successfully.",
		"data":    toEntityProfileTypeDTO(profileType),
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

	out := make([]*dto.MetricsTemplateResponse, 0, len(templates))
	for _, t := range templates {
		out = append(out, toMetricsTemplateDTO(t))
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Metrics templates fetched successfully.",
		"data":    out,
	})
}

func toEntityProfileTypeDTO(t *domain.EntityProfileType) *dto.EntityProfileType {
	if t == nil {
		return nil
	}

	metrics := make([]dto.EntityTypeMetric, 0, len(t.Metrics))
	for _, m := range t.Metrics {
		metrics = append(metrics, dto.EntityTypeMetric{
			ID:               m.ID,
			TemplateID:       m.TemplateID,
			Name:             m.Name,
			Parameters:       m.Parameters,
			CustomDefinition: toDTOCustomDefinition(m.CustomDefinition),
		})
	}

	return &dto.EntityProfileType{
		ID:          t.ID,
		CompanyID:   t.CompanyID,
		Name:        t.Name,
		Slug:        t.Slug,
		Type:        t.Type,
		Description: t.Description,
		ArchivedAt:  t.ArchivedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		Metrics:     metrics,
	}
}

func toDTOCustomDefinition(d *domain.MetricDefinition) *dto.CustomMetricDefinition {
	if d == nil {
		return nil
	}
	filters := make([]dto.CustomMetricFilter, len(d.Filters))
	for i, f := range d.Filters {
		filters[i] = dto.CustomMetricFilter{
			Key:   f.Key,
			Value: f.Value,
		}
	}
	return &dto.CustomMetricDefinition{
		Name:        d.Name,
		Target:      d.Target,
		StepName:    d.StepName,
		Operation:   d.Operation,
		Field:       d.Field,
		Filters:     filters,
		GroupBy:     d.GroupBy,
		Granularity: d.Granularity,
		// Visualisation: d.Visualisation, // commented out for now
	}
}

func toDomainMetricDefinition(d *dto.CustomMetricDefinition) *domain.MetricDefinition {
	if d == nil {
		return nil
	}
	filters := make([]domain.MetricFilter, len(d.Filters))
	for i, f := range d.Filters {
		filters[i] = domain.MetricFilter{
			Key:   f.Key,
			Value: f.Value,
		}
	}
	return &domain.MetricDefinition{
		Name:        d.Name,
		Target:      d.Target,
		StepName:    d.StepName,
		Operation:   d.Operation,
		Field:       d.Field,
		Filters:     filters,
		GroupBy:     d.GroupBy,
		Granularity: d.Granularity,
		// Visualisation: d.Visualisation, // commented out for now
	}
}

func toMetricsTemplateDTO(t *domain.MetricsTemplate) *dto.MetricsTemplateResponse {
	if t == nil {
		return nil
	}
	defs := make([]dto.ParameterDefinition, len(t.ParameterDefinitions))
	for i, d := range t.ParameterDefinitions {
		defs[i] = dto.ParameterDefinition{
			Name:        d.Name,
			Type:        d.Type,
			Values:      d.Values,
			Description: d.Description,
		}
	}
	return &dto.MetricsTemplateResponse{
		ID:                   t.ID,
		MetricsName:          t.MetricsName,
		ParameterDefinitions: defs,
	}
}
