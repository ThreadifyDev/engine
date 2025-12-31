package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/pkg/validator"
)

type ContractService struct {
	repo      *postgres.ContractRepository
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
		repo:      postgres.NewContractRepository(db.Pool),
		validator: validator.NewContractValidator(),
	}
}

// PreviewContract validates YAML and builds contract graph without persisting
func (s *ContractService) PreviewContract(yamlString string) (*validator.Contract, *models.ContractGraph, *validator.ValidationResult, error) {
	// Validate YAML
	contract, validationResult := s.validator.Validate(yamlString)
	if !validationResult.IsValid {
		return nil, nil, validationResult, nil
	}

	// Build contract graph
	graphBuilder := NewGraphBuilder()
	graph, err := graphBuilder.BuildGraph([]byte(yamlString))
	if err != nil {
		return nil, nil, nil, err
	}

	return contract, graph, validationResult, nil
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
	now := time.Now()
	contractModel := &models.Contract{
		ID:            uuid.New().String(),
		Name:          contract.ContractName,
		Description:   contract.Description,
		ContentHash:   &contentHash,
		LatestVersion: 1,
		OwnerID:       ownerID,
		IsPublic:      false,
		IsDeleted:     false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	err = s.repo.Create(ctx, contractModel)
	if err != nil {
		if err.Error() == "unique_contract_name" {
			return 400, map[string]string{"message": "Contract with this name already exists"}
		}
		return 500, map[string]string{"message": "Failed to create contract"}
	}

	// Generate contract graph
	// Use contentOnlyJSON since metadata is already in contract_versions table
	graphBuilder := NewGraphBuilder()
	contractGraph, err := graphBuilder.BuildGraph([]byte(contentOnlyJSON))
	if err != nil {
		return 500, map[string]string{"message": "Failed to build contract graph"}
	}

	// Serialize graph to JSON (serialize the entire ContractGraph including Transitions)
	graphJSON, err := json.Marshal(contractGraph)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize graph"}
	}

	// Create first version
	versionModel := &models.ContractVersion{
		ID:          uuid.New().String(),
		Version:     1,
		Content:     fullJSON,
		ContentHash: contentHash,
		ContractID:  contractModel.ID,
		CreatedBy:   createdBy,
		Graph:       graphJSON,
		IsDeleted:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.repo.CreateVersion(ctx, versionModel)
	if err != nil {
		return 500, map[string]string{"message": "Failed to create contract version"}
	}

	return 200, ContractResponse{
		Contract:        contractModel,
		ContractVersion: versionModel,
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

	updatedContract, err := s.repo.Update(ctx, contractID, contract.Description, contentHash, nextVersion, now)
	if err != nil {
		return 500, map[string]string{"message": "Failed to update contract"}
	}

	// Generate contract graph for new version
	// Use contentOnlyJSON since metadata is already in contract_versions table
	graphBuilder := NewGraphBuilder()
	contractGraph, err := graphBuilder.BuildGraph([]byte(contentOnlyJSON))
	if err != nil {
		return 500, map[string]string{"message": "Failed to build contract graph"}
	}

	// Serialize graph to JSON (serialize the entire ContractGraph including Transitions)
	graphJSON, err := json.Marshal(contractGraph)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize graph"}
	}

	// Create new version
	newVersion := &models.ContractVersion{
		ID:          uuid.New().String(),
		Version:     nextVersion,
		Content:     fullJSON,
		ContentHash: contentHash,
		ContractID:  contractID,
		CreatedBy:   createdBy,
		Graph:       graphJSON,
		IsDeleted:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.repo.CreateVersion(ctx, newVersion)
	if err != nil {
		return 500, map[string]string{"message": "Failed to create new version"}
	}

	return 200, ContractResponse{
		Contract:        updatedContract,
		ContractVersion: newVersion,
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
	err = s.repo.SoftDelete(ctx, contractID, time.Now())
	if err != nil {
		return 500, map[string]string{"message": "Failed to delete contract"}
	}

	return 200, map[string]string{"message": "Contract deleted successfully"}
}

// Helper functions
func (s *ContractService) getContractByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error) {
	return s.repo.GetByIDAndOwner(ctx, contractID, ownerID)
}

func (s *ContractService) getContractByIDNotDeleted(ctx context.Context, contractID string) (*models.Contract, error) {
	return s.repo.GetByID(ctx, contractID)
}

func (s *ContractService) getContractVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error) {
	return s.repo.GetVersion(ctx, contractID, version)
}

func (s *ContractService) getLatestVersionNotDeleted(ctx context.Context, contractID string) (*models.ContractVersion, error) {
	return s.repo.GetLatestVersion(ctx, contractID)
}

func (s *ContractService) calculateHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func (s *ContractService) GetAllContracts(ctx context.Context, ownerID string) (int, interface{}) {
	contracts, err := s.repo.GetAllByOwner(ctx, ownerID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contracts"}
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

	// Get all versions
	versions, err := s.repo.GetAllVersions(ctx, contractID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contract versions"}
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
	err = s.repo.SoftDeleteVersion(ctx, contractID, version, time.Now())
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
