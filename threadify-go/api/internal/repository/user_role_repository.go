package repository

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userRoleRepository struct {
	pool *pgxpool.Pool
}

func NewUserRoleRepository(pool *pgxpool.Pool) UserRoleRepository {
	return &userRoleRepository{pool: pool}
}

func (r *userRoleRepository) AssignRoleToUser(ctx context.Context, userID, roleName, assignedBy string) error {
	return r.AssignRoleToUserTx(ctx, r.pool, userID, roleName, assignedBy)
}

func (r *userRoleRepository) AssignRoleToUserTx(ctx context.Context, execer DBExecer, userID, roleName, assignedBy string) error {
	query := `
		INSERT INTO user_roles (principal_id, principal_type, role_name, assigned_by)
		VALUES ($1, 'user', $2, $3)
		ON CONFLICT (principal_id, role_name) DO NOTHING
	`
	_, err := execer.Exec(ctx, query, userID, roleName, assignedBy)
	return err
}

func (r *userRoleRepository) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	query := `
		SELECT role_name 
		FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'user'
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return nil, err
		}
		roles = append(roles, roleName)
	}

	return roles, rows.Err()
}

func (r *userRoleRepository) RemoveRoleFromUser(ctx context.Context, userID, roleName string) error {
	query := `
		DELETE FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'user' AND role_name = $2
	`
	_, err := r.pool.Exec(ctx, query, userID, roleName)
	return err
}

func (r *userRoleRepository) AssignRoleToServiceAccount(ctx context.Context, serviceAccountID, roleName, assignedBy string) error {
	query := `
		INSERT INTO user_roles (principal_id, principal_type, role_name, assigned_by)
		VALUES ($1, 'service_account', $2, $3)
		ON CONFLICT (principal_id, role_name) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, serviceAccountID, roleName, assignedBy)
	return err
}

func (r *userRoleRepository) GetServiceAccountRoles(ctx context.Context, serviceAccountID string) ([]string, error) {
	query := `
		SELECT role_name 
		FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'service_account'
	`
	rows, err := r.pool.Query(ctx, query, serviceAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var roleName string
		if err := rows.Scan(&roleName); err != nil {
			return nil, err
		}
		roles = append(roles, roleName)
	}

	return roles, rows.Err()
}
