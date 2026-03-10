package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	shderrors "threadify-go/shared/errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/threadify/engine/internal/models"
)

// contractCols and versionCols are the canonical SELECT column lists,
// kept in sync with the Scan calls in scanContract and scanVersion.
const contractCols = `id, name, company_id, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at`
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
		&c.ID, &c.Name, &c.CompanyID, &c.Description, &c.ContentHash,
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

func contractErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return shderrors.ErrContractNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		if strings.Contains(pgErr.ConstraintName, "idx_contracts_name_company_active") ||
			strings.Contains(pgErr.ConstraintName, "unique_contract_name") ||
			strings.Contains(pgErr.Message, "idx_contracts_name_company_active") {
			return shderrors.ErrContractAlreadyExists
		}
	}
	return fmt.Errorf("contract repo: %w", err)
}

func versionErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return shderrors.ErrContractNotFound
	}
	return fmt.Errorf("version repo: %w", err)
}

func (r *ContractRepository) Create(ctx context.Context, contract *models.Contract) error {
	query := `INSERT INTO contracts (` + contractCols + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING ` + contractCols

	return contractErr(scanContract(r.pool.QueryRow(ctx, query,
		contract.ID, contract.Name, contract.CompanyID, contract.Description, contract.ContentHash,
		contract.LatestVersion, contract.OwnerID, contract.IsPublic, contract.IsDeleted,
		contract.CreatedAt, contract.UpdatedAt,
	), contract))
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

func (r *ContractRepository) Get(ctx context.Context, id string) (*models.Contract, error) {
	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE id=$1 AND is_deleted=false`, id,
	), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByNameAndCompany(ctx context.Context, name, companyID string) (*models.Contract, error) {
	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE name=$1 AND company_id=$2 AND is_deleted=false`,
		name, companyID,
	), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error) {
	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE id=$1 AND owner_id=$2 AND is_deleted=false`,
		contractID, ownerID,
	), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByID(ctx context.Context, contractID string) (*models.Contract, error) {
	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE id=$1 AND is_deleted=false`, contractID,
	), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) GetByName(ctx context.Context, name string) (*models.Contract, error) {
	var c models.Contract
	if err := scanContract(r.pool.QueryRow(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE name=$1 AND is_deleted=false`, name,
	), &c); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

// GetByNameSlim returns a partially-populated Contract (id, name, latest_version, owner_id only).
// Do not use where a full Contract is expected.
func (r *ContractRepository) GetByNameSlim(ctx context.Context, name string) (*models.Contract, error) {
	var c models.Contract
	if err := r.pool.QueryRow(ctx,
		`SELECT id, name, latest_version, owner_id FROM contracts WHERE name=$1 AND is_deleted=false`, name,
	).Scan(&c.ID, &c.Name, &c.LatestVersion, &c.OwnerID); err != nil {
		return nil, contractErr(err)
	}
	return &c, nil
}

func (r *ContractRepository) SoftDelete(ctx context.Context, contractID string, updatedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contracts SET is_deleted=true, updated_at=$1 WHERE id=$2`,
		updatedAt, contractID,
	)
	return contractErr(err)
}

func (r *ContractRepository) GetAllByOwner(ctx context.Context, ownerID string) ([]*models.Contract, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+contractCols+` FROM contracts WHERE owner_id=$1 AND is_deleted=false ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("query contracts by owner: %w", err)
	}
	defer rows.Close()

	var contracts []*models.Contract
	for rows.Next() {
		var c models.Contract
		if err := scanContract(rows, &c); err != nil {
			return nil, fmt.Errorf("scan contract: %w", err)
		}
		contracts = append(contracts, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contracts: %w", err)
	}
	return contracts, nil
}

func (r *ContractRepository) CountByCompany(ctx context.Context, companyID string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM contracts WHERE company_id=$1 AND is_deleted=false`, companyID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count contracts by company: %w", err)
	}
	return count, nil
}

func (r *ContractRepository) CreateVersion(ctx context.Context, v *models.ContractVersion) error {
	query := `INSERT INTO contract_versions (` + versionCols + `)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING ` + versionCols

	return versionErr(scanVersion(r.pool.QueryRow(ctx, query,
		v.ID, v.Version, v.Content, v.YAMLContent, v.ContentHash,
		v.ContractID, v.CreatedBy, v.Graph, v.IsDeleted,
		v.CreatedAt, v.UpdatedAt,
	), v))
}

func (r *ContractRepository) GetVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	var v models.ContractVersion
	if err := scanVersion(r.pool.QueryRow(ctx,
		`SELECT `+versionCols+` FROM contract_versions WHERE contract_id=$1 AND version=$2 AND is_deleted=false`,
		contractID, version,
	), &v); err != nil {
		return nil, versionErr(err)
	}
	return &v, nil
}

func (r *ContractRepository) GetLatestVersion(ctx context.Context, contractID string) (*models.ContractVersion, error) {
	var v models.ContractVersion
	if err := scanVersion(r.pool.QueryRow(ctx,
		`SELECT `+versionCols+` FROM contract_versions WHERE contract_id=$1 AND is_deleted=false ORDER BY version DESC LIMIT 1`,
		contractID,
	), &v); err != nil {
		return nil, versionErr(err)
	}
	return &v, nil
}

func (r *ContractRepository) GetAllVersions(ctx context.Context, contractID string) ([]*models.ContractVersion, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+versionCols+` FROM contract_versions WHERE contract_id=$1 AND is_deleted=false ORDER BY version DESC`,
		contractID,
	)
	if err != nil {
		return nil, fmt.Errorf("query contract versions: %w", err)
	}
	defer rows.Close()

	var versions []*models.ContractVersion
	for rows.Next() {
		var v models.ContractVersion
		if err := scanVersion(rows, &v); err != nil {
			return nil, fmt.Errorf("scan contract version: %w", err)
		}
		versions = append(versions, &v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return versions, nil
}

func (r *ContractRepository) SoftDeleteVersion(ctx context.Context, contractID string, version int, updatedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contract_versions SET is_deleted=true, updated_at=$1 WHERE contract_id=$2 AND version=$3`,
		updatedAt, contractID, version,
	)
	if err != nil {
		return fmt.Errorf("soft delete version: %w", err)
	}
	return nil
}
