package dto

import (
	"time"
)

type CustomMetricFilter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CustomMetricDefinition struct {
	Name          string               `json:"name"`
	Target        string               `json:"target"`
	StepName      string               `json:"step_name,omitempty"`
	Operation     string               `json:"operation"`
	Field         string               `json:"field"`
	Filters       []CustomMetricFilter `json:"filters"`
	GroupBy       string               `json:"group_by"`
	Granularity   string               `json:"granularity"`
	Visualisation string               `json:"visualisation"`
}

type EntityTypeMetric struct {
	ID               string                  `json:"id,omitempty"`
	TemplateID       string                  `json:"template_id,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Parameters       map[string]any          `json:"parameters,omitempty"`
	CustomDefinition *CustomMetricDefinition `json:"custom_definition,omitempty"`
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

type ParameterDefinition struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Values      []string `json:"values,omitempty"`
	Description string   `json:"description,omitempty"`
}

type MetricsTemplateResponse struct {
	ID                   string                `json:"id"`
	MetricsName          string                `json:"metrics_name"`
	ParameterDefinitions []ParameterDefinition `json:"parameter_definitions"`
}

type CreateEntityProfileTypeRequest struct {
	Name        string             `json:"name" binding:"required"`
	Type        []string           `json:"type"`
	Description string             `json:"description" binding:"required"`
	Metrics     []EntityTypeMetric `json:"metrics,omitempty"`
}

type UpdateEntityProfileTypeRequest struct {
	Name              string             `json:"name" binding:"required"`
	Type              []string           `json:"type"`
	Description       string             `json:"description"`
	Metrics           []EntityTypeMetric `json:"metrics,omitempty"`
	MarkedForDeletion []string           `json:"marked_for_deletion,omitempty"`
	ModifiedMetricIDs []string           `json:"modified_metric_ids,omitempty"`
}

// ApplyEntityProfileTypeRequest is the canonical, declarative profile type
// payload shared by the UI and config clients.
type ApplyEntityProfileTypeRequest struct {
	Name        string             `json:"name" binding:"required"`
	Type        []string           `json:"type"`
	Description string             `json:"description"`
	Metrics     []EntityTypeMetric `json:"metrics"`
}

type RenameEntityProfileTypeRequest struct {
	Name string `json:"name" binding:"required"`
}

type EntityProfileTypeChanges struct {
	NameChanged        bool     `json:"name_changed"`
	DescriptionChanged bool     `json:"description_changed"`
	TypesChanged       bool     `json:"types_changed"`
	MetricsAdded       []string `json:"metrics_added"`
	MetricsUpdated     []string `json:"metrics_updated"`
	MetricsRemoved     []string `json:"metrics_removed"`
}

type EntityProfileTypeBackfill struct {
	Supported bool `json:"supported"`
	Applied   bool `json:"applied"`
}

type ApplyEntityProfileTypeResponse struct {
	Status     string                    `json:"status"`
	DryRun     bool                      `json:"dry_run"`
	ConfigHash string                    `json:"config_hash"`
	Data       *EntityProfileType        `json:"data"`
	Changes    EntityProfileTypeChanges  `json:"changes"`
	Backfill   EntityProfileTypeBackfill `json:"backfill"`
}
