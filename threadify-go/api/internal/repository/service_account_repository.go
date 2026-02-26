package repository

import (
	"database/sql"
	"threadify-go/api/internal/models"
	serror "threadify-go/shared/errors"
)

type ServiceAccountRepository struct {
	db *sql.DB
}

func NewServiceAccountRepository(db *sql.DB) *ServiceAccountRepository {
	return &ServiceAccountRepository{db: db}
}

func (r *ServiceAccountRepository) Create(sa *models.ServiceAccount) error {
	query := `
		INSERT INTO service_accounts (id, company_id, name, description, is_active, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Exec(query,
		sa.ID, sa.CompanyID, sa.Name, sa.Description, sa.IsActive, sa.CreatedBy, sa.CreatedAt, sa.UpdatedAt)
	return err
}

func (r *ServiceAccountRepository) FindByID(id string) (*models.ServiceAccount, error) {
	query := `
		SELECT id, company_id, name, description, is_active, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE id = $1
	`
	var sa models.ServiceAccount
	err := r.db.QueryRow(query, id).Scan(
		&sa.ID, &sa.CompanyID, &sa.Name, &sa.Description, &sa.IsActive, &sa.CreatedBy, &sa.LastUsedAt, &sa.CreatedAt, &sa.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, serror.ErrServiceAccountNotFound
		}
		return nil, err
	}
	return &sa, nil
}

func (r *ServiceAccountRepository) FindByCompanyID(companyID string) ([]*models.ServiceAccount, error) {
	query := `
		SELECT id, company_id, name, description, is_active, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE company_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*models.ServiceAccount
	for rows.Next() {
		var sa models.ServiceAccount
		if err := rows.Scan(&sa.ID, &sa.CompanyID, &sa.Name, &sa.Description, &sa.IsActive, &sa.CreatedBy, &sa.LastUsedAt, &sa.CreatedAt, &sa.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, &sa)
	}
	return accounts, nil
}

func (r *ServiceAccountRepository) Update(sa *models.ServiceAccount) error {
	query := `
		UPDATE service_accounts
		SET name = $1, description = $2, is_active = $3, updated_at = $4
		WHERE id = $5
	`
	_, err := r.db.Exec(query,
		sa.Name, sa.Description, sa.IsActive, sa.UpdatedAt, sa.ID)
	return err
}

func (r *ServiceAccountRepository) Delete(id string) error {
	query := `DELETE FROM service_accounts WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *ServiceAccountRepository) UpdateLastUsed(id string) error {
	query := `UPDATE service_accounts SET last_used_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

// IsServiceAccountActive checks if a service account exists and is active
func (r *ServiceAccountRepository) IsServiceAccountActive(id string) (bool, error) {
	query := `SELECT is_active FROM service_accounts WHERE id = $1`
	var isActive bool
	err := r.db.QueryRow(query, id).Scan(&isActive)
	if err == sql.ErrNoRows {
		return false, serror.ErrServiceAccountNotFound
	}
	if err != nil {
		return false, err
	}
	return isActive, nil
}

// GetPermissions is deprecated - use role-based permissions instead
func (r *ServiceAccountRepository) GetPermissions(roleStr string) ([]models.Permission, error) {
	return nil, nil
}

// HasPermission is deprecated - use role-based permissions instead
func (r *ServiceAccountRepository) HasPermission(roleStr, resource, action string) (bool, error) {
	return false, nil
}
