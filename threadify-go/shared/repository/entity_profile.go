package repository

import (
	"context"
	"database/sql"
	"fmt"
	"threadify-go/shared/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EntityProfileRepo struct {
	pool *pgxpool.Pool
}

func NewEntityProfileRepo(pool *pgxpool.Pool) *EntityProfileRepo {
	return &EntityProfileRepo{pool: pool}
}

func (r *EntityProfileRepo) CreateProfile(ctx context.Context, profile *models.EntityProfile) error {
	query := `
		INSERT INTO entity_profile (id, company_id, entity_profile_type_id, name, ref_key)
		VALUES ($1, $2, $3, $4, $5) ON CONFLICT (company_id, entity_profile_type_id, ref_key) DO UPDATE SET
			name = EXCLUDED.name,
			last_active_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, profile.ID, profile.CompanyID, profile.ProfileTypeID, profile.Name, profile.RefKey)
	return err
}

func (r *EntityProfileRepo) GetProfileByRefKey(ctx context.Context, companyID, profileTypeID, refKey string) (*models.EntityProfile, error) {
	query := `
		SELECT id, company_id, entity_profile_type_id, name, ref_key, created_at, last_active_at 
		FROM entity_profile 
		WHERE company_id = $1 AND entity_profile_type_id = $2 AND ref_key = $3
	`
	row := r.pool.QueryRow(ctx, query, companyID, profileTypeID, refKey)
	var profile models.EntityProfile
	err := row.Scan(&profile.ID, &profile.CompanyID, &profile.ProfileTypeID, &profile.Name, &profile.RefKey, &profile.CreatedAt, &profile.LastActiveAt)
	return &profile, err
}

func (r *EntityProfileRepo) GetProfileMetrics(ctx context.Context, entityProfileID string) (*models.EntityProfileMetrics, error) {
	query := `
		SELECT entity_profile_id, total_deliveries, completed_successfully, validation_violations, 
		       delivery_health_score, prev_delivery_health_score, health_trend_slope, average_delivery_time_ms, last_calculated_at 
		FROM entity_profile_metrics 
		WHERE entity_profile_id = $1
	`
	row := r.pool.QueryRow(ctx, query, entityProfileID)
	var m models.EntityProfileMetrics
	err := row.Scan(&m.EntityProfileID, &m.TotalDeliveries, &m.CompletedSuccessfully, &m.ValidationViolations,
		&m.DeliveryHealthScore, &m.PrevDeliveryHealthScore, &m.HealthTrendSlope, &m.AverageDeliveryTimeMs, &m.LastCalculatedAt)
	return &m, err
}

// ProfileWithMetrics bundles a profile row with its (optional) metrics.
type ProfileWithMetrics struct {
	Profile *models.EntityProfile
	Metrics *models.EntityProfileMetrics
}

func (r *EntityProfileRepo) ListProfilesByType(ctx context.Context, companyID, typeName, search string, limit, offset int) ([]*ProfileWithMetrics, int, error) {
	const query = `
			SELECT
	    ep.id, ep.company_id, ep.entity_profile_type_id, ep.name, ep.ref_key, ep.created_at, ep.last_active_at,
	    epm.entity_profile_id, epm.total_deliveries, epm.completed_successfully, epm.validation_violations,
	    epm.delivery_health_score, epm.prev_delivery_health_score, epm.health_trend_slope,
	    epm.average_delivery_time_ms, epm.last_calculated_at,
	    COUNT(*) OVER() AS total_count
			FROM entity_profile ep
			JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id
			LEFT JOIN entity_profile_metrics epm ON epm.entity_profile_id = ep.id
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
		results []*ProfileWithMetrics
		total   int
	)
	for rows.Next() {
		var (
			p               models.EntityProfile
			m               models.EntityProfileMetrics
			metricsID       sql.NullString
			totalDeliveries sql.NullInt64
			completed       sql.NullInt64
			violations      sql.NullInt64
			healthScore     sql.NullFloat64
			prevHealthScore sql.NullFloat64
			trendSlope      sql.NullFloat64
			avgDeliveryMs   sql.NullInt64
			lastCalc        sql.NullTime
		)

		if err := rows.Scan(
			&p.ID, &p.CompanyID, &p.ProfileTypeID, &p.Name, &p.RefKey, &p.CreatedAt, &p.LastActiveAt,
			&metricsID, &totalDeliveries, &completed, &violations,
			&healthScore, &prevHealthScore, &trendSlope, &avgDeliveryMs, &lastCalc,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("list profiles by type: %w", err)
		}

		item := &ProfileWithMetrics{Profile: &p}
		if metricsID.Valid {
			m.EntityProfileID = metricsID.String
			m.TotalDeliveries = int(totalDeliveries.Int64)
			m.CompletedSuccessfully = int(completed.Int64)
			m.ValidationViolations = int(violations.Int64)
			m.DeliveryHealthScore = &healthScore.Float64
			m.PrevDeliveryHealthScore = &prevHealthScore.Float64
			m.HealthTrendSlope = &trendSlope.Float64
			m.AverageDeliveryTimeMs = &avgDeliveryMs.Int64
			m.LastCalculatedAt = &lastCalc.Time
			item.Metrics = &m
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("list profiles by type: rows: %w", err)
	}
	return results, total, nil
}

