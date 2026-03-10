package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	shderrors "threadify-go/shared/errors"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"github.com/threadify/engine/pkg/validator"
)

// TODO: CreateContract, UpdateContract, etc. return (int, interface{}) mixing HTTP
// concerns into the service layer. Consider returning typed errors and moving
// status code mapping to the handler layer.

type ContractService struct {
	repo      *postgres.ContractRepository
	planSvc   *PlanService
	validator *validator.ContractValidator
	logger    *zap.Logger
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

func NewContractService(repo *postgres.ContractRepository, planSvc *PlanService, logger *zap.Logger) *ContractService {
	return &ContractService{
		repo:      repo,
		planSvc:   planSvc,
		validator: validator.NewContractValidator(),
		logger:    logger,
	}
}

func (s *ContractService) CountContractsByCompany(ctx context.Context, companyID string) (int, error) {
	return s.repo.CountByCompany(ctx, companyID)
}

// PreviewContract validates YAML and builds a contract graph without persisting.
func (s *ContractService) PreviewContract(yamlString string) (*validator.Contract, *models.ContractGraph, *validator.ValidationResult, error) {
	contract, validationResult := s.validator.Validate(yamlString)
	if !validationResult.IsValid {
		return nil, nil, validationResult, nil
	}

	graph, err := NewGraphBuilder().BuildGraph([]byte(yamlString))
	if err != nil {
		return nil, nil, nil, err
	}

	return contract, graph, validationResult, nil
}

func (s *ContractService) CreateContract(ctx context.Context, ownerID, companyID, createdBy, contractYAML string) (int, interface{}) {
	contract, validationResult := s.validator.Validate(contractYAML)
	if !validationResult.IsValid {
		return 400, map[string]interface{}{
			"message": "Contract is not valid",
			"errors":  validationResult.Errors,
		}
	}

	contract.Version = 1

	fullJSON, contentOnlyJSON, err := s.validator.SerializeContract(contract)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize contract"}
	}

	contentHash := calculateContentHash(contentOnlyJSON)

	now := time.Now()
	contractModel := &models.Contract{
		ID:            uuid.New().String(),
		Name:          contract.ContractName,
		Description:   contract.Description,
		ContentHash:   &contentHash,
		LatestVersion: 1,
		OwnerID:       ownerID,
		CompanyID:     companyID,
		IsPublic:      false,
		IsDeleted:     false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if s.planSvc != nil {
		if _, err := s.planSvc.ClaimContractSlot(ctx, companyID); err != nil {
			s.logger.Warn("contract slot claim denied", zap.String("company_id", companyID), zap.Error(err))
			return 403, map[string]string{"message": err.Error()}
		}
	}

	if err := s.repo.Create(ctx, contractModel); err != nil {
		if s.planSvc != nil {
			s.planSvc.ReleaseContractSlot(ctx, companyID) // Rollback
		}
		if errors.Is(err, shderrors.ErrContractAlreadyExists) {
			return 400, map[string]string{"message": "Contract with this name already exists"}
		}
		s.logger.Error("failed to create contract", zap.Error(err))
		return 500, map[string]string{"message": "Failed to create contract"}
	}

	graphJSON, err := buildGraphJSON(contentOnlyJSON)
	if err != nil {
		return 500, map[string]string{"message": err.Error()}
	}

	versionModel := &models.ContractVersion{
		ID:          uuid.New().String(),
		Version:     1,
		Content:     fullJSON,
		YAMLContent: contractYAML,
		ContentHash: contentHash,
		ContractID:  contractModel.ID,
		CreatedBy:   createdBy,
		Graph:       graphJSON,
		IsDeleted:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateVersion(ctx, versionModel); err != nil {
		return 500, map[string]string{"message": "Failed to create contract version"}
	}

	// Quota claimed above

	return 200, ContractResponse{
		Contract:        contractModel,
		ContractVersion: versionModel,
	}
}

func (s *ContractService) UpdateContract(ctx context.Context, contractID, ownerID, createdBy, contractYAML string) (int, interface{}) {
	existingContract, err := s.repo.GetByIDAndOwner(ctx, contractID, ownerID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission to update it"}
	}

	contract, validationResult := s.validator.Validate(contractYAML)
	if !validationResult.IsValid {
		return 400, map[string]interface{}{
			"message": "Contract is not valid",
			"errors":  validationResult.Errors,
		}
	}

	if contract.Version <= existingContract.LatestVersion {
		return 400, map[string]string{
			"message": fmt.Sprintf("contract version (%d) must be greater than the current latest version (%d)",
				contract.Version, existingContract.LatestVersion),
		}
	}

	fullJSON, contentOnlyJSON, err := s.validator.SerializeContract(contract)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize contract"}
	}

	contentHash := calculateContentHash(contentOnlyJSON)

	if existingContract.ContentHash != nil && *existingContract.ContentHash == contentHash {
		return 400, map[string]string{"message": "Contract content has not changed"}
	}

	// nextVersion is always existingContract.LatestVersion+1 regardless of contract.Version,
	// since the DB is the source of truth for sequential versioning.
	nextVersion := existingContract.LatestVersion + 1
	now := time.Now()

	updatedContract, err := s.repo.Update(ctx, contractID, contract.Description, contentHash, nextVersion, now)
	if err != nil {
		return 500, map[string]string{"message": "Failed to update contract"}
	}

	graphJSON, err := buildGraphJSON(contentOnlyJSON)
	if err != nil {
		return 500, map[string]string{"message": err.Error()}
	}

	newVersion := &models.ContractVersion{
		ID:          uuid.New().String(),
		Version:     nextVersion,
		Content:     fullJSON,
		YAMLContent: contractYAML,
		ContentHash: contentHash,
		ContractID:  contractID,
		CreatedBy:   createdBy,
		Graph:       graphJSON,
		IsDeleted:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := s.repo.CreateVersion(ctx, newVersion); err != nil {
		return 500, map[string]string{"message": "Failed to create new version"}
	}

	return 200, ContractResponse{
		Contract:        updatedContract,
		ContractVersion: newVersion,
	}
}

func (s *ContractService) GetContract(ctx context.Context, contractID, requesterID string, version *int) (int, interface{}) {
	contract, err := s.repo.GetByID(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	var contractVersion *models.ContractVersion
	if version != nil {
		contractVersion, err = s.repo.GetVersion(ctx, contractID, *version)
	} else {
		contractVersion, err = s.repo.GetLatestVersion(ctx, contractID)
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
	contract, err := s.repo.GetByIDAndOwner(ctx, contractID, ownerID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission to delete it"}
	}

	if contract.IsDeleted {
		return 400, map[string]string{"message": "Contract is already deleted"}
	}

	if err := s.repo.SoftDelete(ctx, contractID, time.Now()); err != nil {
		return 500, map[string]string{"message": "Failed to delete contract"}
	}

	// Decrement cached contract count
	if s.planSvc != nil {
		s.planSvc.ReleaseContractSlot(ctx, contract.CompanyID)
	}

	return 200, map[string]string{"message": "Contract deleted successfully"}
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
	contract, err := s.repo.GetByID(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	versions, err := s.repo.GetAllVersions(ctx, contractID)
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contract versions"}
	}

	return 200, map[string]interface{}{
		"contractId":    contractID,
		"name":          contract.Name,
		"description":   contract.Description,
		"latestVersion": contract.LatestVersion,
		"createdAt":     contract.CreatedAt,
		"updatedAt":     contract.UpdatedAt,
		"totalVersions": len(versions),
		"versions":      versions,
		"isOwner":       isOwner,
	}
}

func (s *ContractService) GetContractVersion(ctx context.Context, contractID string, version int, requesterID string) (int, interface{}) {
	contract, err := s.repo.GetByID(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	contractVersion, err := s.repo.GetVersion(ctx, contractID, version)
	if err != nil {
		return 404, map[string]string{"message": "Contract version not found"}
	}

	var graph models.ContractGraph
	if err := json.Unmarshal(contractVersion.Graph, &graph); err != nil {
		s.logger.Warn("failed to parse contract graph",
			zap.String("contract_id", contractID),
			zap.Int("version", version),
			zap.Error(err),
		)
		return 200, contractVersion
	}

	return 200, map[string]interface{}{
		"id":           contractVersion.ID,
		"version":      contractVersion.Version,
		"content":      contractVersion.Content,
		"yamlContent":  contractVersion.YAMLContent,
		"contentHash":  contractVersion.ContentHash,
		"contractId":   contractVersion.ContractID,
		"contractName": contract.Name,
		"createdBy":    contractVersion.CreatedBy,
		"graph":        graph,
		"isDeleted":    contractVersion.IsDeleted,
		"createdAt":    contractVersion.CreatedAt,
		"updatedAt":    contractVersion.UpdatedAt,
	}
}

func (s *ContractService) DeleteContractVersion(ctx context.Context, contractID string, version int, ownerID string) (int, interface{}) {
	if _, err := s.repo.GetByIDAndOwner(ctx, contractID, ownerID); err != nil {
		return 404, map[string]string{"message": "Contract not found or you don't have permission"}
	}

	contractVersion, err := s.repo.GetVersion(ctx, contractID, version)
	if err != nil {
		return 404, map[string]string{"message": "Contract version not found"}
	}

	if contractVersion.IsDeleted {
		return 400, map[string]string{"message": "Contract version is already deleted"}
	}

	if err := s.repo.SoftDeleteVersion(ctx, contractID, version, time.Now()); err != nil {
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

// buildGraphJSON builds a contract graph from content JSON and serializes it.
func buildGraphJSON(contentJSON string) ([]byte, error) {
	graph, err := NewGraphBuilder().BuildGraph([]byte(contentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to build contract graph")
	}
	graphJSON, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize graph")
	}
	return graphJSON, nil
}

// calculateContentHash returns the hex-encoded SHA-256 hash of content.
func calculateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
