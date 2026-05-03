package dto

import (
	"time"
)

type EntityTypeMetric struct {
	TemplateID string         `json:"template_id"`
	Name       string         `json:"name,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type EntityProfileType struct {
	ID          string             `json:"id"`
	CompanyID   string             `json:"company_id"`
	Name        string             `json:"name"`
	Slug        string             `json:"slug"`
	Type        []string           `json:"type"`
	Description string             `json:"description,omitempty"`
	ArchivedAt  *time.Time         `json:"archived_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
	Metrics     []EntityTypeMetric `json:"metrics,omitempty"`
}

type MetricsTemplateResponse struct {
	ID          string   `json:"id"`
	MetricsName string   `json:"metrics_name"`
	Parameters  []string `json:"parameters"`
	SQLContent  string   `json:"sql_content"`
}

type CreateEntityProfileTypeRequest struct {
	Name        string             `json:"name" binding:"required"`
	Type        []string           `json:"type"`
	Description string             `json:"description" binding:"required"`
	Metrics     []EntityTypeMetric `json:"metrics,omitempty"`
}

type UpdateEntityProfileTypeRequest struct {
	Name        string             `json:"name" binding:"required"`
	Type        []string           `json:"type"`
	Description string             `json:"description"`
	Metrics     []EntityTypeMetric `json:"metrics,omitempty"`
}
