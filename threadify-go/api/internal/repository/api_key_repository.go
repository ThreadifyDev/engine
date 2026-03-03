package repository

import (
	"database/sql"
	"threadify-go/api/internal/models"
	serror "threadify-go/shared/errors"
)

type APIKeyRepository struct {
	db *sql.DB
}

func NewAPIKeyRepository(db *sql.DB) *APIKeyRepository {
	return &APIKeyRepository{db: db}
}

func (r *APIKeyRepository) Create(apiKey *models.APIKey) error {
	query := `
		INSERT INTO api_keys (id, key_hash, key_prefix, name, user_id, service_account_id, company_id, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err := r.db.Exec(query, apiKey.ID, apiKey.KeyHash, apiKey.KeyPrefix, apiKey.Name,
		apiKey.UserID, apiKey.ServiceAccountID, apiKey.CompanyID, apiKey.ExpiresAt)
	return err
}

func (r *APIKeyRepository) FindByCompanyID(companyID string) ([]*models.APIKey, error) {
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys
		WHERE company_id = $1 AND revoked_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		key := &models.APIKey{}
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

func (r *APIKeyRepository) FindByID(id string) (*models.APIKey, error) {
	key := &models.APIKey{}
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys WHERE id = $1
	`
	err := r.db.QueryRow(query, id).Scan(
		&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Name,
		&key.UserID, &key.ServiceAccountID, &key.CompanyID,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, serror.ErrApiKeyNotFound
	}
	return key, err
}

func (r *APIKeyRepository) FindByHash(keyHash string) (*models.APIKey, error) {
	key := &models.APIKey{}
	query := `
		SELECT id, key_hash, key_prefix, name, user_id, service_account_id, company_id,
			last_used_at, expires_at, created_at, revoked_at
		FROM api_keys WHERE key_hash = $1
	`
	err := r.db.QueryRow(query, keyHash).Scan(
		&key.ID, &key.KeyHash, &key.KeyPrefix, &key.Name,
		&key.UserID, &key.ServiceAccountID, &key.CompanyID,
		&key.LastUsedAt, &key.ExpiresAt, &key.CreatedAt, &key.RevokedAt,
	)
	if err == sql.ErrNoRows {
		return nil, serror.ErrApiKeyNotFound
	}
	return key, err
}

func (r *APIKeyRepository) Revoke(id string) error {
	query := `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *APIKeyRepository) UpdateLastUsed(id string) error {
	query := `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := r.db.Exec(query, id)
	return err
}

func (r *APIKeyRepository) DeleteExpired() error {
	query := `DELETE FROM api_keys WHERE expires_at < NOW()`
	_, err := r.db.Exec(query)
	return err
}
