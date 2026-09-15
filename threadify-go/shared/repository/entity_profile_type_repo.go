package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxTypes = 5

var paramRegex = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])@([a-zA-Z0-9_]+)`)

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
		var values []string
		var args []interface{}
		n := 1
		for _, m := range profileType.Metrics {
			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			var name *string
			if m.Name != "" {
				name = &m.Name
			}
			var customDef interface{}
			if m.CustomDefinition != nil {
				customDef = m.CustomDefinition
			}
			values = append(values, "($"+fmt.Sprint(n)+", $"+fmt.Sprint(n+1)+", $"+fmt.Sprint(n+2)+", $"+fmt.Sprint(n+3)+"::jsonb, $"+fmt.Sprint(n+4)+"::jsonb)")
			args = append(args, profileType.ID, m.TemplateID, name, params, customDef)
			n += 5
		}
		query := "INSERT INTO entity_profile_type_metrics (entity_profile_type_id, metrics_template_id, name, parameters, custom_definition) VALUES " + strings.Join(values, ", ")
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("create entity profile type metrics: %w", err)
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
		SELECT m.id, m.entity_profile_type_id, m.metrics_template_id, m.name, t.metrics_name, m.parameters, m.custom_definition
		FROM entity_profile_type_metrics m
		LEFT JOIN metrics_template t ON t.id = m.metrics_template_id
		WHERE m.entity_profile_type_id = ANY($1)
	`
	rows, err := r.pool.Query(ctx, query, typeIDs)
	if err != nil {
		return fmt.Errorf("populate metrics: query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var metricID string
		var typeID, tmplID string
		var tmplName *string
		var customName *string
		var params map[string]any
		var customDefRaw []byte

		if err := rows.Scan(&metricID, &typeID, &tmplID, &customName, &tmplName, &params, &customDefRaw); err != nil {
			return fmt.Errorf("populate metrics: scan: %w", err)
		}

		name := ""
		if tmplName != nil && *tmplName != "" {
			name = *tmplName
		}
		if customName != nil && *customName != "" {
			name = *customName
		}

		var customDef *domain.MetricDefinition
		if len(customDefRaw) > 0 {
			customDef = &domain.MetricDefinition{}
			if err := json.Unmarshal(customDefRaw, customDef); err != nil {
				return fmt.Errorf("populate metrics: unmarshal custom_definition: %w", err)
			}
		}

		if pt, ok := typeMap[typeID]; ok {
			pt.Metrics = append(pt.Metrics, domain.EntityTypeMetric{
				ID:               metricID,
				TemplateID:       tmplID,
				Name:             name,
				Parameters:       params,
				CustomDefinition: customDef,
			})
		}
	}

	return rows.Err()
}

