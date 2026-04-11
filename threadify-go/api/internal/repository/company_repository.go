package repository

import (
	"context"
	"fmt"

	"threadify-go/api/internal/models"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CompanyRepository struct {
	pool *pgxpool.Pool
}

func NewCompanyRepository(pool *pgxpool.Pool) *CompanyRepository {
	return &CompanyRepository{pool: pool}
}

type companyExecer interface {
	Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
}

func (r *CompanyRepository) CreateTx(ctx context.Context, execer companyExecer, company *models.Company) error {
	const query = `
        INSERT INTO companies (id, name, industry, size, use_case, created_at, updated_at)
        VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NOW(), NOW())
    `
	_, err := execer.Exec(ctx, query, company.ID, company.Name, company.Industry, company.Size, company.UseCase)
	if err != nil {
		return fmt.Errorf("create company: %w", err)
	}
	return nil
}

func (r *CompanyRepository) FindByID(ctx context.Context, id string) (*models.Company, error) {
	company := &models.Company{}
	const query = `
        SELECT id, name, industry, size, use_case, created_at, updated_at
        FROM companies WHERE id = $1
    `
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&company.ID, &company.Name, &company.Industry, &company.Size,
		&company.UseCase, &company.CreatedAt, &company.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("find company by id: %w", err)
	}
	return company, nil
}

func (r *CompanyRepository) UpdateDetails(ctx context.Context, id string, industry, size, useCase *string) error {
	const query = `
        UPDATE companies
        SET industry = COALESCE($1, industry),
            size = COALESCE($2, size),
            use_case = COALESCE($3, use_case),
            updated_at = NOW()
        WHERE id = $4
    `
	_, err := r.pool.Exec(ctx, query, industry, size, useCase, id)
	if err != nil {
		return fmt.Errorf("update company details: %w", err)
	}
	return nil
}
func (r *CompanyRepository) Delete(ctx context.Context, id string) error {
	return r.DeleteTx(ctx, r.pool, id)
}

func (r *CompanyRepository) DeleteTx(ctx context.Context, execer companyExecer, id string) error {
	_, err := execer.Exec(ctx, `DELETE FROM companies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete company: %w", err)
	}
	return nil
}
