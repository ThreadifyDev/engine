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
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("metric name is required")
	}

	validFields := map[string]map[string]bool{
		"COUNT": {"threads": true, "steps": true, "violations": true, "retries": true, "stepCount": true},
		"RATE":  {"outcome": true, "violations": true},
		"AVG":   {"duration": true, "retries": true},
		"SUM":   {"violations": true, "retries": true, "duration": true, "stepCount": true},
		"MIN":   {"duration": true},
		"MAX":   {"duration": true, "retries": true},
	}
	fields, ok := validFields[m.Operation]
	if !ok {
		return fmt.Errorf("unsupported operation: %s", m.Operation)
	}
	if !fields[m.Field] {
		return fmt.Errorf("field %s is not supported for operation %s", m.Field, m.Operation)
	}

	switch m.Target {
	case "thread":
		if m.Field == "steps" || m.Field == "retries" || m.Field == "stepCount" {
			return fmt.Errorf("field %s requires target step", m.Field)
		}
		if m.GroupBy == "step name" || m.GroupBy == "actor service" {
			return fmt.Errorf("cannot group by %s when target is thread", m.GroupBy)
		}
	case "step":
		if m.Field == "threads" {
			return fmt.Errorf("field threads requires target thread")
		}
	default:
		return fmt.Errorf("target must be thread or step")
	}

	validGroups := map[string]bool{
		"": true, "none": true, "period": true, "step name": true,
		"outcome": true, "actor": true, "actor service": true,
		"process type": true, "violation type": true,
		"validation severity": true, "tag": true,
	}
	if !validGroups[m.GroupBy] {
		return fmt.Errorf("unsupported group by: %s", m.GroupBy)
	}
	if m.Operation == "RATE" && m.Field == "violations" && (m.GroupBy == "violation type" || m.GroupBy == "validation severity") {
		return fmt.Errorf("violation rates cannot be grouped by %s; use a filter or COUNT violations instead", m.GroupBy)
	}

	validFilters := map[string]bool{
		"step name": true, "step outcome": true, "actor": true,
		"actor service": true, "thread outcome": true, "process type": true,
		"violation type": true, "validation severity": true, "tags": true,
	}
	for _, f := range m.Filters {
		key := strings.ToLower(strings.TrimSpace(f.Key))
		if key == "" || strings.TrimSpace(f.Value) == "" {
			return fmt.Errorf("filter key and value are required")
		}
		if key == "tags" {
			hasTag := false
			for _, tag := range strings.Split(f.Value, ",") {
				if strings.TrimSpace(tag) != "" {
					hasTag = true
					break
				}
			}
			if !hasTag {
				return fmt.Errorf("tags filter requires at least one tag")
			}
		}
		if !validFilters[key] {
			return fmt.Errorf("unsupported filter: %s", key)
		}
		if m.Target == "thread" && (key == "step name" || key == "step outcome" || key == "actor service") {
			return fmt.Errorf("cannot filter by %s when target is thread", key)
		}
	}

	if m.GroupBy == "period" && m.Granularity == "" {
		return fmt.Errorf("granularity is required when grouping by period")
	}
	if m.Granularity != "" {
		validGranularities := map[string]bool{"hour": true, "day": true, "week": true, "month": true}
		if !validGranularities[m.Granularity] {
			return fmt.Errorf("unsupported granularity: %s", m.Granularity)
		}
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