func (r *EntityProfileTypeRepo) GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*domain.EntityProfileType, error) {
	query := `
		SELECT id, company_id, name, slug, type, COALESCE(description, '') AS description, archived_at, created_at, updated_at
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
		SELECT id, company_id, name, slug, type, COALESCE(description, '') AS description, archived_at, created_at, updated_at
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
		SELECT id, company_id, name, slug, type, COALESCE(description, '') AS description, archived_at, created_at, updated_at
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

	// Delete only metrics marked for deletion
	if len(profileType.MarkedForDeletion) > 0 {
		// Collect custom template IDs to clean up
		var customTemplateIDs []string
		rows, err := tx.Query(ctx, `
			SELECT metrics_template_id
			FROM entity_profile_type_metrics
			WHERE id = ANY($1) AND metrics_template_id LIKE 'custom_%'
		`, profileType.MarkedForDeletion)
		if err != nil {
			return fmt.Errorf("update entity profile type: query custom templates: %w", err)
		}
		for rows.Next() {
			var tmplID string
			if err := rows.Scan(&tmplID); err != nil {
				rows.Close()
				return fmt.Errorf("update entity profile type: scan custom template: %w", err)
			}
			customTemplateIDs = append(customTemplateIDs, tmplID)
		}
		rows.Close()

		if _, err := tx.Exec(ctx, `
			DELETE FROM entity_profile_type_metrics
			WHERE id = ANY($1)
		`, profileType.MarkedForDeletion); err != nil {
			return fmt.Errorf("update entity profile type: delete marked metrics: %w", err)
		}

		// Clean up orphaned custom metrics_template rows
		if len(customTemplateIDs) > 0 {
			if _, err := tx.Exec(ctx, `
				DELETE FROM metrics_template
				WHERE id = ANY($1)
			`, customTemplateIDs); err != nil {
				return fmt.Errorf("update entity profile type: delete custom templates: %w", err)
			}
		}
	}

	// Update modified metrics — single bulk UPDATE
	var modifiedMetrics []domain.EntityTypeMetric
	for _, m := range profileType.Metrics {
		if m.ID != "" && slices.Contains(profileType.ModifiedMetricIDs, m.ID) {
			modifiedMetrics = append(modifiedMetrics, m)
		}
	}
	if len(modifiedMetrics) > 0 {
		var values []string
		var args []interface{}
		n := 1
		for _, m := range modifiedMetrics {
			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			var name *string
			if m.Name != "" {
				name = &m.Name
			}
			var customDef interface{}
			if m.CustomDefinition != nil {
				customDef = m.CustomDefinition
			}
			values = append(values, "($"+fmt.Sprint(n)+"::uuid, $"+fmt.Sprint(n+1)+", $"+fmt.Sprint(n+2)+", $"+fmt.Sprint(n+3)+"::jsonb, $"+fmt.Sprint(n+4)+"::jsonb)")
			args = append(args, m.ID, m.TemplateID, name, params, customDef)
			n += 5
		}
		query := `
			UPDATE entity_profile_type_metrics
			SET metrics_template_id = v.metrics_template_id,
			    name = v.name,
			    parameters = v.parameters,
			    custom_definition = v.custom_definition
			FROM (VALUES ` + strings.Join(values, ", ") + `) AS v(id, metrics_template_id, name, parameters, custom_definition)
			WHERE entity_profile_type_metrics.id = v.id
		`
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("update entity profile type: update metrics: %w", err)
		}
	}

	// Insert new metrics (those without an ID are new) — bulk insert
	var newMetrics []domain.EntityTypeMetric
	for _, m := range profileType.Metrics {
		if m.ID == "" {
			newMetrics = append(newMetrics, m)
		}
	}
	if len(newMetrics) > 0 {
		var values []string
		var args []interface{}
		n := 1
		for _, m := range newMetrics {
			params := m.Parameters
			if params == nil {
				params = make(map[string]any)
			}
			var name *string
			if m.Name != "" {
				name = &m.Name
			}
			var customDef interface{}
			if m.CustomDefinition != nil {
				customDef = m.CustomDefinition
			}
			values = append(values, "($"+fmt.Sprint(n)+", $"+fmt.Sprint(n+1)+", $"+fmt.Sprint(n+2)+", $"+fmt.Sprint(n+3)+"::jsonb, $"+fmt.Sprint(n+4)+"::jsonb)")
			args = append(args, profileType.ID, m.TemplateID, name, params, customDef)
			n += 5
		}
		query := "INSERT INTO entity_profile_type_metrics (entity_profile_type_id, metrics_template_id, name, parameters, custom_definition) VALUES " + strings.Join(values, ", ")
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return fmt.Errorf("update entity profile type: insert metrics: %w", err)
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

var paramCommentRegex = regexp.MustCompile(`(?m)^--\s*@param\s+([a-zA-Z0-9_]+)\s+(enum\([^)]+\)|[a-zA-Z0-9_]+)(?:\s+(.*))?$`)

func extractParameters(sqlQuery string) []domain.ParameterDef {
	ignoredParams := map[string]struct{}{
		"ref_value":  {},
		"ref_keys":   {},
		"start_time": {},
		"end_time":   {},
	}

	// 1. Parse explicit parameter definitions from comments
	defs := make(map[string]domain.ParameterDef)
	commentMatches := paramCommentRegex.FindAllStringSubmatch(sqlQuery, -1)
	for _, match := range commentMatches {
		if len(match) < 3 {
			continue
		}
		name := match[1]
		if _, ok := ignoredParams[name]; ok {
			continue
		}
		typeStr := match[2]
		desc := ""
		if len(match) > 3 {
			desc = match[3]
		}

		def := domain.ParameterDef{
			Name:        name,
			Type:        typeStr,
			Description: desc,
		}

		// Extract enum values
		if len(typeStr) > 5 && typeStr[:5] == "enum(" && typeStr[len(typeStr)-1] == ')' {
			inner := typeStr[5 : len(typeStr)-1]
			var values []string
			for _, v := range strings.Split(inner, ",") {
				values = append(values, strings.TrimSpace(v))
			}
			def.Type = "enum"
			def.Values = values
		}

		defs[name] = def
	}

	// 2. Fall back to regex-discovered @bindings for params without comments
	// Strip single-line comments to avoid matching @param inside -- comments
	var lines []string
	for _, line := range strings.Split(sqlQuery, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "--") {
			lines = append(lines, line)
		}
	}
	sqlWithoutComments := strings.Join(lines, "\n")

	matches := paramRegex.FindAllStringSubmatch(sqlWithoutComments, -1)
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		paramName := match[1]
		if _, ok := ignoredParams[paramName]; ok {
			continue
		}
		if _, ok := defs[paramName]; ok {
			continue
		}
		defs[paramName] = domain.ParameterDef{
			Name: paramName,
			Type: "string",
		}
	}

	// 3. Build ordered slice (preserve comment order, then fallback order)
	params := make([]domain.ParameterDef, 0)
	seen := make(map[string]struct{})
	for _, match := range commentMatches {
		name := match[1]
		if _, ok := seen[name]; ok {
			continue
		}
		if def, ok := defs[name]; ok {
			params = append(params, def)
			seen[name] = struct{}{}
		}
	}
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		name := match[1]
		if _, ok := seen[name]; ok {
			continue
		}
		if def, ok := defs[name]; ok {
			params = append(params, def)
			seen[name] = struct{}{}
		}
	}

	return params
}

func (r *EntityProfileTypeRepo) ListMetricsTemplates(ctx context.Context) ([]*domain.MetricsTemplate, error) {
	query := `
		SELECT id, metrics_name, sql_content
		FROM metrics_template
		WHERE is_system = false
		ORDER BY metrics_name ASC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics templates: %w", err)
	}
	defer rows.Close()

	var templates []*domain.MetricsTemplate
	for rows.Next() {
		var id, name, sqlContent string
		if err := rows.Scan(&id, &name, &sqlContent); err != nil {
			return nil, fmt.Errorf("failed to scan metrics template: %w", err)
		}

		templates = append(templates, &domain.MetricsTemplate{
			ID:                   id,
			MetricsName:          name,
			ParameterDefinitions: extractParameters(sqlContent),
			SQLContent:           sqlContent,
		})
	}

	return templates, rows.Err()
}

func (r *EntityProfileTypeRepo) CreateMetricsTemplate(ctx context.Context, companyID, id, name, sqlContent string) error {
	query := `
		INSERT INTO metrics_template (id, company_id, metrics_name, sql_content)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			company_id = EXCLUDED.company_id,
			metrics_name = EXCLUDED.metrics_name,
			sql_content = EXCLUDED.sql_content,
			updated_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, id, companyID, name, sqlContent)
	if err != nil {
		return fmt.Errorf("create metrics template: %w", err)
	}
	return nil
}

func (r *EntityProfileTypeRepo) UpdateMetricsTemplate(ctx context.Context, id, name, sqlContent string) error {
	query := `
		UPDATE metrics_template
		SET metrics_name = $1, sql_content = $2, updated_at = NOW()
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, name, sqlContent, id)
	if err != nil {
		return fmt.Errorf("update metrics template: %w", err)
	}
	return nil
}
