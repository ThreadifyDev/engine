package repository

import (
	"context"
	"errors"
	"fmt"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/models"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EntityProfileTypeRepo struct {
	pool *pgxpool.Pool
}

func NewEntityProfileTypeRepository(pool *pgxpool.Pool) *EntityProfileTypeRepo {
	return &EntityProfileTypeRepo{pool: pool}
}

func (r *EntityProfileTypeRepo) CreateProfileType(ctx context.Context, profileType *models.EntityProfileType) error {
	query := `
		INSERT INTO entity_profile_type (id, company_id, name, type, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.pool.Exec(ctx, query, profileType.ID, profileType.CompanyID, profileType.Name, profileType.Type, profileType.Description, profileType.CreatedAt, profileType.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return serror.ErrEntityProfileTypeAlreadyExists
		}
		return fmt.Errorf("create entity profile type: %w", err)
	}
	return nil
}

func (r *EntityProfileTypeRepo) GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*models.EntityProfileType, error) {
	query := `
	SELECT id, company_id, name, type, description, archived_at, created_at, updated_at FROM entity_profile_type WHERE company_id = $1 AND archived_at IS NULL
	`
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []*models.EntityProfileType
	for rows.Next() {
		var pt models.EntityProfileType
		if err := rows.Scan(&pt.ID, &pt.CompanyID, &pt.Name, &pt.Type, &pt.Description, &pt.ArchivedAt, &pt.CreatedAt, &pt.UpdatedAt); err != nil {
			return nil, err
		}
		types = append(types, &pt)
	}
	return types, rows.Err()
}

func (r *EntityProfileTypeRepo) GetProfileTypeByType(ctx context.Context, companyID, profileType string) (*models.EntityProfileType, error) {
	query := `
	SELECT id, company_id, name, type, description, archived_at, created_at, updated_at FROM entity_profile_type WHERE company_id = $1 AND type = $2 AND archived_at IS NULL
	`
	row := r.pool.QueryRow(ctx, query, companyID, profileType)
	var pt models.EntityProfileType
	err := row.Scan(&pt.ID, &pt.CompanyID, &pt.Name, &pt.Type, &pt.Description, &pt.ArchivedAt, &pt.CreatedAt, &pt.UpdatedAt)
	return &pt, err
}

func (r *EntityProfileTypeRepo) UpdateProfileType(ctx context.Context, profileType *models.EntityProfileType) error {
	query := `
		UPDATE entity_profile_type SET
			name = $1,
			description = $2,
			updated_at = NOW()
		WHERE id = $3 AND company_id = $4
		RETURNING id, company_id, name, type, description, archived_at, created_at, updated_at
	`
	row := r.pool.QueryRow(ctx, query, profileType.Name, profileType.Description, profileType.ID, profileType.CompanyID)
	return row.Scan(&profileType.ID, &profileType.CompanyID, &profileType.Name, &profileType.Type, &profileType.Description, &profileType.ArchivedAt, &profileType.CreatedAt, &profileType.UpdatedAt)
}

func (r *EntityProfileTypeRepo) ArchiveProfileType(ctx context.Context, companyID, profileTypeID string) error {
	query := `
		UPDATE entity_profile_type SET
			archived_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND company_id = $2
	`
	_, err := r.pool.Exec(ctx, query, profileTypeID, companyID)
	return err
}
