package repository

import (
	"database/sql"
)

type UserRoleRepository struct {
	db *sql.DB
}

func NewUserRoleRepository(db *sql.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) AssignRoleToUser(userID, roleName, assignedBy string) error {
	return r.AssignRoleToUserTx(r.db, userID, roleName, assignedBy)
}

type roleExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (r *UserRoleRepository) AssignRoleToUserTx(execer roleExecer, userID, roleName, assignedBy string) error {
	query := `
		INSERT INTO user_roles (principal_id, principal_type, role_name, assigned_by)
		VALUES ($1, 'user', $2, $3)
		ON CONFLICT (principal_id, role_name) DO NOTHING
	`
	_, err := execer.Exec(query, userID, roleName, assignedBy)
	return err
}

func (r *UserRoleRepository) GetUserRoles(userID string) ([]string, error) {
	query := `
		SELECT role_name 
		FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'user'
	`
	rows, err := r.db.Query(query, userID)
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

func (r *UserRoleRepository) RemoveRoleFromUser(userID, roleName string) error {
	query := `
		DELETE FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'user' AND role_name = $2
	`
	_, err := r.db.Exec(query, userID, roleName)
	return err
}

func (r *UserRoleRepository) AssignRoleToServiceAccount(serviceAccountID, roleName, assignedBy string) error {
	query := `
		INSERT INTO user_roles (principal_id, principal_type, role_name, assigned_by)
		VALUES ($1, 'service_account', $2, $3)
		ON CONFLICT (principal_id, role_name) DO NOTHING
	`
	_, err := r.db.Exec(query, serviceAccountID, roleName, assignedBy)
	return err
}

func (r *UserRoleRepository) GetServiceAccountRoles(serviceAccountID string) ([]string, error) {
	query := `
		SELECT role_name 
		FROM user_roles 
		WHERE principal_id = $1 AND principal_type = 'service_account'
	`
	rows, err := r.db.Query(query, serviceAccountID)
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
