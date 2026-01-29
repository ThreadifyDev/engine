package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

type ValidationRepository struct {
	pool *pgxpool.Pool
}

func NewValidationRepository(pool *pgxpool.Pool) *ValidationRepository {
	return &ValidationRepository{pool: pool}
}

// GetValidationResults retrieves validation results for a specific step
func (r *ValidationRepository) GetValidationResults(ctx context.Context, threadID, stepName, idempotencyKey string) ([]*models.ValidationResultInfo, error) {
	stepID := fmt.Sprintf("%s:%s", stepName, idempotencyKey)

	query := `
		SELECT 
			validation_id,
			thread_id,
			step_id,
			step_name,
			idempotency_key,
			timestamp,
			validations,
			overall_status,
			has_critical_violation,
			critical_count,
			warning_count,
			minor_count,
			info_count,
			total_validations
		FROM thread_validations 
		WHERE thread_id = $1 AND step_id = $2
		ORDER BY timestamp ASC
	`

	rows, err := r.pool.Query(ctx, query, threadID, stepID)
	if err != nil {
		return nil, fmt.Errorf("failed to query validation results: %w", err)
	}
	defer rows.Close()

	var results []*models.ValidationResultInfo
	for rows.Next() {
		var result models.ValidationResultInfo
		var validationsJSON string

		err := rows.Scan(
			&result.ValidationID,
			&result.ThreadID,
			&result.StepID,
			&result.StepName,
			&result.IdempotencyKey,
			&result.Timestamp,
			&validationsJSON,
			&result.OverallStatus,
			&result.HasCriticalViolation,
			&result.CriticalCount,
			&result.WarningCount,
			&result.MinorCount,
			&result.InfoCount,
			&result.TotalValidations,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation result: %w", err)
		}

		// Parse validations JSON array
		if err := json.Unmarshal([]byte(validationsJSON), &result.Validations); err != nil {
			return nil, fmt.Errorf("failed to parse validations JSON: %w", err)
		}

		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating validation results: %w", err)
	}

	return results, nil
}

// GetThreadValidationResults retrieves all validation results for a thread
func (r *ValidationRepository) GetThreadValidationResults(ctx context.Context, threadID string, options *models.ValidationQueryOptions) ([]*models.ValidationResultInfo, error) {
	// Build query with optional filters
	query := `
		SELECT 
			validation_id,
			thread_id,
			step_id,
			step_name,
			idempotency_key,
			timestamp,
			validations,
			overall_status,
			has_critical_violation,
			critical_count,
			warning_count,
			minor_count,
			info_count,
			total_validations
		FROM thread_validations 
		WHERE thread_id = $1
	`

	args := []interface{}{threadID}
	argIndex := 2

	// Add optional filters
	if options != nil {
		if options.StepID != "" {
			query += fmt.Sprintf(" AND step_id = $%d", argIndex)
			args = append(args, options.StepID)
			argIndex++
		}
		if options.StepName != "" {
			query += fmt.Sprintf(" AND step_name = $%d", argIndex)
			args = append(args, options.StepName)
			argIndex++
		}
		if options.IdempotencyKey != "" {
			query += fmt.Sprintf(" AND idempotency_key = $%d", argIndex)
			args = append(args, options.IdempotencyKey)
			argIndex++
		}
	}

	query += " ORDER BY timestamp ASC"

	// Add pagination
	if options != nil && options.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, options.Limit)
		argIndex++

		if options.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, options.Offset)
		}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread validation results: %w", err)
	}
	defer rows.Close()

	var results []*models.ValidationResultInfo
	for rows.Next() {
		var result models.ValidationResultInfo
		var validationsJSON string

		err := rows.Scan(
			&result.ValidationID,
			&result.ThreadID,
			&result.StepID,
			&result.StepName,
			&result.IdempotencyKey,
			&result.Timestamp,
			&validationsJSON,
			&result.OverallStatus,
			&result.HasCriticalViolation,
			&result.CriticalCount,
			&result.WarningCount,
			&result.MinorCount,
			&result.InfoCount,
			&result.TotalValidations,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation result: %w", err)
		}

		// Parse validations JSON array
		if err := json.Unmarshal([]byte(validationsJSON), &result.Validations); err != nil {
			return nil, fmt.Errorf("failed to parse validations JSON: %w", err)
		}

		// Filter by validation type if specified
		if options != nil && options.ValidationType != "" {
			var filteredValidations []models.ValidationIssue
			for _, validation := range result.Validations {
				if strings.EqualFold(validation.Type, options.ValidationType) {
					filteredValidations = append(filteredValidations, validation)
				}
			}
			result.Validations = filteredValidations
		}

		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating validation results: %w", err)
	}

	return results, nil
}

