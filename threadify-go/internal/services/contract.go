package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/pkg/validator"
)

type ContractService struct {
	db        *database.PostgresDB
	validator *validator.ContractValidator
}

type ContractResponse struct {
	Contract        *models.Contract        `json:"contract"`
	ContractVersion *models.ContractVersion `json:"contractVersion"`
}

type ContractWithOwnershipResponse struct {
	Contract        *models.Contract        `json:"contract"`
	ContractVersion *models.ContractVersion `json:"contractVersion"`
	IsOwner         bool                    `json:"isOwner"`
}

func NewContractService(db *database.PostgresDB) *ContractService {
	return &ContractService{
		db:        db,
		validator: validator.NewContractValidator(),
	}
}

func (s *ContractService) CreateContract(ctx context.Context, ownerID, createdBy, contractYAML string) (int, interface{}) {
	// Validate contract YAML
	contract, validationResult := s.validator.Validate(contractYAML)
	if !validationResult.IsValid {
		return 400, map[string]interface{}{
			"message": "Contract is not valid",
			"errors":  validationResult.Errors,
		}
	}

	// Force version to 1 for new contracts
	contract.Version = 1

	// Serialize contract
	fullJSON, contentOnlyJSON, err := s.validator.SerializeContract(contract)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize contract"}
	}

	// Calculate content hash (excluding metadata)
	contentHash := s.calculateHash(contentOnlyJSON)

	// Create contract in database
	contractID := uuid.New().String()
	now := time.Now()

	query := `
		INSERT INTO contracts (id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
	`

	var contractModel models.Contract
	err = s.db.Pool.QueryRow(ctx, query,
		contractID, contract.ContractName, contract.Description, contentHash, 1, ownerID, false, false, now, now,
	).Scan(
		&contractModel.ID, &contractModel.Name, &contractModel.Description, &contractModel.ContentHash,
		&contractModel.LatestVersion, &contractModel.OwnerID, &contractModel.IsPublic, &contractModel.IsDeleted,
		&contractModel.CreatedAt, &contractModel.UpdatedAt,
	)

	if err != nil {
		if err.Error() == "unique_contract_name" {
			return 400, map[string]string{"message": "Contract with this name already exists"}
		}
		return 500, map[string]string{"message": "Failed to create contract"}
	}

	// Create first version
	versionID := uuid.New().String()
	versionQuery := `
		INSERT INTO contract_versions (id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at
	`

	var versionModel models.ContractVersion
	err = s.db.Pool.QueryRow(ctx, versionQuery,
		versionID, 1, fullJSON, contentHash, contractID, createdBy, false, now, now,
	).Scan(
		&versionModel.ID, &versionModel.Version, &versionModel.Content, &versionModel.ContentHash,
		&versionModel.ContractID, &versionModel.CreatedBy, &versionModel.IsDeleted,
		&versionModel.CreatedAt, &versionModel.UpdatedAt,
	)

	if err != nil {
		return 500, map[string]string{"message": "Failed to create contract version"}
	}

	return 200, ContractResponse{
		Contract:        &contractModel,
		ContractVersion: &versionModel,
	}
}

