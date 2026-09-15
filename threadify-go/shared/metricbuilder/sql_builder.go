package metricbuilder

import (
	"fmt"
	"strings"

	"threadify-go/shared/domain"

	sq "github.com/Masterminds/squirrel"
)

func BuildSQLFromDefinition(def *domain.MetricDefinition) (string, error) {
	if def == nil {
		return "", fmt.Errorf("metric definition is nil")
	}
	if err := def.Validate(); err != nil {
		return "", fmt.Errorf("invalid metric definition: %w", err)
	}
	return buildSentenceBuilderSQL(def)
}

func buildSentenceBuilderSQL(def *domain.MetricDefinition) (string, error) {
	b, err := buildBaseQuery(def)
	if err != nil {
		return "", err
	}

	b = applyFilters(b, def)
	b = applyGroupBy(b, def)
	b = applyOrderBy(b, def)

	sql, _, err := b.ToSql()
	if err != nil {
		return "", fmt.Errorf("build sentence SQL: %w", err)
	}
	return sql, nil
}

func needsViolationsJoin(def *domain.MetricDefinition) bool {
	if def.Field == "violations" {
		return true
	}
	if def.GroupBy == "violation type" || def.GroupBy == "validation severity" {
		return true
	}
	for _, f := range def.Filters {
		key := strings.ToLower(strings.TrimSpace(f.Key))
		if key == "violation type" || key == "validation severity" {
			return true
		}
	}
	return false
}

func needsTagsJoin(def *domain.MetricDefinition) bool {
	return def.GroupBy == "tag"
}

func violationJoinClause(def *domain.MetricDefinition) string {
	clause := "thread_notifications v ON v.thread_id = t.id"
	if def.Target == "step" {
		clause += " AND v.step_id = s.id"
	}
	clause += " AND v.notification_type = 'rule.violated' AND v.violation_type IS NOT NULL"
	if def.Operation == "RATE" && def.Field == "violations" {
		for _, filter := range def.Filters {
			switch strings.ToLower(strings.TrimSpace(filter.Key)) {
			case "violation type":
				clause += " AND v.violation_type = " + quoteLiteral(strings.TrimSpace(filter.Value))
			case "validation severity":
				clause += " AND v.severity = " + quoteLiteral(strings.TrimSpace(filter.Value))
			}
		}
	}
	return clause
}

func buildBaseQuery(def *domain.MetricDefinition) (sq.SelectBuilder, error) {
	selectClause, err := buildSelectClause(def)
	if err != nil {
		return sq.Select(), err
	}

	if def.Target == "step" {
		b := sq.Select(selectClause).
			From("threads t").
			Join("thread_step_states s ON s.thread_id = t.id")

		if needsViolationsJoin(def) {
			if def.Operation == "RATE" && def.Field == "violations" {
				b = b.LeftJoin(violationJoinClause(def))
			} else {
				b = b.Join(violationJoinClause(def))
			}
		}
		if needsTagsJoin(def) {
			b = b.Join("thread_tags tt ON tt.thread_id = t.id")
		}

		b = b.Where("EXISTS (SELECT 1 FROM thread_refs tr WHERE tr.thread_id = t.id AND tr.ref_value = @ref_value AND tr.ref_key = ANY(@ref_keys))").
			Where("t.created_at >= @start_time").
			Where("t.created_at <= @end_time")

		if def.StepName != "" {
			b = b.Where("s.step_name = @step_name")
		}
		return b, nil
	}

	b := sq.Select(selectClause).From("threads t")

	if needsViolationsJoin(def) {
		if def.Operation == "RATE" && def.Field == "violations" {
			b = b.LeftJoin(violationJoinClause(def))
		} else {
			b = b.Join(violationJoinClause(def))
		}
	}
	if needsTagsJoin(def) {
		b = b.Join("thread_tags tt ON tt.thread_id = t.id")
	}

	b = b.Where("EXISTS (SELECT 1 FROM thread_refs tr WHERE tr.thread_id = t.id AND tr.ref_value = @ref_value AND tr.ref_key = ANY(@ref_keys))").
		Where("t.created_at >= @start_time").
		Where("t.created_at <= @end_time")

	return b, nil
}

func buildSelectClause(def *domain.MetricDefinition) (string, error) {
	target := def.Target

	expr, err := buildFieldExpression(def)
	if err != nil {
		return "", err
	}

	groupBy := def.GroupBy
	granularity := def.Granularity

	var parts []string

	if groupBy == "period" && granularity != "" {
		parts = append(parts, "DATE_TRUNC('"+granularity+"', t.created_at) AS period")
	}

	if groupBy == "step name" {
		if target == "step" {
			parts = append(parts, "s.step_name AS label")
		} else {
			parts = append(parts, "t.contract_name AS label")
		}
	}

	if groupBy == "outcome" {
		if target == "step" {
			parts = append(parts, "s.status AS label")
		} else {
			parts = append(parts, "t.status AS label")
		}
	}

	if groupBy == "actor" {
		if target == "step" {
			parts = append(parts, "s.actor AS label")
		} else {
			parts = append(parts, "t.owner_id AS label")
		}
	}

	if groupBy == "actor service" {
		if target == "step" {
			parts = append(parts, "s.actor_service AS label")
		}
	}

	if groupBy == "process type" {
		parts = append(parts, "t.contract_name AS label")
	}

	if groupBy == "violation type" {
		parts = append(parts, "v.violation_type AS label")
	}

	if groupBy == "validation severity" {
		parts = append(parts, "v.severity AS label")
	}

	if groupBy == "tag" {
		parts = append(parts, "tt.tag AS label")
	}

	parts = append(parts, expr+" AS value")

	return strings.Join(parts, ", "), nil
}

