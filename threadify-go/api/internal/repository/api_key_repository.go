package repository

import (
	"context"
	"database/sql"
	"errors"
	"threadify-go/api/internal/domain"
	serror "threadify-go/shared/errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type apiKeyRepository struct {
	pool *pgxpool.Pool
}

func NewAPIKeyRepository(pool *pgxpool.Pool) domain.APIKeyRepository {
	return &apiKeyRepository{pool: pool}
}


func (r *apiKeyRepository) Create(ctx context.Context, apiKey *domain.APIKey) error {
	query := `
		INSERT INTO api_keys (id, key_hash, key_prefix, name, user_id, service_account_id, company_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := r.pool.Exec(ctx, query, apiKey.ID, apiKey.KeyHash, apiKey.KeyPrefix, apiKey.Name,
		apiKey.UserID, apiKey.ServiceAccountID, apiKey.CompanyID, apiKey.ExpiresAt)
	return err
}

func (r *apiKeyRepository) FindByCompanyID(ctx context.Context, companyID string) ([]*domain.APIKey, error) {
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys
		WHERE company_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*domain.APIKey
	for rows.Next() {
		key := &domain.APIKey{}
		err := rows.Scan(
			&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Name,
			&key.UserID, &key.ServiceAccountID, &key.CompanyID,
			&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt,
		)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (r *apiKeyRepository) FindByID(ctx context.Context, id string) (*domain.APIKey, error) {
	key := &domain.APIKey{}
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys WHERE id = $1
	`
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Name,
		&key.UserID, &key.ServiceAccountID, &key.CompanyID,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, serror.ErrApiKeyNotFound
		}
		return nil, err
	}
	return key, nil
}

func (r *apiKeyRepository) FindByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	key := &domain.APIKey{}
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys WHERE key_hash = $1
	`
	err := r.pool.QueryRow(ctx, query, keyHash).Scan(
		&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Name,
		&key.UserID, &key.ServiceAccountID, &key.CompanyID,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, serror.ErrApiKeyNotFound
		}
		return nil, err
	}
	return key, nil
}

func (r *apiKeyRepository) Revoke(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *apiKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *apiKeyRepository) DeleteExpired(ctx context.Context) error {
	query := `DELETE FROM api_keys WHERE expires_at < NOW()`
	_, err := r.pool.Exec(ctx, query)
	return err
}