func (s *ContractService) UpdateContract(ctx context.Context, contractID, ownerID, createdBy, contractYAML string) (int, interface{}) {
	// Find existing contract and verify ownership
	existingContract, err := s.getContractByIDAndOwner(ctx, contractID, ownerID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission to update it"}
	}

	// Validate contract YAML
	contract, validationResult := s.validator.Validate(contractYAML)
	if !validationResult.IsValid {
		return 400, map[string]interface{}{
			"message": "Contract is not valid",
			"errors":  validationResult.Errors,
		}
	}

	// Confirm version is greater than latest
	if contract.Version <= existingContract.LatestVersion {
		return 400, map[string]string{
			"message": fmt.Sprintf("Set Contract version: (%d) cannot be less or equal to the last created version of this contract (%d)",
				contract.Version, existingContract.LatestVersion),
		}
	}

	// Serialize contract
	fullJSON, contentOnlyJSON, err := s.validator.SerializeContract(contract)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize contract"}
	}

	// Calculate content hash
	contentHash := s.calculateHash(contentOnlyJSON)

	// Check if content has changed
	if existingContract.ContentHash != nil && *existingContract.ContentHash == contentHash {
		return 400, map[string]string{"message": "Contract content has not changed"}
	}

	// Update contract
	nextVersion := existingContract.LatestVersion + 1
	now := time.Now()

	updateQuery := `
		UPDATE contracts 
		SET description = $1, content_hash = $2, latest_version = $3, updated_at = $4
		WHERE id = $5
		RETURNING id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
	`

	var updatedContract models.Contract
	err = s.db.Pool.QueryRow(ctx, updateQuery,
		contract.Description, contentHash, nextVersion, now, contractID,
	).Scan(
		&updatedContract.ID, &updatedContract.Name, &updatedContract.Description, &updatedContract.ContentHash,
		&updatedContract.LatestVersion, &updatedContract.OwnerID, &updatedContract.IsPublic, &updatedContract.IsDeleted,
		&updatedContract.CreatedAt, &updatedContract.UpdatedAt,
	)

	if err != nil {
		return 500, map[string]string{"message": "Failed to update contract"}
	}

	// Create new version
	versionID := uuid.New().String()
	versionQuery := `
		INSERT INTO contract_versions (id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at
	`

	var newVersion models.ContractVersion
	err = s.db.Pool.QueryRow(ctx, versionQuery,
		versionID, nextVersion, fullJSON, contentHash, contractID, createdBy, false, now, now,
	).Scan(
		&newVersion.ID, &newVersion.Version, &newVersion.Content, &newVersion.ContentHash,
		&newVersion.ContractID, &newVersion.CreatedBy, &newVersion.IsDeleted,
		&newVersion.CreatedAt, &newVersion.UpdatedAt,
	)

	if err != nil {
		return 500, map[string]string{"message": "Failed to create new version"}
	}

	return 200, ContractResponse{
		Contract:        &updatedContract,
		ContractVersion: &newVersion,
	}
}

