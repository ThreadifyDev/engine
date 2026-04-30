package dto

import sharedmodels "threadify-go/shared/domain"

type CreateEntityProfileTypeRequest struct {
	Name        string                          `json:"name" binding:"required"`
	Type        []string                        `json:"type"`
	Description string                          `json:"description" binding:"required"`
	Metrics     []sharedmodels.EntityTypeMetric `json:"metrics,omitempty"`
}

type UpdateEntityProfileTypeRequest struct {
	Name        string                          `json:"name" binding:"required"`
	Type        []string                        `json:"type"`
	Description string                          `json:"description"`
	Metrics     []sharedmodels.EntityTypeMetric `json:"metrics,omitempty"`
}
