package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"threadify-go/api/internal/models"
)

type CompanyRepository struct {
	db *sql.DB
}

func NewCompanyRepository(db *sql.DB) *CompanyRepository {
	return &CompanyRepository{db: db}
}

type companyExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (r *CompanyRepository) CreateTx(execer companyExecer, company *models.Company) error {
	const query = `
        INSERT INTO companies (id, name, industry, size, use_case, created_at, updated_at)
        VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NOW(), NOW())
    `
	_, err := execer.Exec(query, company.ID, company.Name, company.Industry, company.Size, company.UseCase)
	if err != nil {
		return fmt.Errorf("create company: %w", err)
	}
	return nil
}

func (r *CompanyRepository) FindByID(id string) (*models.Company, error) {
	company := &models.Company{}
	const query = `
        SELECT id, name, industry, size, use_case, created_at, updated_at
        FROM companies WHERE id = $1
    `
	err := r.db.QueryRow(query, id).Scan(
		&company.ID, &company.Name, &company.Industry, &company.Size,
		&company.UseCase, &company.CreatedAt, &company.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find company by id: %w", err)
	}
	return company, nil
}

func (r *CompanyRepository) UpdateDetails(id string, industry, size, useCase *string) error {
	const query = `
        UPDATE companies
        SET industry = COALESCE($1, industry),
            size = COALESCE($2, size),
            use_case = COALESCE($3, use_case),
            updated_at = NOW()
        WHERE id = $4
    `
	_, err := r.db.Exec(query, industry, size, useCase, id)
	if err != nil {
		return fmt.Errorf("update company details: %w", err)
	}
	return nil
}