func (s *ContractService) GetContract(ctx context.Context, contractID, requesterID string, version *int) (int, interface{}) {
	// Find contract (only non-deleted)
	contract, err := s.getContractByIDNotDeleted(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	// Check if contract is accessible (public or owned by requester)
	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	// Get the requested version or latest version
	var contractVersion *models.ContractVersion
	if version != nil {
		contractVersion, err = s.getContractVersion(ctx, contractID, *version)
	} else {
		contractVersion, err = s.getLatestVersionNotDeleted(ctx, contractID)
	}

	if err != nil {
		return 404, map[string]string{"message": "Contract version not found"}
	}

	return 200, ContractWithOwnershipResponse{
		Contract:        contract,
		ContractVersion: contractVersion,
		IsOwner:         isOwner,
	}
}

func (s *ContractService) DeleteContract(ctx context.Context, contractID, ownerID string) (int, interface{}) {
	// Find contract and verify ownership
	contract, err := s.getContractByIDAndOwner(ctx, contractID, ownerID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission to delete it"}
	}

	if contract.IsDeleted {
		return 400, map[string]string{"message": "Contract is already deleted"}
	}

	// Soft delete
	query := `UPDATE contracts SET is_deleted = true, updated_at = $1 WHERE id = $2`
	_, err = s.db.Pool.Exec(ctx, query, time.Now(), contractID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to delete contract"}
	}

	return 200, map[string]string{"message": "Contract deleted successfully"}
}

// Helper functions
func (s *ContractService) getContractByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE id = $1 AND owner_id = $2 AND is_deleted = false
	`

	var contract models.Contract
	err := s.db.Pool.QueryRow(ctx, query, contractID, ownerID).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &contract, nil
}

func (s *ContractService) getContractByIDNotDeleted(ctx context.Context, contractID string) (*models.Contract, error) {
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts WHERE id = $1 AND is_deleted = false
	`

	var contract models.Contract
	err := s.db.Pool.QueryRow(ctx, query, contractID).Scan(
		&contract.ID, &contract.Name, &contract.Description, &contract.ContentHash,
		&contract.LatestVersion, &contract.OwnerID, &contract.IsPublic, &contract.IsDeleted,
		&contract.CreatedAt, &contract.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &contract, nil
}

func (s *ContractService) getContractVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	query := `
		SELECT id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at
		FROM contract_versions WHERE contract_id = $1 AND version = $2 AND is_deleted = false
	`

	var v models.ContractVersion
	err := s.db.Pool.QueryRow(ctx, query, contractID, version).Scan(
		&v.ID, &v.Version, &v.Content, &v.ContentHash, &v.ContractID,
		&v.CreatedBy, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *ContractService) getLatestVersionNotDeleted(ctx context.Context, contractID string) (*models.ContractVersion, error) {
	query := `
		SELECT id, version, content, content_hash, contract_id, created_by, is_deleted, created_at, updated_at
		FROM contract_versions WHERE contract_id = $1 AND is_deleted = false
		ORDER BY version DESC LIMIT 1
	`

	var v models.ContractVersion
	err := s.db.Pool.QueryRow(ctx, query, contractID).Scan(
		&v.ID, &v.Version, &v.Content, &v.ContentHash, &v.ContractID,
		&v.CreatedBy, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *ContractService) calculateHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func (s *ContractService) GetAllContracts(ctx context.Context, ownerID string) (int, interface{}) {
	// Get all contracts owned by the user (non-deleted)
	query := `
		SELECT id, name, description, content_hash, latest_version, owner_id, is_public, is_deleted, created_at, updated_at
		FROM contracts 
		WHERE owner_id = $1 AND is_deleted = false
		ORDER BY created_at DESC
	`

	rows, err := s.db.Pool.Query(ctx, query, ownerID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contracts"}
	}
	defer rows.Close()

	var contracts []models.Contract
	for rows.Next() {
		var c models.Contract
		err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &c.ContentHash,
			&c.LatestVersion, &c.OwnerID, &c.IsPublic, &c.IsDeleted,
			&c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return 500, map[string]string{"message": "Failed to parse contract data"}
		}
		contracts = append(contracts, c)
	}

	return 200, map[string]interface{}{
		"contracts": contracts,
		"total":     len(contracts),
	}
}

func (s *ContractService) GetAllContractVersions(ctx context.Context, contractID, requesterID string) (int, interface{}) {
	// Find contract (only non-deleted)
	contract, err := s.getContractByIDNotDeleted(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	// Check if contract is accessible (public or owned by requester)
	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	// Get all versions metadata (without full content)
	query := `
		SELECT id, version, content_hash, contract_id, created_by, is_deleted, created_at, updated_at
		FROM contract_versions 
		WHERE contract_id = $1 AND is_deleted = false
		ORDER BY version DESC
	`

	rows, err := s.db.Pool.Query(ctx, query, contractID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contract versions"}
	}
	defer rows.Close()

	type VersionMetadata struct {
		ID          string    `json:"id"`
		Version     int       `json:"version"`
		ContentHash string    `json:"contentHash"`
		ContractID  string    `json:"contractId"`
		CreatedBy   string    `json:"createdBy"`
		IsDeleted   bool      `json:"isDeleted"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}

	var versions []VersionMetadata
	for rows.Next() {
		var v VersionMetadata
		err := rows.Scan(
			&v.ID, &v.Version, &v.ContentHash, &v.ContractID,
			&v.CreatedBy, &v.IsDeleted, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			return 500, map[string]string{"message": "Failed to parse version data"}
		}
		versions = append(versions, v)
	}

	return 200, map[string]interface{}{
		"contractId":    contractID,
		"totalVersions": len(versions),
		"versions":      versions,
		"isOwner":       isOwner,
	}
}

func (s *ContractService) DeleteContractVersion(ctx context.Context, contractID string, version int, ownerID string) (int, interface{}) {
	// Find contract and verify ownership
	_, err := s.getContractByIDAndOwner(ctx, contractID, ownerID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission"}
	}

	// Find the specific version
	contractVersion, err := s.getContractVersion(ctx, contractID, version)
	if err != nil {
		return 404, map[string]string{"message": "Contract version not found"}
	}

	// Check if already deleted
	if contractVersion.IsDeleted {
		return 400, map[string]string{"message": "Contract version is already deleted"}
	}

	// Soft delete the version
	query := `UPDATE contract_versions SET is_deleted = true, updated_at = $1 WHERE id = $2`
	_, err = s.db.Pool.Exec(ctx, query, time.Now(), contractVersion.ID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to delete contract version"}
	}

	return 200, map[string]interface{}{
		"message": "Contract version deleted successfully",
		"version": map[string]interface{}{
			"id":         contractVersion.ID,
			"version":    contractVersion.Version,
			"contractId": contractVersion.ContractID,
		},
	}
}
