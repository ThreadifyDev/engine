package domain

import "time"

type EntityTypeMetric struct {
	TemplateID string
	Name       string
	Parameters map[string]any
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

	TypesToAdd []string
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
