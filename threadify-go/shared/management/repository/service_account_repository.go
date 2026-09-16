package repository

import (
	"context"
	"database/sql"
	"errors"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type serviceAccountRepository struct {
	pool *pgxpool.Pool
}

func NewServiceAccountRepository(pool *pgxpool.Pool) domain.ServiceAccountRepository {
	return &serviceAccountRepository{pool: pool}
}

func (r *serviceAccountRepository) Create(ctx context.Context, sa *domain.ServiceAccount) error {
	query := `
		INSERT INTO service_accounts (id, company_id, name, description, is_active, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.pool.Exec(ctx, query,
		sa.ID, sa.CompanyID, sa.Name, sa.Description, sa.IsActive, sa.CreatedBy, sa.CreatedAt, sa.UpdatedAt)
	return err
}

func (r *serviceAccountRepository) FindByID(ctx context.Context, id string) (*domain.ServiceAccount, error) {
	query := `
		SELECT id, company_id, name, description, is_active, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE id = $1
	`
	var sa domain.ServiceAccount
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&sa.ID, &sa.CompanyID, &sa.Name, &sa.Description, &sa.IsActive, &sa.CreatedBy, &sa.LastUsedAt, &sa.CreatedAt, &sa.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, serror.ErrServiceAccountNotFound
		}
		return nil, err
	}
	return &sa, nil
}

func (r *serviceAccountRepository) FindByCompanyID(ctx context.Context, companyID string) ([]*domain.ServiceAccount, error) {
	query := `
		SELECT id, company_id, name, description, is_active, created_by, last_used_at, created_at, updated_at
		FROM service_accounts
		WHERE company_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []*domain.ServiceAccount
	for rows.Next() {
		var sa domain.ServiceAccount
		if err := rows.Scan(&sa.ID, &sa.CompanyID, &sa.Name, &sa.Description, &sa.IsActive, &sa.CreatedBy, &sa.LastUsedAt, &sa.CreatedAt, &sa.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, &sa)
	}
	return accounts, nil
}

func (r *serviceAccountRepository) Update(ctx context.Context, sa *domain.ServiceAccount) error {
	query := `
		UPDATE service_accounts
		SET name = $1, description = $2, is_active = $3, updated_at = $4
		WHERE id = $5
	`
	_, err := r.pool.Exec(ctx, query,
		sa.Name, sa.Description, sa.IsActive, sa.UpdatedAt, sa.ID)
	return err
}

func (r *serviceAccountRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM service_accounts WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *serviceAccountRepository) UpdateLastUsed(ctx context.Context, id string) error {
	query := `UPDATE service_accounts SET last_used_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

// IsServiceAccountActive checks if a service account exists and is active
func (r *serviceAccountRepository) IsServiceAccountActive(ctx context.Context, id string) (bool, error) {
	query := `SELECT is_active FROM service_accounts WHERE id = $1`
	var isActive bool
	err := r.pool.QueryRow(ctx, query, id).Scan(&isActive)
	if errors.Is(err, sql.ErrNoRows) {
		return false, serror.ErrServiceAccountNotFound
	}
	if err != nil {
		return false, err
	}
	return isActive, nil
}