func (r *EntityProfileRepo) GetProfileWithMetrics(ctx context.Context, companyID, typeName, refKey string) (*models.EntityProfile, *models.EntityProfileMetrics, error) {
	query := `
			SELECT
				ep.id, ep.company_id, ep.entity_profile_type_id, ep.name, ep.ref_key, ep.created_at, ep.last_active_at,
				epm.entity_profile_id, epm.total_deliveries, epm.completed_successfully, epm.validation_violations,
				epm.delivery_health_score, epm.prev_delivery_health_score, epm.health_trend_slope, epm.average_delivery_time_ms, epm.last_calculated_at
			FROM entity_profile ep
			JOIN entity_profile_type ept ON ept.id = ep.entity_profile_type_id
			LEFT JOIN entity_profile_metrics epm ON epm.entity_profile_id = ep.id
			WHERE ep.company_id = $1 AND ept.slug = $2 AND ep.ref_key = $3 AND ept.archived_at IS NULL
		`
	row := r.pool.QueryRow(ctx, query, companyID, typeName, refKey)

	var p models.EntityProfile
	var m models.EntityProfileMetrics
	var metricsID *string

	var totalDeliveries, completed, validations *int

	err := row.Scan(
		&p.ID, &p.CompanyID, &p.ProfileTypeID, &p.Name, &p.RefKey, &p.CreatedAt, &p.LastActiveAt,
		&metricsID, &totalDeliveries, &completed, &validations,
		&m.DeliveryHealthScore, &m.PrevDeliveryHealthScore, &m.HealthTrendSlope, &m.AverageDeliveryTimeMs, &m.LastCalculatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	if metricsID != nil {
		m.EntityProfileID = *metricsID
		if totalDeliveries != nil {
			m.TotalDeliveries = *totalDeliveries
		}
		if completed != nil {
			m.CompletedSuccessfully = *completed
		}
		if validations != nil {
			m.ValidationViolations = *validations
		}
		return &p, &m, nil
	}
	return &p, nil, nil
}

func (r *EntityProfileRepo) GetProfileByIDWithMetrics(ctx context.Context, companyID, entityProfileID string) (*models.EntityProfile, *models.EntityProfileMetrics, error) {
	query := `
		SELECT
			ep.id, ep.company_id, ep.entity_profile_type_id, ep.name, ep.ref_key, ep.created_at, ep.last_active_at,
			epm.entity_profile_id, epm.total_deliveries, epm.completed_successfully, epm.validation_violations,
			epm.delivery_health_score, epm.prev_delivery_health_score, epm.health_trend_slope, epm.average_delivery_time_ms, epm.last_calculated_at
		FROM entity_profile ep
		LEFT JOIN entity_profile_metrics epm ON epm.entity_profile_id = ep.id
		WHERE ep.company_id = $1 AND ep.id = $2
	`
	row := r.pool.QueryRow(ctx, query, companyID, entityProfileID)

	var p models.EntityProfile
	var m models.EntityProfileMetrics
	var metricsID *string

	var totalDeliveries, completed, validations *int

	err := row.Scan(
		&p.ID, &p.CompanyID, &p.ProfileTypeID, &p.Name, &p.RefKey, &p.CreatedAt, &p.LastActiveAt,
		&metricsID, &totalDeliveries, &completed, &validations,
		&m.DeliveryHealthScore, &m.PrevDeliveryHealthScore, &m.HealthTrendSlope, &m.AverageDeliveryTimeMs, &m.LastCalculatedAt,
	)
	if err != nil {
		return nil, nil, err
	}

	if metricsID != nil {
		m.EntityProfileID = *metricsID
		if totalDeliveries != nil {
			m.TotalDeliveries = *totalDeliveries
		}
		if completed != nil {
			m.CompletedSuccessfully = *completed
		}
		if validations != nil {
			m.ValidationViolations = *validations
		}
		return &p, &m, nil
	}
	return &p, nil, nil
}
