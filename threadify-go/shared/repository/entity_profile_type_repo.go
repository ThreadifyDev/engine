package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/domain"

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

func (r *EntityProfileTypeRepo) CreateProfileType(ctx context.Context, profileType *domain.EntityProfileType) error {
	if len(profileType.Type) > maxTypes {
		return serror.ErrEntityProfileTypeExceedsMaxTypes
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("create entity profile type: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO entity_profile_type (id, company_id, name, slug, type, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	if _, err := tx.Exec(ctx, query,
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

	if len(profileType.Metrics) > 0 {
		metricQuery := `
			INSERT INTO entity_profile_type_metrics (entity_profile_type_id, metrics_template_id, name, parameters)
			VALUES ($1, $2, $3, $4)
		`
		for _, m := range profileType.Metrics {
			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			var name *string
			if m.Name != "" {
				name = &m.Name
			}
			if _, err := tx.Exec(ctx, metricQuery, profileType.ID, m.TemplateID, name, params); err != nil {
				return fmt.Errorf("create entity profile type metrics: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *EntityProfileTypeRepo) populateMetrics(ctx context.Context, profileTypes []*domain.EntityProfileType) error {
	if len(profileTypes) == 0 {
		return nil
	}

	var typeIDs []string
	typeMap := make(map[string]*domain.EntityProfileType)
	for _, pt := range profileTypes {
		typeIDs = append(typeIDs, pt.ID)
		typeMap[pt.ID] = pt
	}

	query := `
		SELECT m.entity_profile_type_id, m.metrics_template_id, m.name, t.metrics_name, m.parameters 
		FROM entity_profile_type_metrics m
		JOIN metrics_template t ON t.id = m.metrics_template_id
		WHERE m.entity_profile_type_id = ANY($1)
	`
	rows, err := r.pool.Query(ctx, query, typeIDs)
	if err != nil {
		return fmt.Errorf("populate metrics: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var typeID, tmplID, tmplName string
		var customName *string
		var params map[string]any

		if err := rows.Scan(&typeID, &tmplID, &customName, &tmplName, &params); err != nil {
			return fmt.Errorf("populate metrics: scan: %w", err)
		}

		name := tmplName
		if customName != nil && *customName != "" {
			name = *customName
		}

		if pt, ok := typeMap[typeID]; ok {
			pt.Metrics = append(pt.Metrics, domain.EntityTypeMetric{
				TemplateID: tmplID,
				Name:       name,
				Parameters: params,
			})
		}
	}

	return rows.Err()
}

func (r *EntityProfileTypeRepo) GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*domain.EntityProfileType, error) {
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

	var profileTypes []*domain.EntityProfileType
	for rows.Next() {
		var pt domain.EntityProfileType
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

	if err := r.populateMetrics(ctx, profileTypes); err != nil {
		return nil, err
	}

	return profileTypes, nil
}

func (r *EntityProfileTypeRepo) GetProfileTypeByID(ctx context.Context, profileTypeID string) (*domain.EntityProfileType, error) {
	query := `
		SELECT id, company_id, name, slug, type, description, archived_at, created_at, updated_at
		FROM entity_profile_type
		WHERE id = $1 AND archived_at IS NULL
	`
	var pt domain.EntityProfileType
	if err := r.pool.QueryRow(ctx, query, profileTypeID).Scan(
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
		return nil, fmt.Errorf("get entity profile type by id: %w", err)
	}

	if err := r.populateMetrics(ctx, []*domain.EntityProfileType{&pt}); err != nil {
		return nil, err
	}

	return &pt, nil
}

func (r *EntityProfileTypeRepo) GetProfileTypeByType(ctx context.Context, companyID, profileType string) (*domain.EntityProfileType, error) {
	query := `
		SELECT id, company_id, name, slug, type, description, archived_at, created_at, updated_at
		FROM entity_profile_type
		WHERE company_id = $1 AND slug = $2 AND archived_at IS NULL
	`
	var pt domain.EntityProfileType
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

	if err := r.populateMetrics(ctx, []*domain.EntityProfileType{&pt}); err != nil {
		return nil, err
	}

	return &pt, nil
}

func (r *EntityProfileTypeRepo) UpdateProfileType(ctx context.Context, profileType *domain.EntityProfileType) error {
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

	// Update metrics (full replacement)
	if _, err := tx.Exec(ctx, `
		DELETE FROM entity_profile_type_metrics
		WHERE entity_profile_type_id = $1
	`, profileType.ID); err != nil {
		return fmt.Errorf("update entity profile type: delete metrics: %w", err)
	}

	if len(profileType.Metrics) > 0 {
		metricQuery := `
			INSERT INTO entity_profile_type_metrics (entity_profile_type_id, metrics_template_id, name, parameters)
			VALUES ($1, $2, $3, $4)
		`
		for _, m := range profileType.Metrics {
			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			var name *string
			if m.Name != "" {
				name = &m.Name
			}
			if _, err := tx.Exec(ctx, metricQuery, profileType.ID, m.TemplateID, name, params); err != nil {
				return fmt.Errorf("update entity profile type: insert metric: %w", err)
			}
		}
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

func (r *EntityProfileTypeRepo) ListMetricsTemplates(ctx context.Context) ([]domain.MetricsTemplateResponse, error) {
	query := `
		SELECT id, metrics_name, sql_content
		FROM metrics_template
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list metrics templates: %w", err)
	}
	defer rows.Close()

	paramRegex := regexp.MustCompile(`@([a-zA-Z0-9_]+)`)
	ignoredParams := map[string]struct{}{
		"ref_value":  {},
		"ref_keys":   {},
		"start_time": {},
		"end_time":   {},
	}

	var templates []domain.MetricsTemplateResponse
	for rows.Next() {
		var t domain.MetricsTemplateResponse
		if err := rows.Scan(&t.ID, &t.MetricsName, &t.SQLContent); err != nil {
			return nil, fmt.Errorf("list metrics templates: scan: %w", err)
		}

		// Extract parameters from SQL content
		matches := paramRegex.FindAllStringSubmatch(t.SQLContent, -1)
		var params []string
		seen := make(map[string]struct{})
		for _, m := range matches {
			if len(m) > 1 {
				p := m[1]
				if _, ok := ignoredParams[p]; !ok {
					if _, alreadySeen := seen[p]; !alreadySeen {
						seen[p] = struct{}{}
						params = append(params, p)
					}
				}
			}
		}
		t.Parameters = params
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list metrics templates: rows: %w", err)
	}

	return templates, nil
}
