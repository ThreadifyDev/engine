package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type MetricsTemplate struct {
	ID          string
	MetricsName string
	SQLContent  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type MetricsRepository struct {
	db           *pgxpool.Pool
	valkeyClient domain.ValkeyStringClient
	logger       *zap.Logger
}

func NewMetricsRepository(db *pgxpool.Pool, valkeyClient domain.ValkeyStringClient, logger *zap.Logger) *MetricsRepository {
	return &MetricsRepository{
		db:           db,
		valkeyClient: valkeyClient,
		logger:       logger,
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
		SELECT id, metrics_name, sql_content, created_at, updated_at
		FROM metrics_template
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
		if err := rows.Scan(
			&tmpl.ID,
			&tmpl.MetricsName,
			&tmpl.SQLContent,
			&tmpl.CreatedAt,
			&tmpl.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan metrics template: %w", err)
		}
		templates = append(templates, tmpl)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metrics templates: %w", err)
	}

	return templates, nil
}

// EvaluateEntityMetric executes a metric template SQL query for a specific entity profile type
// It merges runtime arguments (startTime, endTime, ref_key) with the static parameters configured
// for the entity profile type in the entity_profile_type_metrics junction table.
func (r *MetricsRepository) EvaluateEntityMetric(
	ctx context.Context,
	sqlQuery string,
	entityRefValue string,
	entityRefKeys []string,
	startTime time.Time,
	endTime time.Time,
	configuredParams map[string]any,
) ([]map[string]interface{}, error) {
	// 4. Time range as natural bound
	if startTime.After(endTime) {
		return nil, fmt.Errorf("startTime cannot be after endTime")
	}

	// Max 3 months constraint (~92 days)
	maxDuration := 92 * 24 * time.Hour
	if endTime.Sub(startTime) > maxDuration {
		return nil, fmt.Errorf("time range exceeds the maximum allowed 3 months")
	}

	// Build Named Arguments map with our protected runtime context
	args := pgx.NamedArgs{
		"start_time": startTime,
		"end_time":   endTime,
		"ref_value":  entityRefValue,
		"ref_keys":   entityRefKeys,
	}

	// Merge configured parameters from the EntityProfileType
	// Only add if it doesn't overwrite our protected runtime context
	for k, v := range configuredParams {
		if _, exists := args[k]; !exists {
			args[k] = v
		}
	}

	// Create a new context with a hard timeout of 30 seconds for the entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Begin a transaction to isolate session settings
	tx, err := r.db.BeginTx(timeoutCtx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// 2. Read-only DB role fallback in session if role is not fully read-only
	// 3. Statement timeout to prevent heavy query running forever (e.g., 20 seconds)
	if _, err := tx.Exec(timeoutCtx, "SET LOCAL statement_timeout = '20s'"); err != nil {
		return nil, fmt.Errorf("failed to set statement timeout: %w", err)
	}

	// Execute the query using pgx.NamedArgs
	rows, err := tx.Query(timeoutCtx, sqlQuery, args)
	if err != nil {
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

// 1. Parameterised queries (args passed as variadic interface{})
// 2. Read-only session (SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY)
// 3. Statement timeout to prevent heavy queries running forever (e.g., 30s)
// 4. Time range bounds check (startTime and endTime must be within max 3 months)
func (r *MetricsRepository) ExecuteMetricsQuery(
	ctx context.Context,
	sqlQuery string,
	startTime time.Time,
	endTime time.Time,
	args ...interface{},
) ([]map[string]interface{}, error) {
	// 4. Time range as natural bound
	if startTime.After(endTime) {
		return nil, fmt.Errorf("startTime cannot be after endTime")
	}

	// Max 3 months constraint (~92 days)
	maxDuration := 92 * 24 * time.Hour
	if endTime.Sub(startTime) > maxDuration {
		return nil, fmt.Errorf("time range exceeds the maximum allowed 3 months")
	}

	// Create a new context with a hard timeout of 30 seconds for the entire operation
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Begin a transaction to isolate session settings
	tx, err := r.db.BeginTx(timeoutCtx, pgx.TxOptions{
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// 2. Read-only DB role fallback in session if role is not fully read-only
	// 3. Statement timeout to prevent heavy query running forever (e.g., 20 seconds)
	if _, err := tx.Exec(timeoutCtx, "SET LOCAL statement_timeout = '20s'"); err != nil {
		return nil, fmt.Errorf("failed to set statement timeout: %w", err)
	}

	// Enforce startTime and endTime as the first two parameters ($1 and $2).
	// Subsequent args start from $3.
	queryArgs := make([]interface{}, 0, 2+len(args))
	queryArgs = append(queryArgs, startTime, endTime)
	queryArgs = append(queryArgs, args...)

	rows, err := tx.Query(timeoutCtx, sqlQuery, queryArgs...)
	if err != nil {
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

// GetCachedEntityMetrics retrieves cached entity metrics from Valkey
func (r *MetricsRepository) GetCachedEntityMetrics(ctx context.Context, profileID, rangeVal string) (map[string]any, bool) {
	if r.valkeyClient == nil {
		return nil, false
	}

	cacheKey := "entity_metrics:" + profileID + ":" + rangeVal
	cachedData, err := r.valkeyClient.Get(ctx, cacheKey)
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

// CacheEntityMetrics stores computed entity metrics in Valkey with 30-minute TTL
func (r *MetricsRepository) CacheEntityMetrics(ctx context.Context, profileID, rangeVal string, metrics map[string]any) {
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

	cacheKey := "entity_metrics:" + profileID + ":" + rangeVal
	if err := r.valkeyClient.Set(ctx, cacheKey, string(resultBytes), 30*time.Minute); err != nil {
		r.logger.Warn("failed to cache entity metrics",
			zap.String("profileID", profileID),
			zap.String("range", rangeVal),
			zap.Error(err),
		)
	}
}
