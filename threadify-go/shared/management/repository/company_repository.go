package repository

import (
	"context"
	"fmt"

	"threadify-go/shared/management/domain"
	"threadify-go/shared/management/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

type companyRepository struct {
	pool *pgxpool.Pool
}

func NewCompanyRepository(pool *pgxpool.Pool) domain.CompanyRepository {
	return &companyRepository{pool: pool}
}

func (r *companyRepository) CreateTx(ctx context.Context, tx domain.ExecContext, company *domain.Company) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}
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

func (r *companyRepository) FindByID(ctx context.Context, id string) (*domain.Company, error) {
	company := &domain.Company{}
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

func (r *companyRepository) UpdateDetails(ctx context.Context, id string, industry, size, useCase *string) error {
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
func (r *companyRepository) Delete(ctx context.Context, id string) error {
	return r.DeleteTx(ctx, r.pool, id)
}

func (r *companyRepository) DeleteTx(ctx context.Context, tx domain.ExecContext, id string) error {
	execer, ok := tx.(ports.SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid execer type: expected ports.SQLExecutor, got %T", tx)
	}
	_, err := execer.Exec(ctx, `DELETE FROM companies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete company: %w", err)
	}
	return nil
}
