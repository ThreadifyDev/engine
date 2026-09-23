package postgres

import (
	"context"
	"fmt"
	"time"
)

// DeliveryOutcomeTrend counts current outcomes by UTC creation day. EXISTS avoids
// Archived timestamps are stored as UTC TIMESTAMP WITHOUT TIME ZONE.
// This avoids session-timezone casts when filtering or bucketing them.
// EXISTS avoids counting a thread twice when multiple profile reference keys match it.
func (r *MetricsRepository) DeliveryOutcomeTrend(ctx context.Context, companyID, ref string, keys []string, days int, end time.Time) ([]map[string]interface{}, error) {
	if companyID == "" || ref == "" || len(keys) == 0 {
		return nil, fmt.Errorf("delivery trend requires company and entity scope")
	}
	if days != 7 && days != 30 && days != 90 {
		return nil, fmt.Errorf("unsupported delivery trend range")
	}
	end = end.UTC()
	start := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	return r.EvaluateEntityMetric(ctx, `
WITH days AS (
  SELECT generate_series((@start_time::timestamptz AT TIME ZONE 'UTC')::date,
    (@end_time::timestamptz AT TIME ZONE 'UTC')::date, interval '1 day')::date AS day
), outcomes AS (
  SELECT t.created_at::date AS day,
    COUNT(*) FILTER (WHERE t.status = 'completed') AS completed,
    COUNT(*) FILTER (WHERE t.status IN ('failed', 'cancelled')) AS failed,
    COUNT(*) AS total
  FROM threads t
  WHERE t.company_id = @company_id
    AND t.created_at >= (@start_time::timestamptz AT TIME ZONE 'UTC')
    AND t.created_at <= (@end_time::timestamptz AT TIME ZONE 'UTC')
    AND EXISTS (SELECT 1 FROM thread_refs tr WHERE tr.thread_id = t.id
      AND tr.ref_value = @ref_value AND tr.ref_key = ANY(@ref_keys))
  GROUP BY 1
)
SELECT to_char(days.day, 'YYYY-MM-DD') AS date,
  COALESCE(outcomes.completed, 0) AS completed,
  COALESCE(outcomes.failed, 0) AS failed,
  COALESCE(outcomes.total, 0) AS total
FROM days LEFT JOIN outcomes USING (day) ORDER BY days.day`, ref, keys, start, end, map[string]any{"company_id": companyID})
}
