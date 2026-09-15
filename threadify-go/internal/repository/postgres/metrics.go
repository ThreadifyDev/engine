package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

// maxConcurrentMetricQueries limits the number of metric SQL queries that can
// execute simultaneously, preventing connection-pool exhaustion under load.
const maxConcurrentMetricQueries = 10

type MetricsTemplate struct {
	ID          string
	MetricsName string
	Description string
	SQLContent  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MetricsRepository struct {
	db           *pgxpool.Pool
	valkeyClient domain.ValkeyClient
	logger       *zap.Logger
	sem          chan struct{} // concurrency limiter for metric queries
}

func NewMetricsRepository(db *pgxpool.Pool, valkeyClient domain.ValkeyClient, logger *zap.Logger) *MetricsRepository {
	return &MetricsRepository{
		db:           db,
		valkeyClient: valkeyClient,
		logger:       logger,
		sem:          make(chan struct{}, maxConcurrentMetricQueries),
	}
}

// GetMetricsTemplate retrieves a metrics template by ID
func (r *MetricsRepository) GetMetricsTemplate(ctx context.Context, id string) (*MetricsTemplate, error) {
	query := `
		SELECT id, metrics_name, sql_content, created_at, updated_at
		FROM metrics_template
		WHERE id = $1
	`
	var tmpl MetricsTemplate
	err := r.db.QueryRow(ctx, query, id).Scan(
		&tmpl.ID,
		&tmpl.MetricsName,
		&tmpl.SQLContent,
		&tmpl.CreatedAt,
		&tmpl.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, fmt.Errorf("failed to get metrics template: %w", err)
	}

	return &tmpl, nil
}

// ListMetricsTemplates retrieves all metrics templates
func (r *MetricsRepository) ListMetricsTemplates(ctx context.Context) ([]MetricsTemplate, error) {
	query := `
		SELECT id, metrics_name, description, sql_content, created_at, updated_at
		FROM metrics_template
		WHERE is_system = false
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics templates: %w", err)
	}
	defer rows.Close()

	var templates []MetricsTemplate
	for rows.Next() {
		var tmpl MetricsTemplate
		var desc sql.NullString
		if err := rows.Scan(
			&tmpl.ID,
			&tmpl.MetricsName,
			&desc,
			&tmpl.SQLContent,
			&tmpl.CreatedAt,
			&tmpl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan metrics template: %w", err)
		}
		if desc.Valid {
			tmpl.Description = desc.String
		}
		templates = append(templates, tmpl)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics templates: %w", err)
	}

	return templates, nil
}

// GetMetricsTemplatesByIDs retrieves metrics templates by their IDs
func (r *MetricsRepository) GetMetricsTemplatesByIDs(ctx context.Context, ids []string) ([]MetricsTemplate, error) {
	if len(ids) == 0 {
		return []MetricsTemplate{}, nil
	}

	query := `
		SELECT id, metrics_name, description, sql_content, created_at, updated_at
		FROM metrics_template
		WHERE id = ANY($1)
	`
	rows, err := r.db.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics templates by ids: %w", err)
	}
	defer rows.Close()

	var templates []MetricsTemplate
	for rows.Next() {
		var tmpl MetricsTemplate
		var desc sql.NullString
		if err := rows.Scan(
			&tmpl.ID,
			&tmpl.MetricsName,
			&desc,
			&tmpl.SQLContent,
			&tmpl.CreatedAt,
			&tmpl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan metrics template: %w", err)
		}
		if desc.Valid {
			tmpl.Description = desc.String
		}
		templates = append(templates, tmpl)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics templates: %w", err)
	}

	return templates, nil
}

func (r *MetricsRepository) EvaluateEntityMetric(
	ctx context.Context,
	sqlQuery string,
	entityRefValue string,
	entityRefKeys []string,
	startTime time.Time,
	endTime time.Time,
	configuredParams map[string]any,
) ([]map[string]interface{}, error) {
	if startTime.After(endTime) {
		return nil, fmt.Errorf("startTime cannot be after endTime")
	}

	maxDuration := 92 * 24 * time.Hour
	if endTime.Sub(startTime) > maxDuration {
		return nil, fmt.Errorf("time range exceeds the maximum allowed 3 months")
	}

	// Acquire semaphore slot to limit concurrent DB connections used by metrics.
	select {
	case r.sem <- struct{}{}:
		defer func() { <-r.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	args := pgx.NamedArgs{
		"start_time": startTime,
		"end_time":   endTime,
		"ref_value":  entityRefValue,
		"ref_keys":   entityRefKeys,
	}

	for k, v := range configuredParams {
		if _, exists := args[k]; !exists {
			args[k] = v
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tx, err := r.db.BeginTx(timeoutCtx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if _, err := tx.Exec(timeoutCtx, "SET LOCAL statement_timeout = '20s'"); err != nil {
		return nil, fmt.Errorf("failed to set statement timeout: %w", err)
	}

	// Remove ALL semicolons from the template SQL — they are statement terminators
	// and are invalid inside the subquery wrapper we add for safety.
	cleanQuery := strings.ReplaceAll(sqlQuery, ";", "")
	cleanQuery = strings.TrimSpace(cleanQuery)

	limitedQuery := fmt.Sprintf("SELECT * FROM (%s) AS subquery LIMIT 1000", cleanQuery)

	// Execute the query using pgx.NamedArgs
	rows, err := tx.Query(timeoutCtx, limitedQuery, args)
	if err != nil {
		r.logger.Error("metrics query execution failed",
			zap.String("cleanQuery", cleanQuery),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to execute metrics query: %w", err)
	}
	defer rows.Close()

	results, err := pgx.CollectRows(rows, pgx.RowToMap)
	if err != nil {
		return nil, fmt.Errorf("error collecting metrics results: %w", err)
	}

	if err := tx.Commit(timeoutCtx); err != nil {
		return nil, fmt.Errorf("failed to commit metrics transaction: %w", err)
	}

	return results, nil
}

func (r *MetricsRepository) GetCachedEntityMetrics(ctx context.Context, profileID, rangeVal, configVersion string) (map[string]any, bool) {
	if r.valkeyClient == nil {
		return nil, false
	}

	hashKey := "entity_metrics:" + profileID
	fieldKey := rangeVal + ":" + configVersion

	cachedData, err := r.valkeyClient.HGet(ctx, hashKey, fieldKey)
	if err != nil || cachedData == "" {
		return nil, false
	}

	var cachedMap map[string]any
	if err := json.Unmarshal([]byte(cachedData), &cachedMap); err != nil {
		r.logger.Warn("failed to unmarshal cached entity metrics",
			zap.String("profileID", profileID),
			zap.String("range", rangeVal),
			zap.Error(err),
		)
		return nil, false
	}

	return cachedMap, true
}

func (r *MetricsRepository) CacheEntityMetrics(ctx context.Context, profileID, rangeVal, configVersion string, metrics map[string]any) {
	if r.valkeyClient == nil {
		return
	}

	resultBytes, err := json.Marshal(metrics)
	if err != nil {
		r.logger.Warn("failed to marshal entity metrics for caching",
			zap.String("profileID", profileID),
			zap.String("range", rangeVal),
			zap.Error(err),
		)
		return
	}

	hashKey := "entity_metrics:" + profileID
	fieldKey := rangeVal + ":" + configVersion

	if err := r.valkeyClient.HSet(ctx, hashKey, fieldKey, string(resultBytes)); err != nil {
		r.logger.Warn("failed to cache entity metrics",
			zap.String("profileID", profileID),
			zap.String("range", rangeVal),
			zap.Error(err),
		)
	} else {
		// Set an expiration on the whole hash as a fallback TTL
		_ = r.valkeyClient.Expire(ctx, hashKey, 30*time.Minute)
	}
}

// InvalidateEntityMetrics immediately deletes all cached metrics for a profile
func (r *MetricsRepository) InvalidateEntityMetrics(ctx context.Context, profileID string) error {
	if r.valkeyClient == nil {
		return nil
	}

	hashKey := "entity_metrics:" + profileID
	return r.valkeyClient.Del(ctx, hashKey)
}

// ValidateTemplateSQL runs EXPLAIN on the template SQL with dummy parameters
// to catch syntax errors at bind time rather than at dashboard render time.
// This is a best-effort check — it validates SQL syntax and table/column
// references but cannot guarantee runtime correctness with real data.
func (r *MetricsRepository) ValidateTemplateSQL(ctx context.Context, sqlContent string, configuredParams map[string]any) error {
	cleanQuery := strings.ReplaceAll(sqlContent, ";", "")
	cleanQuery = strings.TrimSpace(cleanQuery)
	if cleanQuery == "" {
		return fmt.Errorf("template SQL content is empty")
	}

	// Build dummy args matching what EvaluateEntityMetric provides at runtime
	args := pgx.NamedArgs{
		"start_time": time.Now().Add(-7 * 24 * time.Hour),
		"end_time":   time.Now(),
		"ref_value":  "__validate_dummy__",
		"ref_keys":   []string{"__validate_dummy__"},
	}
	for k, v := range configuredParams {
		if _, exists := args[k]; !exists {
			args[k] = v
		}
	}

	explainQuery := fmt.Sprintf("EXPLAIN SELECT * FROM (%s) AS subquery LIMIT 1000", cleanQuery)

	validateCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if _, err := r.db.Exec(validateCtx, explainQuery, args); err != nil {
		return fmt.Errorf("invalid metrics template SQL: %w", err)
	}
	return nil
}
