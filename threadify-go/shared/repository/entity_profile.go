package repository

import (
	"context"
	"fmt"
	"threadify-go/shared/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EntityProfileRepo struct {
	pool *pgxpool.Pool
}

func NewEntityProfileRepo(pool *pgxpool.Pool) *EntityProfileRepo {
	return &EntityProfileRepo{pool: pool}
}

func (r *EntityProfileRepo) CreateProfile(ctx context.Context, profile *domain.EntityProfile) error {
	query := `
		INSERT INTO entity_profile (id, company_id, entity_profile_type_id, name, ref_key)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (company_id, entity_profile_type_id, ref_key) DO UPDATE SET
			name = EXCLUDED.name,
			last_active_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, profile.ID, profile.CompanyID, profile.ProfileTypeID, profile.Name, profile.RefKey)
	return err
}

func (r *EntityProfileRepo) GetProfileByRefKey(ctx context.Context, companyID, profileTypeID, refKey string) (*domain.EntityProfile, error) {
	query := `
		SELECT id, company_id, entity_profile_type_id, name, ref_key, created_at, last_active_at 
		FROM entity_profile 
		WHERE company_id = $1 AND entity_profile_type_id = $2 AND ref_key = $3
	`
	row := r.pool.QueryRow(ctx, query, companyID, profileTypeID, refKey)
	var profile domain.EntityProfile
	err := row.Scan(&profile.ID, &profile.CompanyID, &profile.ProfileTypeID, &profile.Name, &profile.RefKey, &profile.CreatedAt, &profile.LastActiveAt)
	return &profile, err
}

func (r *EntityProfileRepo) GetProfileByID(ctx context.Context, companyID, profileID string) (*domain.EntityProfile, error) {
	query := `
		SELECT id, company_id, entity_profile_type_id, name, ref_key, created_at, last_active_at 
		FROM entity_profile 
		WHERE company_id = $1 AND id = $2
	`
	row := r.pool.QueryRow(ctx, query, companyID, profileID)
	var profile domain.EntityProfile
	err := row.Scan(&profile.ID, &profile.CompanyID, &profile.ProfileTypeID, &profile.Name, &profile.RefKey, &profile.CreatedAt, &profile.LastActiveAt)
	return &profile, err
}

func (r *EntityProfileRepo) GetProfileByTypeName(ctx context.Context, companyID, typeSlug, refKey string) (*domain.EntityProfile, error) {
	query := `
		SELECT ep.id, ep.company_id, ep.entity_profile_type_id, ep.name, ep.ref_key, ep.created_at, ep.last_active_at 
		FROM entity_profile ep
		JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id
		WHERE ep.company_id = $1 AND ept.slug = $2 AND ep.ref_key = $3
	`
	row := r.pool.QueryRow(ctx, query, companyID, typeSlug, refKey)
	var profile domain.EntityProfile
	err := row.Scan(&profile.ID, &profile.CompanyID, &profile.ProfileTypeID, &profile.Name, &profile.RefKey, &profile.CreatedAt, &profile.LastActiveAt)
	return &profile, err
}

func (r *EntityProfileRepo) ListProfilesByType(ctx context.Context, companyID, typeName, search string, limit, offset int) ([]*domain.EntityProfile, int, error) {
	const query = `
			SELECT
	    ep.id, ep.company_id, ep.entity_profile_type_id, ep.name, ep.ref_key, ep.created_at, ep.last_active_at,
	    COUNT(*) OVER() AS total_count
			FROM entity_profile ep
			JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id
			WHERE ep.company_id = $1
			AND ept.slug = $2
			AND ept.archived_at IS NULL
			AND (NOT $3 OR ep.name ILIKE $4 OR ep.ref_key ILIKE $4)
			ORDER BY ep.last_active_at DESC
			LIMIT $5 OFFSET $6
	`

	hasSearch := search != ""
	var searchPattern string
	if hasSearch {
		searchPattern = "%" + search + "%"
	}

	rows, err := r.pool.Query(ctx, query,
		companyID,
		typeName,
		hasSearch,
		searchPattern,
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list profiles by type: query: %w", err)
	}
	defer rows.Close()

	var (
		results []*domain.EntityProfile
		total   int
	)
	for rows.Next() {
		var p domain.EntityProfile

		if err := rows.Scan(
			&p.ID, &p.CompanyID, &p.ProfileTypeID, &p.Name, &p.RefKey, &p.CreatedAt, &p.LastActiveAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("list profiles by type: %w", err)
		}

		results = append(results, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list profiles by type: rows: %w", err)
	}
	return results, total, nil
}
