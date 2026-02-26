package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
	customerrors "github.com/threadify/engine/internal/utils/errors"
)

// contractCols and versionCols are the canonical SELECT column lists,
// kept in sync with the Scan calls below.
const contractCols = `id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at`
const versionCols = `id, version, content, yaml_content, content_hash, contract_id, created_by, graph, is_deleted, created_at, updated_at`

type ContractRepository struct {
	pool *pgxpool.Pool
}

func NewContractRepository(pool *pgxpool.Pool) *ContractRepository {
	return &ContractRepository{pool: pool}
}

// scanContract scans a standard contract row.
func scanContract(row pgx.Row, c *models.Contract) error {
	return row.Scan(
		&c.ID, &c.Name, &c.Description, &c.ContentHash,
		&c.LatestVersion, &c.OwnerID, &c.IsPublic, &c.IsDeleted,
		&c.CreatedAt, &c.UpdatedAt,
	)
}

// scanVersion scans a standard contract version row.
func scanVersion(row pgx.Row, v *models.ContractVersion) error {
	return row.Scan(
		&v.ID, &v.Version, &v.Content, &v.YAMLContent, &v.ContentHash,
		&v.ContractID, &v.CreatedBy, &v.Graph, &v.IsDeleted,
		&v.CreatedAt, &v.UpdatedAt,
	)
}

// contractErr maps pgx errors to domain errors for contract lookups.
func contractErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return customerrors.NewNotFoundError(customerrors.MsgContractNotFound, err)
	}
	return customerrors.NewInternalError(customerrors.MsgInternalError, err)
}

// versionErr maps pgx errors to domain errors for version lookups.
func versionErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return customerrors.NewNotFoundError(customerrors.MsgContractVersionNotFound, err)
	}
	return customerrors.NewInternalError(customerrors.MsgInternalError, err)
}

func (r *ContractRepository) Create(ctx context.Context, contract *models.Contract) error {
	query := `INSERT INTO contracts (` + contractCols + `, company_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING ` + contractCols

	err := scanContract(r.pool.QueryRow(ctx, query,
		contract.ID, contract.Name, contract.Description, contract.ContentHash,
		contract.LatestVersion, contract.OwnerID, contract.IsPublic, contract.IsDeleted,
		contract.CreatedAt, contract.UpdatedAt, contract.CompanyID,
	), contract)
	if err != nil {
		return customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return nil
}

func (r *ContractRepository) Update(ctx context.Context, contractID, description, contentHash string, latestVersion int, updatedAt time.Time) (*models.Contract, error) {
	query := `UPDATE contracts
		SET description=$1, content_hash=$2, latest_version=$3, updated_at=$4
		WHERE id=$5 AND is_deleted=false
		RETURNING ` + contractCols

	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx, query, description, contentHash, latestVersion, updatedAt, contractID), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByNameAndCompany(ctx context.Context, name, companyID string) (*models.Contract, error) {
	query := `SELECT id, name, company_id, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE name=$1 AND company_id=$2 AND is_deleted=false`

	var c models.Contract
	err := r.pool.QueryRow(ctx, query, name, companyID).Scan(
		&c.ID, &c.Name, &c.CompanyID, &c.Description, &c.ContentHash,
		&c.LatestVersion, &c.OwnerID, &c.IsPublic, &c.IsDeleted,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error) {
	query := `SELECT ` + contractCols + ` FROM contracts WHERE id=$1 AND owner_id=$2 AND is_deleted=false`

	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx, query, contractID, ownerID), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByID(ctx context.Context, contractID string) (*models.Contract, error) {
	query := `SELECT ` + contractCols + ` FROM contracts WHERE id=$1 AND is_deleted=false`

	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx, query, contractID), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByName(ctx context.Context, name string) (*models.Contract, error) {
	query := `SELECT ` + contractCols + ` FROM contracts WHERE name=$1 AND is_deleted=false`

	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx, query, name), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByNameSlim(ctx context.Context, name string) (*models.Contract, error) {
	query := `SELECT id, name, latest_version, owner_id FROM contracts WHERE name=$1 AND is_deleted=false`

	var c models.Contract
	if err := r.pool.QueryRow(ctx, query, name).Scan(&c.ID, &c.Name, &c.LatestVersion, &c.OwnerID); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) SoftDelete(ctx context.Context, contractID string, updatedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contracts SET is_deleted=true, updated_at=$1 WHERE id=$2`,
		updatedAt, contractID,
	)
	if err != nil {
		return customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return nil
}

func (r *ContractRepository) GetAllByOwner(ctx context.Context, ownerID string) ([]*models.Contract, error) {
	query := `SELECT ` + contractCols + `
		FROM contracts WHERE owner_id=$1 AND is_deleted=false ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, query, ownerID)
	if err != nil {
		return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	defer rows.Close()

	var contracts []*models.Contract
	for rows.Next() {
		var c models.Contract
		if err := scanContract(rows, &c); err != nil {
			return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
		}
		contracts = append(contracts, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return contracts, nil
}

func (r *ContractRepository) CreateVersion(ctx context.Context, v *models.ContractVersion) error {
	query := `INSERT INTO contract_versions (` + versionCols + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING ` + versionCols

	if err := scanVersion(r.pool.QueryRow(ctx, query,
		v.ID, v.Version, v.Content, v.YAMLContent, v.ContentHash,
		v.ContractID, v.CreatedBy, v.Graph, v.IsDeleted,
		v.CreatedAt, v.UpdatedAt,
	), v); err != nil {
		return customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return nil
}

func (r *ContractRepository) GetVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	query := `SELECT ` + versionCols + `
		FROM contract_versions WHERE contract_id=$1 AND version=$2 AND is_deleted=false`

	var v models.ContractVersion
	if err := scanVersion(r.pool.QueryRow(ctx, query, contractID, version), &v); err != nil {
		return nil, versionErr(err)
	}
	return &v, nil
}

func (r *ContractRepository) GetLatestVersion(ctx context.Context, contractID string) (*models.ContractVersion, error) {
	query := `SELECT ` + versionCols + `
		FROM contract_versions WHERE contract_id=$1 AND is_deleted=false
		ORDER BY version DESC LIMIT 1`

	var v models.ContractVersion
	if err := scanVersion(r.pool.QueryRow(ctx, query, contractID), &v); err != nil {
		return nil, versionErr(err)
	}
	return &v, nil
}

func (r *ContractRepository) GetAllVersions(ctx context.Context, contractID string) ([]*models.ContractVersion, error) {
	query := `SELECT ` + versionCols + `
		FROM contract_versions WHERE contract_id=$1 AND is_deleted=false ORDER BY version DESC`

	rows, err := r.pool.Query(ctx, query, contractID)
	if err != nil {
		return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	defer rows.Close()

	var versions []*models.ContractVersion
	for rows.Next() {
		var v models.ContractVersion
		if err := scanVersion(rows, &v); err != nil {
			return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return versions, nil
}

func (r *ContractRepository) SoftDeleteVersion(ctx context.Context, contractID string, version int, updatedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contract_versions SET is_deleted=true, updated_at=$1 WHERE contract_id=$2 AND version=$3`,
		updatedAt, contractID, version,
	)
	if err != nil {
		return customerrors.NewInternalError(customerrors.MsgInternalError, err)
	}
	return nil
}
