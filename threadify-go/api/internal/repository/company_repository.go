package repository

import (
	"database/sql"
	"threadify-go/api/internal/models"
)

type CompanyRepository struct {
	db *sql.DB
}

func NewCompanyRepository(db *sql.DB) *CompanyRepository {
	return &CompanyRepository{db: db}
}

func (r *CompanyRepository) Create(company *models.Company) error {
	query := `
		INSERT INTO companies (id, name, industry, size, use_case, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NOW(), NOW())
	`
	_, err := r.db.Exec(query, company.ID, company.Name, company.Industry, company.Size, company.UseCase)
	return err
}

func (r *CompanyRepository) FindByID(id string) (*models.Company, error) {
	company := &models.Company{}
	query := `
		SELECT id, name, industry, size, use_case, created_at, updated_at
		FROM companies WHERE id = $1
	`
	err := r.db.QueryRow(query, id).Scan(
		&company.ID, &company.Name, &company.Industry, &company.Size,
		&company.UseCase, &company.CreatedAt, &company.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return company, err
}

func (r *CompanyRepository) UpdateDetails(id string, industry, size, useCase *string) error {
	query := `
		UPDATE companies 
		SET industry = COALESCE($1, industry), 
		    size = COALESCE($2, size), 
		    use_case = COALESCE($3, use_case), 
		    updated_at = NOW() 
		WHERE id = $4
	`
	_, err := r.db.Exec(query, industry, size, useCase, id)
	return err
}
