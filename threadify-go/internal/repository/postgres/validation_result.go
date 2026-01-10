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
			validations
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
			validations
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
