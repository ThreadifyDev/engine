package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/interfaces"
)

// AccessRepository handles access control retrieval from PostgreSQL
type AccessRepository struct {
	pool *pgxpool.Pool
}

// NewAccessRepository creates a new PostgreSQL access repository
func NewAccessRepository(pool *pgxpool.Pool) *AccessRepository {
	return &AccessRepository{
		pool: pool,
	}
}

// GetUserAccess retrieves user access from thread_access table
func (r *AccessRepository) GetUserAccess(ctx context.Context, threadID, userID string) (*interfaces.UserAccess, error) {
	query := `
		SELECT roles, permissions, granted_by, granted_at, updated_at, status
		FROM thread_access
		WHERE thread_id = $1 AND user_id = $2
	`

	var rolesJSON, permissionsJSON []byte
	var grantedBy, grantedAt, status string
	var updatedAt sql.NullString

	err := r.pool.QueryRow(ctx, query, threadID, userID).Scan(
		&rolesJSON,
		&permissionsJSON,
		&grantedBy,
		&grantedAt,
		&updatedAt,
		&status,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user does not have access")
		}
		return nil, fmt.Errorf("failed to get user access: %w", err)
	}

	// Parse JSON arrays
	var roles []string
	if err := json.Unmarshal(rolesJSON, &roles); err != nil {
		return nil, fmt.Errorf("failed to parse roles: %w", err)
	}

	var permissions []string
	if err := json.Unmarshal(permissionsJSON, &permissions); err != nil {
		return nil, fmt.Errorf("failed to parse permissions: %w", err)
	}

	access := &interfaces.UserAccess{
		Roles:       roles,
		Permissions: permissions,
		GrantedBy:   grantedBy,
		GrantedAt:   grantedAt,
		Status:      status,
	}

	if updatedAt.Valid {
		access.UpdatedAt = updatedAt.String
	}

	return access, nil
}

// GetAllAccess retrieves all user access for a thread from thread_access table
func (r *AccessRepository) GetAllAccess(ctx context.Context, threadID string) (map[string]*interfaces.UserAccess, error) {
	query := `
		SELECT user_id, roles, permissions, granted_by, granted_at, updated_at, status
		FROM thread_access
		WHERE thread_id = $1
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread access: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*interfaces.UserAccess)

	for rows.Next() {
		var userID string
		var rolesJSON, permissionsJSON []byte
		var grantedBy, grantedAt, status string
		var updatedAt sql.NullString

		err := rows.Scan(
			&userID,
			&rolesJSON,
			&permissionsJSON,
			&grantedBy,
			&grantedAt,
			&updatedAt,
			&status,
		)

		if err != nil {
			continue // Skip invalid entries
		}

		// Parse JSON arrays
		var roles []string
		if err := json.Unmarshal(rolesJSON, &roles); err != nil {
			continue
		}

		var permissions []string
		if err := json.Unmarshal(permissionsJSON, &permissions); err != nil {
			continue
		}

		access := &interfaces.UserAccess{
			Roles:       roles,
			Permissions: permissions,
			GrantedBy:   grantedBy,
			GrantedAt:   grantedAt,
			Status:      status,
		}

		if updatedAt.Valid {
			access.UpdatedAt = updatedAt.String
		}

		result[userID] = access
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}