func buildFieldExpression(def *domain.MetricDefinition) (string, error) {
	switch def.Operation {
	case "COUNT":
		return buildCountExpression(def.Field, def.Target)
	case "RATE":
		return buildRateExpression(def)
	case "AVG":
		return buildAvgExpression(def.Field, def.Target)
	case "SUM":
		return buildSumExpression(def.Field, def.Target)
	case "MIN":
		return buildMinExpression(def.Field, def.Target)
	case "MAX":
		return buildMaxExpression(def.Field, def.Target)
	default:
		return "", fmt.Errorf("unsupported operation: %s", def.Operation)
	}
}

func buildCountExpression(field, target string) (string, error) {
	switch field {
	case "threads":
		return "COUNT(DISTINCT t.id)", nil
	case "steps":
		if target == "step" {
			return "COUNT(DISTINCT s.id)", nil
		}
		return "COUNT(DISTINCT s.id)", nil
	case "violations":
		return "COUNT(DISTINCT v.notification_id)", nil
	case "retries":
		if target == "step" {
			return "SUM(s.retry_count)", nil
		}
		return "SUM(s.retry_count)", nil
	case "stepCount":
		if target == "step" {
			return "COUNT(DISTINCT s.step_name)", nil
		}
		return "COUNT(DISTINCT s.step_name)", nil
	default:
		return "", fmt.Errorf("unsupported COUNT field: %s", field)
	}
}

func buildRateExpression(def *domain.MetricDefinition) (string, error) {
	switch def.Field {
	case "outcome":
		status := rateOutcome(def)
		if def.Target == "step" {
			return "ROUND((COUNT(DISTINCT CASE WHEN s.status = " + quoteLiteral(status) + " THEN s.id END)::numeric / NULLIF(COUNT(DISTINCT s.id), 0)) * 100, 2)", nil
		}
		return "ROUND((COUNT(DISTINCT CASE WHEN t.status = " + quoteLiteral(status) + " THEN t.id END)::numeric / NULLIF(COUNT(DISTINCT t.id), 0)) * 100, 2)", nil
	case "violations":
		if def.Target == "step" {
			return "ROUND((COUNT(DISTINCT CASE WHEN v.notification_id IS NOT NULL THEN s.id END)::numeric / NULLIF(COUNT(DISTINCT s.id), 0)) * 100, 2)", nil
		}
		return "ROUND((COUNT(DISTINCT CASE WHEN v.notification_id IS NOT NULL THEN t.id END)::numeric / NULLIF(COUNT(DISTINCT t.id), 0)) * 100, 2)", nil
	default:
		return "", fmt.Errorf("unsupported RATE field: %s", def.Field)
	}
}

func rateOutcome(def *domain.MetricDefinition) string {
	filterKey := "thread outcome"
	defaultValue := "completed"
	if def.Target == "step" {
		filterKey = "step outcome"
		defaultValue = "success"
	}
	for _, filter := range def.Filters {
		if strings.EqualFold(strings.TrimSpace(filter.Key), filterKey) && strings.TrimSpace(filter.Value) != "" {
			return strings.TrimSpace(filter.Value)
		}
	}
	return defaultValue
}

func buildAvgExpression(field, target string) (string, error) {
	switch field {
	case "duration":
		if target == "step" {
			return "ROUND(AVG(EXTRACT(EPOCH FROM (s.finished_at - s.started_at)) * 1000)::numeric, 2)", nil
		}
		return "ROUND(AVG(EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000)::numeric, 2)", nil
	case "retries":
		if target == "step" {
			return "ROUND(AVG(s.retry_count)::numeric, 2)", nil
		}
		return "ROUND(AVG(s.retry_count)::numeric, 2)", nil
	default:
		return "", fmt.Errorf("unsupported AVG field: %s", field)
	}
}

func buildSumExpression(field, target string) (string, error) {
	switch field {
	case "violations":
		return "COUNT(DISTINCT v.notification_id)", nil
	case "retries":
		if target == "step" {
			return "SUM(s.retry_count)", nil
		}
		return "SUM(s.retry_count)", nil
	case "duration":
		if target == "step" {
			return "ROUND(SUM(EXTRACT(EPOCH FROM (s.finished_at - s.started_at)) * 1000)::numeric, 2)", nil
		}
		return "ROUND(SUM(EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000)::numeric, 2)", nil
	case "stepCount":
		if target == "step" {
			return "COUNT(DISTINCT s.step_name)", nil
		}
		return "COUNT(DISTINCT s.step_name)", nil
	default:
		return "", fmt.Errorf("unsupported SUM field: %s", field)
	}
}