// ValidationResultExists checks if validation results exist for a step
func (r *ValidationRepository) ValidationResultExists(ctx context.Context, threadID, stepName, idempotencyKey string) (bool, error) {
	stepID := fmt.Sprintf("%s:%s", stepName, idempotencyKey)

	query := `SELECT EXISTS(SELECT 1 FROM thread_validations WHERE thread_id = $1 AND step_id = $2)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, threadID, stepID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check validation result existence: %w", err)
	}

	return exists, nil
}

// GetValidationResultsWithPermissionCheck retrieves validation results with SQL-level permission filtering
// Joins with thread_step_states to get step actor for .own permission filtering
func (r *ValidationRepository) GetValidationResultsWithPermissionCheck(
	ctx context.Context,
	threadID string,
	userID string,
	options *models.ValidationQueryOptions,
) ([]*models.ValidationResultInfo, error) {
	// Build query with permission filtering via JOIN to thread_access and thread_step_states
	// Permission logic:
	// - thread.read.* = can see all validations
	// - thread.read.own = can only see validations for steps where actor = userID
	query := `
		SELECT 
			v.validation_id,
			v.thread_id,
			v.step_id,
			v.step_name,
			v.idempotency_key,
			v.timestamp,
			v.validations,
			v.overall_status,
			v.has_critical_violation,
			v.critical_count,
			v.warning_count,
			v.minor_count,
			v.info_count,
			v.total_validations
		FROM thread_validations v
		INNER JOIN thread_access ta ON v.thread_id = ta.thread_id
		LEFT JOIN thread_step_states s ON v.thread_id = s.thread_id 
			AND v.step_name = s.step_name 
			AND v.idempotency_key = s.idempotency_key
		WHERE v.thread_id = $1
		  AND ta.user_id = $2
		  AND ta.status = 'active'
		  AND (
			'thread.read.*' = ANY(ta.permissions)
			OR ('thread.read.own' = ANY(ta.permissions) AND (s.actor = $2 OR s.actor IS NULL))
		  )
	`

	args := []interface{}{threadID, userID}
	argIndex := 3

	// Add optional filters
	if options != nil {
		if options.StepID != "" {
			query += fmt.Sprintf(" AND v.step_id = $%d", argIndex)
			args = append(args, options.StepID)
			argIndex++
		}
		if options.StepName != "" {
			query += fmt.Sprintf(" AND v.step_name = $%d", argIndex)
			args = append(args, options.StepName)
			argIndex++
		}
		if options.IdempotencyKey != "" {
			query += fmt.Sprintf(" AND v.idempotency_key = $%d", argIndex)
			args = append(args, options.IdempotencyKey)
			argIndex++
		}
	}

	query += " ORDER BY v.timestamp ASC"

	// Add pagination
	if options != nil && options.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, options.Limit)
		argIndex++

		if options.Offset > 0 {
			query += fmt.Sprintf(" OFFSET $%d", argIndex)
			args = append(args, options.Offset)
		}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query validation results with permission check: %w", err)
	}
	defer rows.Close()

	var results []*models.ValidationResultInfo
	for rows.Next() {
		var result models.ValidationResultInfo
		var validationsJSON string

		err := rows.Scan(
			&result.ValidationID,
			&result.ThreadID,
			&result.StepID,
			&result.StepName,
			&result.IdempotencyKey,
			&result.Timestamp,
			&validationsJSON,
			&result.OverallStatus,
			&result.HasCriticalViolation,
			&result.CriticalCount,
			&result.WarningCount,
			&result.MinorCount,
			&result.InfoCount,
			&result.TotalValidations,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan validation result: %w", err)
		}

		// Parse validations JSON array
		if err := json.Unmarshal([]byte(validationsJSON), &result.Validations); err != nil {
			return nil, fmt.Errorf("failed to parse validations JSON: %w", err)
		}

		// Filter by validation type if specified
		if options != nil && options.ValidationType != "" {
			var filteredValidations []models.ValidationIssue
			for _, validation := range result.Validations {
				if strings.EqualFold(validation.Type, options.ValidationType) {
					filteredValidations = append(filteredValidations, validation)
				}
			}
			result.Validations = filteredValidations
		}

		results = append(results, &result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating validation results: %w", err)
	}

	return results, nil
}
