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
	valkeyClient domain.ValkeyClient
	logger       *zap.Logger
}

func NewMetricsRepository(db *pgxpool.Pool, valkeyClient domain.ValkeyClient, logger *zap.Logger) *MetricsRepository {
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

// GetMetricsTemplatesByIDs retrieves metrics templates by their IDs
func (r *MetricsRepository) GetMetricsTemplatesByIDs(ctx context.Context, ids []string) ([]MetricsTemplate, error) {
	if len(ids) == 0 {
		return []MetricsTemplate{}, nil
	}

	query := `
		SELECT id, metrics_name, sql_content, created_at, updated_at
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

// CacheEntityMetrics stores computed entity metrics in Valkey Hash with 30-minute TTL
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
