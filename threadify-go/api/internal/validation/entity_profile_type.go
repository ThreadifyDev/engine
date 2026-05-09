package validation

import (
	"fmt"
	"slices"
	"strings"
	"threadify-go/api/internal/dto"
)

const maxTypes = 5
const maxDescriptionLen = 255

func ValidateCreateEntityProfileTypeRequest(req *dto.CreateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if req == nil {
		b.add("request", "Request body is required")
		return b.err()
	}

	if strings.TrimSpace(req.Name) == "" {
		b.add("name", "Name is required.")
	}

	if len(req.Description) > maxDescriptionLen {
		b.add("description", fmt.Sprintf("Description cannot exceed %d characters.", maxDescriptionLen))
	}

	hasTypes := false
	for _, t := range req.Type {
		if strings.TrimSpace(t) != "" {
			hasTypes = true
			break
		}
	}

	if !hasTypes {
		b.add("type", "At least one type is required.")
	}

	if len(req.Type) > maxTypes {
		b.add("type", fmt.Sprintf("Types cannot exceed %d values.", maxTypes))
	}

	for i, m := range req.Metrics {
		validateCustomMetricDefinition(b, m.CustomDefinition, fmt.Sprintf("metrics[%d].custom_definition", i))
	}

	return b.err()
}

func ValidateUpdateEntityProfileTypeRequest(req *dto.UpdateEntityProfileTypeRequest) error {
	b := &validationBuilder{}

	if len(req.Description) > maxDescriptionLen {
		b.add("description", fmt.Sprintf("Description cannot exceed %d characters.", maxDescriptionLen))
	}

	if len(req.Type) > maxTypes {
		b.add("type", fmt.Sprintf("Types cannot exceed %d values.", maxTypes))
	}

	for i, m := range req.Metrics {
		validateCustomMetricDefinition(b, m.CustomDefinition, fmt.Sprintf("metrics[%d].custom_definition", i))
	}

	return b.err()
}

var validTargets = []string{"thread", "step"}
var validOperations = []string{"COUNT", "RATE", "AVG", "SUM", "MIN", "MAX"}
var validFields = []string{"threads", "steps", "violations", "retries", "stepCount", "outcome", "duration"}
var validGroupBy = []string{"step name", "outcome", "process type", "violation type", "period", "none"}
var validGranularity = []string{"hour", "day", "week", "month"}
var validVisualisation = []string{"number", "line", "table", "bar"}

func validateCustomMetricDefinition(b *validationBuilder, def *dto.CustomMetricDefinition, prefix string) {
	if def == nil {
		return
	}

	if strings.TrimSpace(def.Name) == "" {
		b.add(prefix+".name", "Metric name is required.")
	}

	if !slices.Contains(validTargets, def.Target) {
		b.add(prefix+".target", "Target must be 'thread' or 'step'.")
	}

	if def.Target == "step" && strings.TrimSpace(def.StepName) == "" {
		b.add(prefix+".step_name", "Step name is required when target is 'step'.")
	}

	if !slices.Contains(validOperations, def.Operation) {
		b.add(prefix+".operation", "Operation must be one of: COUNT, RATE, AVG, SUM, MIN, MAX.")
	}

	if !slices.Contains(validFields, def.Field) {
		b.add(prefix+".field", "Field is not valid for the selected operation.")
	}

	if def.GroupBy != "" && !slices.Contains(validGroupBy, def.GroupBy) {
		b.add(prefix+".group_by", "Group by must be one of: step name, outcome, process type, violation type, period, none.")
	}

	if def.Granularity != "" && !slices.Contains(validGranularity, def.Granularity) {
		b.add(prefix+".granularity", "Granularity must be one of: hour, day, week, month.")
	}

	if def.Visualisation != "" && !slices.Contains(validVisualisation, def.Visualisation) {
		b.add(prefix+".visualisation", "Visualisation must be one of: number, line, table, bar.")
	}

	for i, f := range def.Filters {
		if f.Key == "" || f.Value == "" {
			b.add(prefix+fmt.Sprintf(".filters[%d]", i), "Filter key and value are required.")
		}
	}
}
