package metricbuilder

import (
	"strings"
	"testing"

	"threadify-go/shared/domain"
)

func TestBuildSQLCountsViolationsFromNotifications(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:      "Violations by type",
		Target:    "thread",
		Operation: "COUNT",
		Field:     "violations",
		GroupBy:   "violation type",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"JOIN thread_notifications v ON v.thread_id = t.id",
		"v.notification_type = 'rule.violated'",
		"v.violation_type AS label",
		"COUNT(DISTINCT v.notification_id) AS value",
		"GROUP BY v.violation_type",
	)
	assertNotContains(t, sql, "thread_validations", "v.validation_id")
}

func TestBuildSQLSupportsValidationSeverity(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:      "Critical violations",
		Target:    "thread",
		Operation: "COUNT",
		Field:     "violations",
		Filters: []domain.MetricFilter{
			{Key: "validation severity", Value: "critical"},
		},
		GroupBy: "validation severity",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"v.severity AS label",
		"v.severity = 'critical'",
		"GROUP BY v.severity",
	)
}

func TestBuildSQLSupportsPeriodGrouping(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:        "Daily activity",
		Target:      "thread",
		Operation:   "COUNT",
		Field:       "threads",
		GroupBy:     "period",
		Granularity: "day",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"DATE_TRUNC('day', t.created_at) AS period",
		"GROUP BY DATE_TRUNC('day', t.created_at)",
		"ORDER BY period ASC",
	)
}

func TestBuildSQLOutcomeRateUsesFilterAsNumerator(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:      "Failure rate",
		Target:    "thread",
		Operation: "RATE",
		Field:     "outcome",
		Filters: []domain.MetricFilter{
			{Key: "thread outcome", Value: "failed"},
		},
		GroupBy: "none",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"CASE WHEN t.status = 'failed' THEN t.id END",
		"NULLIF(COUNT(DISTINCT t.id), 0)",
	)
	assertNotContains(t, sql, "WHERE t.status = 'failed'")
}

func TestBuildSQLViolationRatePreservesDenominator(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:      "Critical violation rate",
		Target:    "thread",
		Operation: "RATE",
		Field:     "violations",
		Filters: []domain.MetricFilter{
			{Key: "validation severity", Value: "critical"},
		},
		GroupBy: "none",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"LEFT JOIN thread_notifications v",
		"v.severity = 'critical'",
		"CASE WHEN v.notification_id IS NOT NULL THEN t.id END",
	)
	assertNotContains(t, sql, "WHERE v.severity")
}

func TestBuildSQLTagListIsEscapedAndDoesNotDuplicateBaseRows(t *testing.T) {
	def := &domain.MetricDefinition{
		Name:      "Tagged threads",
		Target:    "thread",
		Operation: "COUNT",
		Field:     "threads",
		Filters: []domain.MetricFilter{
			{Key: "tags", Value: "priority, joe's account"},
		},
		GroupBy: "none",
	}

	sql := mustBuild(t, def)
	assertContains(t, sql,
		"EXISTS (SELECT 1 FROM thread_tags tf",
		"tf.tag IN ('priority', 'joe''s account')",
	)
	assertNotContains(t, sql, "JOIN thread_tags tt")
}

func TestBuildSQLRejectsInvalidCombinations(t *testing.T) {
	tests := []domain.MetricDefinition{
		{Name: "Bad field", Target: "thread", Operation: "AVG", Field: "violations", GroupBy: "none"},
		{Name: "Bad target", Target: "thread", Operation: "COUNT", Field: "steps", GroupBy: "none"},
		{Name: "Bad rate grouping", Target: "thread", Operation: "RATE", Field: "violations", GroupBy: "violation type"},
		{Name: "Missing bucket", Target: "thread", Operation: "COUNT", Field: "threads", GroupBy: "period"},
	}

	for _, def := range tests {
		def := def
		t.Run(def.Name, func(t *testing.T) {
			if _, err := BuildSQLFromDefinition(&def); err == nil {
				t.Fatalf("expected definition to be rejected: %+v", def)
			}
		})
	}
}

func mustBuild(t *testing.T, def *domain.MetricDefinition) string {
	t.Helper()
	sql, err := BuildSQLFromDefinition(def)
	if err != nil {
		t.Fatalf("BuildSQLFromDefinition() error = %v", err)
	}
	return sql
}

func assertContains(t *testing.T, value string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			t.Errorf("SQL did not contain %q:\n%s", fragment, value)
		}
	}
}

func assertNotContains(t *testing.T, value string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(value, fragment) {
			t.Errorf("SQL unexpectedly contained %q:\n%s", fragment, value)
		}
	}
}
