package domain

import (
	"fmt"
	"strings"
	"time"
)

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

func (m *MetricDefinition) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("metric name is required")
	}
	if m.Target == "thread" {
		if m.GroupBy == "step name" || m.GroupBy == "outcome" || m.GroupBy == "actor service" {
			return fmt.Errorf("cannot group by %s when target is thread", m.GroupBy)
		}
		for _, f := range m.Filters {
			key := strings.ToLower(strings.TrimSpace(f.Key))
			if key == "step name" || key == "step outcome" || key == "actor" || key == "actor service" {
				return fmt.Errorf("cannot filter by %s when target is thread", key)
			}
		}
	}
	if m.GroupBy == "period" && m.Granularity == "" {
		return fmt.Errorf("granularity is required when grouping by period")
	}
	return nil
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
