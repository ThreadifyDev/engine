package domain

import "time"

type MetricFilter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MetricDefinition struct {
	Name          string
	Target        string
	StepName      string
	Operation     string
	Field         string
	Filters       []MetricFilter
	GroupBy       string
	Granularity   string
	Visualisation string
}

type EntityTypeMetric struct {
	ID               string
	TemplateID       string
	Name             string
	Parameters       map[string]any
	CustomDefinition *MetricDefinition
}

type ParameterDef struct {
	Name        string
	Type        string
	Values      []string
	Description string
}

type MetricsTemplate struct {
	ID                   string
	MetricsName          string
	ParameterDefinitions []ParameterDef
	SQLContent           string
}

type EntityProfileType struct {
	ID          string
	CompanyID   string
	Name        string
	Slug        string
	Type        []string
	Description string
	ArchivedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Metrics []EntityTypeMetric

	TypesToAdd        []string
	MarkedForDeletion []string
	ModifiedMetricIDs []string
}

type EntityProfile struct {
	ID            string
	RefKey        string
	CompanyID     string
	ProfileTypeID string
	Name          string
	CreatedAt     time.Time
	LastActiveAt  time.Time
}
