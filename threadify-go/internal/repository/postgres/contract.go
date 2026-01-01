package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

type ContractRepository struct {
	pool *pgxpool.Pool
}

func NewContractRepository(pool *pgxpool.Pool) *ContractRepository {
	return &ContractRepository{pool: pool}
}

func (r *ContractRepository) Create(ctx context.Context, contract *models.Contract) error {
	query := `
		INSERT INTO contracts (id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
	`

	return r.pool.QueryRow(ctx, query,
		contract.ID, contract.Name, contract.Description, contract.ContentHash,
		contract.LatestVersion, contract.OwnerID, contract.IsPublic, contract.IsDeleted,
		contract.CreatedAt, contract.UpdatedAt,
	).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)
}

func (r *ContractRepository) Update(ctx context.Context, contractID string, description, contentHash string, latestVersion int, updatedAt time.Time) (*models.Contract, error) {
	query := `
		UPDATE contracts 
		SET description = $1, content_hash = $2, latest_version = $3, updated_at = $4
		WHERE id = $5
		RETURNING id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
	`

	var contract models.Contract
	err := r.pool.QueryRow(ctx, query, description, contentHash, latestVersion, updatedAt, contractID).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	return &contract, err
}

func (r *ContractRepository) GetByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE id = $1 AND owner_id = $2 AND is_deleted = false
	`

	var contract models.Contract
	err := r.pool.QueryRow(ctx, query, contractID, ownerID).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	return &contract, err
}

func (r *ContractRepository) GetByID(ctx context.Context, contractID string) (*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE id = $1 AND is_deleted = false
	`

	var contract models.Contract
	err := r.pool.QueryRow(ctx, query, contractID).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	return &contract, err
}

func (r *ContractRepository) GetByName(ctx context.Context, contractName string) (*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE name = $1 AND is_deleted = false
	`

	var contract models.Contract
	err := r.pool.QueryRow(ctx, query, contractName).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	return &contract, err
}

func (r *ContractRepository) GetByNameSlim(ctx context.Context, contractName string) (*models.Contract, error) {
	query := `
		SELECT id, name, latest_version, owner_id
		FROM contracts WHERE name = $1 AND is_deleted = false
	`

	var contract models.Contract
	err := r.pool.QueryRow(ctx, query, contractName).Scan(
		&contract.ID, &contract.Name, &contract.LatestVersion, &contract.OwnerID,
	)

	return &contract, err
}

func (r *ContractRepository) SoftDelete(ctx context.Context, contractID string, updatedAt time.Time) error {
	query := `UPDATE contracts SET is_deleted = true, updated_at = $1 WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, updatedAt, contractID)
	return err
}

func (r *ContractRepository) GetAllByOwner(ctx context.Context, ownerID string) ([]*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts 
		WHERE owner_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := r.pool.Query(ctx, query, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []*models.Contract
	for rows.Next() {
		var contract models.Contract
		err := rows.Scan(
			&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
			&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
			&contract.CreatedAt, &contract.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		contracts = append(contracts, &contract)
	}

	return contracts, rows.Err()
}

// Contract Version operations

func (r *ContractRepository) CreateVersion(ctx context.Context, version *models.ContractVersion) error {
	query := `
		INSERT INTO contract_versions (id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at
	`

	return r.pool.QueryRow(ctx, query,
		version.ID, version.Version, version.Content, version.YAMLContent, version.ContentHash,
		version.ContractID, version.CreatedBy, version.Graph, version.IsDeleted,
		version.CreatedAt, version.UpdatedAt,
	).Scan(
		&version.ID, &version.Version, &version.Content, &version.YAMLContent, &version.ContentHash,
		&version.ContractID, &version.CreatedBy, &version.Graph, &version.IsDeleted,
		&version.CreatedAt, &version.UpdatedAt,
	)
}

func (r *ContractRepository) GetVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	query := `
		SELECT id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at
		FROM contract_versions WHERE contract_id = $1 AND version = $2 AND is_deleted = false
	`

	var v models.ContractVersion
	err := r.pool.QueryRow(ctx, query, contractID, version).Scan(
		&v.ID, &v.Version, &v.Content, &v.YAMLContent, &v.ContentHash, &v.ContractID,
		&v.CreatedBy, &v.Graph, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
	)

	return &v, err
}

func (r *ContractRepository) GetLatestVersion(ctx context.Context, contractID string) (*models.ContractVersion, error) {
	query := `
		SELECT id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at
		FROM contract_versions WHERE contract_id = $1 AND is_deleted = false
		ORDER BY version DESC LIMIT 1
	`

	var v models.ContractVersion
	err := r.pool.QueryRow(ctx, query, contractID).Scan(
		&v.ID, &v.Version, &v.Content, &v.YAMLContent, &v.ContentHash, &v.ContractID,
		&v.CreatedBy, &v.Graph, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
	)

	return &v, err
}

func (r *ContractRepository) GetAllVersions(ctx context.Context, contractID string) ([]*models.ContractVersion, error) {
	query := `
		SELECT id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at
		FROM contract_versions 
		WHERE contract_id = $1 AND is_deleted = false
		ORDER BY version DESC
	`

	rows, err := r.pool.Query(ctx, query, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []*models.ContractVersion
	for rows.Next() {
		var v models.ContractVersion
		err := rows.Scan(
			&v.ID, &v.Version, &v.Content, &v.YAMLContent, &v.ContentHash, &v.ContractID,
			&v.CreatedBy, &v.Graph, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		versions = append(versions, &v)
	}

	return versions, rows.Err()
}

func (r *ContractRepository) SoftDeleteVersion(ctx context.Context, contractID string, version int, updatedAt time.Time) error {
	query := `UPDATE contract_versions SET is_deleted = true, updated_at = $1 WHERE contract_id = $2 AND version = $3`
	_, err := r.pool.Exec(ctx, query, updatedAt, contractID, version)
	return err
}
