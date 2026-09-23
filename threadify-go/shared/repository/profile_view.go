package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
)

type ProfileViewRepository struct{ pool *pgxpool.Pool }

func NewProfileViewRepository(pool *pgxpool.Pool) *ProfileViewRepository {
	return &ProfileViewRepository{pool: pool}
}

const profileViewColumns = `id, view_definition, view_requests, view_revision, view_updated_at, COALESCE(view_updated_by, '')`

func scanProfileView(row pgx.Row) (*domain.SavedProfileView, error) {
	var out domain.SavedProfileView
	var definition, requests []byte
	if err := row.Scan(&out.ProfileTypeID, &definition, &requests, &out.Revision, &out.UpdatedAt, &out.UpdatedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, serror.ErrEntityProfileTypeNotFound
		}
		return nil, err
	}
	out.Requests = []domain.ProfileViewRequest{}
	if len(definition) > 0 {
		if err := json.Unmarshal(definition, &out.Definition); err != nil {
			return nil, err
		}
		if err := out.Definition.Validate(); err != nil {
			return nil, fmt.Errorf("invalid saved profile view: %w", err)
		}
		if err := json.Unmarshal(requests, &out.Requests); err != nil {
			return nil, err
		}
	}
	return &out, nil
}
func (r *ProfileViewRepository) Get(ctx context.Context, companyID, typeID string) (*domain.SavedProfileView, error) {
	return scanProfileView(r.pool.QueryRow(ctx, `SELECT `+profileViewColumns+` FROM entity_profile_type WHERE company_id=$1 AND id=$2 AND archived_at IS NULL`, companyID, typeID))
}
func (r *ProfileViewRepository) Save(ctx context.Context, companyID, typeID, actorID string, expected int64, input domain.ProfileViewDefinition) (*domain.SavedProfileView, error) {
	definition, requests, err := domain.CompileProfileView(input)
	if err != nil {
		return nil, err
	}
	defJSON, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	reqJSON, err := json.Marshal(requests)
	if err != nil {
		return nil, err
	}
	// Resolve references under the same lock as the shared view update.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var lockedID string
	if err := tx.QueryRow(ctx, `SELECT id FROM entity_profile_type WHERE company_id=$1 AND id=$2 AND archived_at IS NULL FOR UPDATE`, companyID, typeID).Scan(&lockedID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, serror.ErrEntityProfileTypeNotFound
		}
		return nil, err
	}
	for _, block := range definition.Blocks {
		if block.MetricID == "" {
			continue
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM entity_profile_type_metrics WHERE entity_profile_type_id=$1 AND id=$2)`, typeID, block.MetricID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, domain.ErrProfileMetricBinding
		}
	}
	// The row's revision is checked atomically with its update. PostgreSQL rechecks
	// the predicate after concurrent writers release their lock.
	out, err := scanProfileView(tx.QueryRow(ctx, `UPDATE entity_profile_type SET
  view_definition=$4::jsonb, view_requests=$5::jsonb, view_revision=view_revision+1,
  view_updated_at=NOW(), view_updated_by=$6
  WHERE company_id=$1 AND id=$2 AND archived_at IS NULL AND view_revision=$3
  RETURNING `+profileViewColumns, companyID, typeID, expected, defJSON, reqJSON, actorID))
	if errors.Is(err, serror.ErrEntityProfileTypeNotFound) {
		return nil, domain.ErrProfileViewConflict
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return out, err
}
