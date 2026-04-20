package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

type AuthRepository struct {
	db *pgxpool.Pool
}

func NewAuthRepository(db *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{db: db}
}

// ValidateAPIKey retrieves API key information with service account and role details
func (r *AuthRepository) ValidateAPIKey(ctx context.Context, keyHash string) (*models.AuthInfo, error) {
	query := `
		SELECT
			sa.id as owner_id,
			sa.company_id,
			COALESCE(ur.role_name, 'standard_service') as role,
			ak.is_active,
			ak.expires_at
		FROM api_keys ak
		JOIN service_accounts sa ON ak.service_account_id = sa.id
		LEFT JOIN user_roles ur ON ur.principal_id = sa.id AND ur.principal_type = 'service_account'
		WHERE ak.key_hash = $1
		AND ak.is_active = true
		AND sa.is_active = true
		LIMIT 1
	`

	var info models.AuthInfo
	var isActive bool

	err := r.db.QueryRow(ctx, query, keyHash).Scan(
		&info.OwnerID,
		&info.CompanyID,
		&info.Role,
		&isActive,
		&info.ExpiresAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	info.IsActive = isActive
	return &info, nil
}

// GetUserRoles retrieves roles for a principal (user or service account)
func (r *AuthRepository) GetUserRoles(ctx context.Context, principalID string, principalType string) ([]string, error) {
	query := `
		SELECT role_name
		FROM user_roles
		WHERE principal_id = $1 AND principal_type = $2
	`

	rows, err := r.db.Query(ctx, query, principalID, principalType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}
