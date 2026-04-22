package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/models"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxTypes = 5

type EntityProfileTypeRepo struct {
	pool *pgxpool.Pool
}

func NewEntityProfileTypeRepository(pool *pgxpool.Pool) *EntityProfileTypeRepo {
	return &EntityProfileTypeRepo{pool: pool}
}

func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return serror.ErrEntityProfileTypeAlreadyExists
	}
	return nil
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	slices.Sort(out)
	return out
}

func (r *EntityProfileTypeRepo) CreateProfileType(ctx context.Context, profileType *models.EntityProfileType) error {
	if len(profileType.Type) > maxTypes {
		return serror.ErrEntityProfileTypeExceedsMaxTypes
	}
	query := `
		INSERT INTO entity_profile_type (id, company_id, name, slug, type, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	if _, err := r.pool.Exec(ctx, query,
		profileType.ID,
		profileType.CompanyID,
		profileType.Name,
		profileType.Slug,
		profileType.Type,
		profileType.Description,
		profileType.CreatedAt,
		profileType.UpdatedAt,
	); err != nil {
		if mapped := mapUniqueViolation(err); mapped != nil {
			return mapped
		}
		return fmt.Errorf("create entity profile type: %w", err)
	}
	return nil
}

func (r *EntityProfileTypeRepo) GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*models.EntityProfileType, error) {
	query := `
		SELECT id, company_id, name, slug, type, description, archived_at, created_at, updated_at
		FROM entity_profile_type
		WHERE company_id = $1 AND archived_at IS NULL
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, fmt.Errorf("get entity profile types: %w", err)
	}
	defer rows.Close()

	var profileTypes []*models.EntityProfileType
	for rows.Next() {
		var pt models.EntityProfileType
		if err := rows.Scan(
			&pt.ID,
			&pt.CompanyID,
			&pt.Name,
			&pt.Slug,
			&pt.Type,
			&pt.Description,
			&pt.ArchivedAt,
			&pt.CreatedAt,
			&pt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("get entity profile types: scan: %w", err)
		}
		profileTypes = append(profileTypes, &pt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get entity profile types: rows: %w", err)
	}
	return profileTypes, nil
}

func (r *EntityProfileTypeRepo) GetProfileTypeByType(ctx context.Context, companyID, profileType string) (*models.EntityProfileType, error) {
	query := `
		SELECT id, company_id, name, slug, type, description, archived_at, created_at, updated_at
		FROM entity_profile_type
		WHERE company_id = $1 AND slug = $2 AND archived_at IS NULL
	`
	var pt models.EntityProfileType
	if err := r.pool.QueryRow(ctx, query, companyID, profileType).Scan(
		&pt.ID,
		&pt.CompanyID,
		&pt.Name,
		&pt.Slug,
		&pt.Type,
		&pt.Description,
		&pt.ArchivedAt,
		&pt.CreatedAt,
		&pt.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, serror.ErrEntityProfileTypeNotFound
		}
		return nil, fmt.Errorf("get entity profile type by type: %w", err)
	}
	return &pt, nil
}

func (r *EntityProfileTypeRepo) UpdateProfileType(ctx context.Context, profileType *models.EntityProfileType) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("update entity profile type: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentTypes []string
	if err := tx.QueryRow(ctx, `
			SELECT type FROM entity_profile_type
			WHERE id = $1 AND company_id = $2 AND archived_at IS NULL
			FOR UPDATE
		`, profileType.ID, profileType.CompanyID).Scan(&currentTypes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return serror.ErrEntityProfileTypeNotFound
		}
		return fmt.Errorf("update entity profile type: lock: %w", err)
	}

	nextTypes := currentTypes
	if profileType.Type != nil {
		nextTypes = dedupe(profileType.Type)
	}
	if len(nextTypes) > maxTypes {
		return serror.ErrEntityProfileTypeExceedsMaxTypes
	}

	if err := tx.QueryRow(ctx, `
			UPDATE entity_profile_type SET
				name        = $1,
				slug        = $2,
				description = $3,
				type        = $4,
				updated_at  = NOW()
			WHERE id = $5 AND company_id = $6
			RETURNING id, company_id, name, slug, type, description, archived_at, created_at, updated_at
		`,
		profileType.Name,
		profileType.Slug,
		profileType.Description,
		nextTypes,
		profileType.ID,
		profileType.CompanyID,
	).Scan(
		&profileType.ID,
		&profileType.CompanyID,
		&profileType.Name,
		&profileType.Slug,
		&profileType.Type,
		&profileType.Description,
		&profileType.ArchivedAt,
		&profileType.CreatedAt,
		&profileType.UpdatedAt,
	); err != nil {
		return fmt.Errorf("update entity profile type: update: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *EntityProfileTypeRepo) ArchiveProfileType(ctx context.Context, companyID, profileTypeID string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE entity_profile_type SET
			archived_at = NOW(),
			updated_at  = NOW()
		WHERE id = $1 AND company_id = $2 AND archived_at IS NULL
	`, profileTypeID, companyID)
	if err != nil {
		return fmt.Errorf("archive entity profile type: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return serror.ErrEntityProfileTypeNotFound
	}
	return nil
}
