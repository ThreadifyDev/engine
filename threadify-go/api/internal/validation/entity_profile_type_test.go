package validation

import (
	"testing"

	"threadify-go/api/internal/dto"
)

func TestValidateCustomMetricSupportsExpandedBuilderOptions(t *testing.T) {
	req := &dto.CreateEntityProfileTypeRequest{
		Name: "Account",
		Type: []string{"account_id"},
		Metrics: []dto.EntityTypeMetric{
			{CustomDefinition: &dto.CustomMetricDefinition{
				Name: "Daily activity", Target: "thread", Operation: "COUNT", Field: "threads",
				GroupBy: "period", Granularity: "day",
			}},
			{CustomDefinition: &dto.CustomMetricDefinition{
				Name: "Violations by severity", Target: "thread", Operation: "COUNT", Field: "violations",
				GroupBy: "validation severity",
			}},
			{CustomDefinition: &dto.CustomMetricDefinition{
				Name: "Duration by service", Target: "step", Operation: "AVG", Field: "duration",
				GroupBy: "actor service",
			}},
		},
	}

	if err := ValidateCreateEntityProfileTypeRequest(req); err != nil {
		t.Fatalf("expected expanded custom metrics to validate: %v", err)
	}
}

func TestValidateCustomMetricRejectsInvalidCombinations(t *testing.T) {
	tests := []dto.CustomMetricDefinition{
		{Name: "Missing bucket", Target: "thread", Operation: "COUNT", Field: "threads", GroupBy: "period"},
		{Name: "Wrong field", Target: "thread", Operation: "AVG", Field: "violations", GroupBy: "none"},
		{Name: "Wrong target", Target: "thread", Operation: "COUNT", Field: "steps", GroupBy: "none"},
		{Name: "Bad rate group", Target: "thread", Operation: "RATE", Field: "violations", GroupBy: "violation type"},
	}

	for _, definition := range tests {
		definition := definition
		t.Run(definition.Name, func(t *testing.T) {
			req := &dto.CreateEntityProfileTypeRequest{
				Name:    "Account",
				Type:    []string{"account_id"},
				Metrics: []dto.EntityTypeMetric{{CustomDefinition: &definition}},
			}
			if err := ValidateCreateEntityProfileTypeRequest(req); err == nil {
				t.Fatalf("expected custom metric to be rejected: %+v", definition)
			}
		})
	}
}