func buildMinExpression(field, target string) (string, error) {
	switch field {
	case "duration":
		if target == "step" {
			return "ROUND(MIN(EXTRACT(EPOCH FROM (s.finished_at - s.started_at)) * 1000)::numeric, 2)", nil
		}
		return "ROUND(MIN(EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000)::numeric, 2)", nil
	default:
		return "", fmt.Errorf("unsupported MIN field: %s", field)
	}
}

func buildMaxExpression(field, target string) (string, error) {
	switch field {
	case "duration":
		if target == "step" {
			return "ROUND(MAX(EXTRACT(EPOCH FROM (s.finished_at - s.started_at)) * 1000)::numeric, 2)", nil
		}
		return "ROUND(MAX(EXTRACT(EPOCH FROM (t.completed_at - t.created_at)) * 1000)::numeric, 2)", nil
	case "retries":
		if target == "step" {
			return "MAX(s.retry_count)", nil
		}
		return "MAX(s.retry_count)", nil
	default:
		return "", fmt.Errorf("unsupported MAX field: %s", field)
	}
}

func applyFilters(b sq.SelectBuilder, def *domain.MetricDefinition) sq.SelectBuilder {
	for _, f := range def.Filters {
		key := strings.ToLower(strings.TrimSpace(f.Key))
		value := strings.TrimSpace(f.Value)
		if key == "" || value == "" {
			continue
		}

		switch key {
		case "step name":
			if def.Target == "step" {
				b = b.Where("s.step_name = " + quoteLiteral(value))
			}
		case "step outcome":
			if def.Target == "step" && !(def.Operation == "RATE" && def.Field == "outcome") {
				b = b.Where("s.status = " + quoteLiteral(value))
			}
		case "actor":
			if def.Target == "step" {
				b = b.Where("s.actor = " + quoteLiteral(value))
			} else {
				b = b.Where("t.owner_id = " + quoteLiteral(value))
			}
		case "actor service":
			if def.Target == "step" {
				b = b.Where("s.actor_service = " + quoteLiteral(value))
			}
		case "thread outcome":
			if !(def.Operation == "RATE" && def.Field == "outcome" && def.Target == "thread") {
				b = b.Where("t.status = " + quoteLiteral(value))
			}
		case "process type":
			b = b.Where("t.contract_name = " + quoteLiteral(value))
		case "violation type":
			if !(def.Operation == "RATE" && def.Field == "violations") {
				b = b.Where("v.violation_type = " + quoteLiteral(value))
			}
		case "validation severity":
			if !(def.Operation == "RATE" && def.Field == "violations") {
				b = b.Where("v.severity = " + quoteLiteral(value))
			}
		case "tags":
			values := splitCommaSeparated(value)
			quoted := make([]string, 0, len(values))
			for _, tag := range values {
				quoted = append(quoted, quoteLiteral(tag))
			}
			if def.GroupBy == "tag" {
				b = b.Where("tt.tag IN (" + strings.Join(quoted, ", ") + ")")
			} else {
				b = b.Where("EXISTS (SELECT 1 FROM thread_tags tf WHERE tf.thread_id = t.id AND tf.tag IN (" + strings.Join(quoted, ", ") + "))")
			}
		}
	}
	return b
}

func quoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func applyGroupBy(b sq.SelectBuilder, def *domain.MetricDefinition) sq.SelectBuilder {
	groupBy := def.GroupBy
	if groupBy == "" || groupBy == "none" {
		return b
	}

	granularity := def.Granularity

	switch groupBy {
	case "step name":
		if def.Target == "step" {
			b = b.GroupBy("s.step_name")
		} else {
			b = b.GroupBy("t.contract_name")
		}
	case "outcome":
		if def.Target == "step" {
			b = b.GroupBy("s.status")
		} else {
			b = b.GroupBy("t.status")
		}
	case "actor":
		if def.Target == "step" {
			b = b.GroupBy("s.actor")
		} else {
			b = b.GroupBy("t.owner_id")
		}
	case "actor service":
		if def.Target == "step" {
			b = b.GroupBy("s.actor_service")
		}
	case "process type":
		b = b.GroupBy("t.contract_name")
	case "violation type":
		b = b.GroupBy("v.violation_type")
	case "validation severity":
		b = b.GroupBy("v.severity")
	case "tag":
		b = b.GroupBy("tt.tag")
	case "period":
		if granularity != "" {
			b = b.GroupBy("DATE_TRUNC('" + granularity + "', t.created_at)")
		} else {
			b = b.GroupBy("DATE_TRUNC('day', t.created_at)")
		}
	}

	return b
}

func applyOrderBy(b sq.SelectBuilder, def *domain.MetricDefinition) sq.SelectBuilder {
	groupBy := def.GroupBy
	if groupBy == "period" {
		return b.OrderBy("period ASC")
	}
	if groupBy != "" && groupBy != "none" {
		return b.OrderBy("value DESC")
	}
	return b
}
