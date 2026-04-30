package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/domain"
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
func (r *AccessRepository) GetUserAccess(ctx context.Context, threadID, userID string) (*domain.UserAccess, error) {
	query := `
		SELECT roles, runtime_role, permissions, granted_by, granted_at, updated_at, status
		FROM thread_access
		WHERE thread_id = $1 AND user_id = $2
	`

	var rolesJSON []byte
	var runtimeRole string
	var permissions []string
	var grantedBy, status string
	var grantedAt time.Time
	var updatedAt sql.NullTime

	err := r.pool.QueryRow(ctx, query, threadID, userID).Scan(
		&rolesJSON,
		&runtimeRole,
		&permissions,
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

	// Parse JSON array for roles
	var roles []string
	if err := json.Unmarshal(rolesJSON, &roles); err != nil {
		return nil, fmt.Errorf("failed to parse roles: %w", err)
	}

	access := &domain.UserAccess{
		Roles:       roles,
		RuntimeRole: runtimeRole,
		Permissions: permissions,
		GrantedBy:   grantedBy,
		GrantedAt:   grantedAt,
		Status:      status,
	}

	if updatedAt.Valid {
		access.UpdatedAt = &updatedAt.Time
	}

	return access, nil
}

// GetAllAccess retrieves all user access for a thread from thread_access table
func (r *AccessRepository) GetAllAccess(ctx context.Context, threadID string) (map[string]*domain.UserAccess, error) {
	query := `
		SELECT user_id, roles, runtime_role, permissions, granted_by, granted_at, updated_at, status
		FROM thread_access
		WHERE thread_id = $1
	`

	rows, err := r.pool.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query thread access: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*domain.UserAccess)

	for rows.Next() {
		var userID string
		var rolesJSON []byte
		var runtimeRole string
		var permissions []string
		var grantedBy, status string
		var grantedAt time.Time
		var updatedAt sql.NullTime

		err := rows.Scan(
			&userID,
			&rolesJSON,
			&runtimeRole,
			&permissions,
			&grantedBy,
			&grantedAt,
			&updatedAt,
			&status,
		)

		if err != nil {
			continue // Skip invalid entries
		}

		// Parse JSON array for roles
		var roles []string
		if err := json.Unmarshal(rolesJSON, &roles); err != nil {
			continue
		}

		result[userID] = &domain.UserAccess{
			Roles:       roles,
			RuntimeRole: runtimeRole,
			Permissions: permissions,
			GrantedBy:   grantedBy,
			GrantedAt:   grantedAt,
			Status:      status,
		}

		if updatedAt.Valid {
			result[userID].UpdatedAt = &updatedAt.Time
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}

// GetUsersByRuntimeRoles retrieves users filtered by runtime_role for a thread
// Returns minimal data (user_id, runtime_role) for efficient notification routing
func (r *AccessRepository) GetUsersByRuntimeRoles(
	ctx context.Context,
	threadID string,
	runtimeRoles []string,
) ([]domain.UserRoleInfo, error) {
	query := `
		SELECT user_id, runtime_role
		FROM thread_access
		WHERE thread_id = $1
		  AND status = 'active'
		  AND runtime_role = ANY($2)
	`

	rows, err := r.pool.Query(ctx, query, threadID, runtimeRoles)
	if err != nil {
		return nil, fmt.Errorf("failed to query users by runtime roles: %w", err)
	}
	defer rows.Close()

	var users []domain.UserRoleInfo

	for rows.Next() {
		var user domain.UserRoleInfo
		if err := rows.Scan(&user.UserID, &user.RuntimeRole); err != nil {
			continue // Skip invalid entries
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}

// GetUsersByPermissions retrieves users who have ANY of the required permissions
// Uses GIN index on permissions column for efficient array overlap queries
// Returns user_id AND permissions array for .own filtering in application layer
func (r *AccessRepository) GetUsersByPermissions(
	ctx context.Context,
	threadID string,
	requiredPermissions []string,
) ([]domain.UserPermissionInfo, error) {
	// Use && operator for array overlap - returns true if arrays have any common elements
	// GIN index on permissions column makes this O(log N) instead of O(N)
	query := `
		SELECT user_id, permissions
		FROM thread_access
		WHERE thread_id = $1
		  AND status = 'active'
		  AND permissions && $2
	`

	rows, err := r.pool.Query(ctx, query, threadID, requiredPermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to query users by permissions: %w", err)
	}
	defer rows.Close()

	var users []domain.UserPermissionInfo

	for rows.Next() {
		var user domain.UserPermissionInfo
		if err := rows.Scan(&user.UserID, &user.Permissions); err != nil {
			continue // Skip invalid entries
		}
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return users, nil
}
