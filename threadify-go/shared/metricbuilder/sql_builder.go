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

func needsValidationsJoin(def *domain.MetricDefinition) bool {
	if def.Field == "violations" {
		return true
	}
	if def.GroupBy == "violation type" {
		return true
	}
	for _, f := range def.Filters {
		if strings.ToLower(strings.TrimSpace(f.Key)) == "violation type" {
			return true
		}
	}
	return false
}

func needsTagsJoin(def *domain.MetricDefinition) bool {
	if def.GroupBy == "tag" {
		return true
	}
	for _, f := range def.Filters {
		if strings.ToLower(strings.TrimSpace(f.Key)) == "tags" {
			return true
		}
	}
	return false
}

func buildBaseQuery(def *domain.MetricDefinition) (sq.SelectBuilder, error) {
	selectClause, err := buildSelectClause(def)
	if err != nil {
		return sq.Select(), err
	}

	if def.Target == "step" {
		b := sq.Select(selectClause).
			From("thread_refs tr").
			Join("threads t ON t.id = tr.thread_id").
			Join("thread_step_states s ON s.thread_id = tr.thread_id")

		if needsValidationsJoin(def) {
			b = b.Join("thread_validations v ON v.thread_id = t.id")
		}
		if needsTagsJoin(def) {
			b = b.Join("thread_tags tt ON tt.thread_id = t.id")
		}

		b = b.Where("tr.ref_value = @ref_value").
			Where("tr.ref_key = ANY(@ref_keys)").
			Where("t.created_at >= @start_time").
			Where("t.created_at <= @end_time")

		if def.StepName != "" {
			b = b.Where("s.step_name = @step_name")
		}
		return b, nil
	}

	b := sq.Select(selectClause).
		From("thread_refs tr").
		Join("threads t ON t.id = tr.thread_id")

	if needsValidationsJoin(def) {
		b = b.Join("thread_validations v ON v.thread_id = t.id")
	}
	if needsTagsJoin(def) {
		b = b.Join("thread_tags tt ON tt.thread_id = t.id")
	}

	b = b.Where("tr.ref_value = @ref_value").
		Where("tr.ref_key = ANY(@ref_keys)").
		Where("t.created_at >= @start_time").
		Where("t.created_at <= @end_time")

	return b, nil
}

func buildSelectClause(def *domain.MetricDefinition) (string, error) {
	op := def.Operation
	field := def.Field
	target := def.Target

	expr, err := buildFieldExpression(op, field, target)
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

	if groupBy == "tag" {
		parts = append(parts, "tt.tag AS label")
	}

	parts = append(parts, expr+" AS value")

	return strings.Join(parts, ", "), nil
}

func buildFieldExpression(op, field, target string) (string, error) {
	switch op {
	case "COUNT":
		return buildCountExpression(field, target)
	case "RATE":
		return buildRateExpression(field, target)
	case "AVG":
		return buildAvgExpression(field, target)
	case "SUM":
		return buildSumExpression(field, target)
	case "MIN":
		return buildMinExpression(field, target)
	case "MAX":
		return buildMaxExpression(field, target)
	default:
		return "", fmt.Errorf("unsupported operation: %s", op)
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
		return "COUNT(DISTINCT v.validation_id)", nil
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
		return "COUNT(*)", nil
	}
}

func buildRateExpression(field, target string) (string, error) {
	switch field {
	case "outcome":
		if target == "step" {
			return "ROUND((COUNT(CASE WHEN s.status = 'completed' THEN 1 END)::numeric / NULLIF(COUNT(*), 0)) * 100, 2)", nil
		}
		return "ROUND((COUNT(CASE WHEN t.status = 'completed' THEN 1 END)::numeric / NULLIF(COUNT(*), 0)) * 100, 2)", nil
	case "violations":
		return "ROUND((COUNT(DISTINCT v.validation_id)::numeric / NULLIF(COUNT(DISTINCT t.id), 0)) * 100, 2)", nil
	default:
		return "ROUND((COUNT(*)::numeric / NULLIF(COUNT(DISTINCT t.id), 0)) * 100, 2)", nil
	}
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
		return "AVG(0)", nil
	}
}

func buildSumExpression(field, target string) (string, error) {
	switch field {
	case "violations":
		return "COUNT(DISTINCT v.validation_id)", nil
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
		return "SUM(0)", nil
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
		return "MIN(0)", nil
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
		return "MAX(0)", nil
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
				b = b.Where("s.step_name = '" + value + "'")
			}
		case "step outcome":
			if def.Target == "step" {
				b = b.Where("s.status = '" + value + "'")
			}
		case "actor":
			if def.Target == "step" {
				b = b.Where("s.actor = '" + value + "'")
			} else {
				b = b.Where("t.owner_id = '" + value + "'")
			}
		case "actor service":
			if def.Target == "step" {
				b = b.Where("s.actor_service = '" + value + "'")
			}
		case "thread outcome":
			b = b.Where("t.status = '" + value + "'")
		case "process type":
			b = b.Where("t.contract_name = '" + value + "'")
		case "violation type":
			b = b.Where("v.violation_type = '" + value + "'")
		case "tags":
			b = b.Where("tt.tag = '" + value + "'")
		}
	}
	return b
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
		}
	case "actor service":
		if def.Target == "step" {
			b = b.GroupBy("s.actor_service")
		}
	case "process type":
		b = b.GroupBy("t.contract_name")
	case "violation type":
		b = b.GroupBy("v.violation_type")
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
