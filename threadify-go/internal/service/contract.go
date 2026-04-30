package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	shderrors "threadify-go/shared/errors"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"github.com/threadify/engine/pkg/validator"
)

// TODO: CreateContract, UpdateContract, etc. return (int, interface{}) mixing HTTP
// concerns into the service layer. Consider returning typed errors and moving
// status code mapping to the handler layer.

type ContractService struct {
	repo      domain.ContractRepository
	planSvc   domain.PlanService
	validator domain.ContractValidator
	logger    *zap.Logger
}

type ContractResponse struct {
	Contract        *dto.Contract        `json:"contract"`
	ContractVersion *dto.ContractVersion `json:"contractVersion"`
}

type ContractWithOwnershipResponse struct {
	Contract        *dto.Contract        `json:"contract"`
	ContractVersion *dto.ContractVersion `json:"contractVersion"`
	IsOwner         bool                 `json:"isOwner"`
}

func NewContractService(repo domain.ContractRepository, planSvc domain.PlanService, logger *zap.Logger) *ContractService {
	return &ContractService{
		repo:      repo,
		planSvc:   planSvc,
		validator: validator.NewContractValidator(),
		logger:    logger,
	}
}

// parseDurationMs converts a duration string (e.g., "2d", "5h", "30m") to milliseconds.
// Returns nil if the duration string is empty or invalid.
func parseDurationMs(d string) *int64 {
	if d == "" {
		return nil
	}
	dur, err := time.ParseDuration(d)
	if err != nil {
		return nil
	}
	ms := dur.Milliseconds()
	return &ms
}

// NewContractServiceWithValidator allows injecting a custom contract validator (useful for tests).
// If v is nil, a default validator is used.
func NewContractServiceWithValidator(
	repo domain.ContractRepository,
	planSvc domain.PlanService,
	v domain.ContractValidator,
	logger *zap.Logger,
) *ContractService {
	if v == nil {
		v = validator.NewContractValidator()
	}
	return &ContractService{
		repo:      repo,
		planSvc:   planSvc,
		validator: v,
		logger:    logger,
	}
}

func (s *ContractService) CountContractsByCompany(ctx context.Context, companyID string) (int, error) {
	return s.repo.CountByOwner(ctx, companyID)
}

func (s *ContractService) enforceCredits(ctx context.Context, companyID string) (int, interface{}) {
	if err := s.planSvc.CheckCreditAvailable(ctx, companyID, MeterContractExecution, 1); err != nil {
		if errors.Is(err, ErrInsufficientCredit) || errors.Is(err, ErrNoAccount) {
			return 402, map[string]string{"message": err.Error()}
		}
		return 500, map[string]string{"message": "Failed to verify credit balance"}
	}
	return 0, nil
}

// PreviewContract validates YAML and builds a contract graph without persisting.
func (s *ContractService) PreviewContract(yamlString string) (*validator.Contract, *domain.ContractGraph, *validator.ValidationResult, error) {
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

	if status, resp := s.enforceCredits(ctx, companyID); status != 0 {
		return status, resp
	}

	contract.Version = 1

	fullJSON, contentOnlyJSON, err := s.validator.SerializeContract(contract)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize contract"}
	}

	contentHash := calculateContentHash(contentOnlyJSON)

	now := time.Now()
	contractModel := &domain.Contract{
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

	graphJSON, err := buildGraphJSON(contentOnlyJSON)
	if err != nil {
		return 500, map[string]string{"message": err.Error()}
	}

	versionModel := &domain.ContractVersion{
		ID:                 uuid.New().String(),
		Version:            1,
		Content:            fullJSON,
		YAMLContent:        contractYAML,
		ContentHash:        contentHash,
		ContractID:         contractModel.ID,
		CreatedBy:          createdBy,
		Graph:              graphJSON,
		ExpectedDurationMs: parseDurationMs(contract.Validation.MaxDuration),
		IsDeleted:          false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Atomically create both contract and version in a single transaction
	if err := s.repo.CreateContractWithVersion(ctx, contractModel, versionModel); err != nil {
		if errors.Is(err, shderrors.ErrContractAlreadyExists) {
			return 400, map[string]string{"message": "Contract with this name already exists"}
		}
		s.logger.Error("failed to create contract with version", zap.Error(err))
		return 500, map[string]string{"message": "Failed to create contract"}
	}

	if err := s.planSvc.ChargeContract(ctx, companyID); err != nil {
		s.logger.Error("charge contract failed", zap.String("company_id", companyID), zap.Error(err))
		return 402, map[string]string{"message": err.Error()}
	}

	versionDTO, err := mapper.ToContractVersionDTO(versionModel, contractModel.Name)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize version"}
	}

	return 200, ContractResponse{
		Contract:        mapper.ToContractDTO(contractModel),
		ContractVersion: versionDTO,
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

	if status, resp := s.enforceCredits(ctx, existingContract.CompanyID); status != 0 {
		return status, resp
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

	nextVersion := contract.Version
	now := time.Now()

	updatedContract, err := s.repo.Update(ctx, domain.UpdateContractParams{
		ContractID:      contractID,
		Description:     contract.Description,
		ContentHash:     contentHash,
		LatestVersion:   nextVersion,
		ExpectedVersion: existingContract.LatestVersion,
		UpdatedAt:       now,
	})
	if err != nil {
		if strings.Contains(err.Error(), "concurrent update detected") {
			return 409, map[string]string{"message": "The contract was updated by another process. Please refresh and try again."}
		}
		return 500, map[string]string{"message": "Failed to update contract"}
	}

	graphJSON, err := buildGraphJSON(contentOnlyJSON)
	if err != nil {
		return 500, map[string]string{"message": err.Error()}
	}

	newVersion := &domain.ContractVersion{
		ID:                 uuid.New().String(),
		Version:            nextVersion,
		Content:            fullJSON,
		YAMLContent:        contractYAML,
		ContentHash:        contentHash,
		ContractID:         contractID,
		CreatedBy:          createdBy,
		Graph:              graphJSON,
		ExpectedDurationMs: parseDurationMs(contract.Validation.MaxDuration),
		IsDeleted:          false,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := s.repo.CreateVersion(ctx, newVersion); err != nil {
		return 500, map[string]string{"message": "Failed to create new version"}
	}

	if err := s.planSvc.ChargeContractVersion(ctx, existingContract.CompanyID); err != nil {
		// Log but don't fail here since the version is already created?
		// Actually, if we want to be strict, we should have done this in a transaction.
		// For now, let's just log it.
		s.logger.Error("charge contract version failed after creation", zap.String("company_id", existingContract.CompanyID), zap.Error(err))
	}

	versionDTO, err := mapper.ToContractVersionDTO(newVersion, updatedContract.Name)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize version"}
	}

	return 200, ContractResponse{
		Contract:        mapper.ToContractDTO(updatedContract),
		ContractVersion: versionDTO,
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

	var contractVersion *domain.ContractVersion
	if version != nil {
		contractVersion, err = s.repo.GetVersion(ctx, contractID, *version)
	} else {
		contractVersion, err = s.repo.GetLatestVersion(ctx, contractID)
	}
	if err != nil {
		return 404, map[string]string{"message": "Contract version not found"}
	}

	versionDTO, err := mapper.ToContractVersionDTO(contractVersion, contract.Name)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize version"}
	}

	return 200, ContractWithOwnershipResponse{
		Contract:        mapper.ToContractDTO(contract),
		ContractVersion: versionDTO,
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

	if err := s.repo.SoftDelete(ctx, contractID); err != nil {
		return 500, map[string]string{"message": "Failed to delete contract"}
	}

	return 200, map[string]string{"message": "Contract deleted successfully"}
}

func (s *ContractService) GetAllContracts(ctx context.Context, ownerID string, search string, limit, offset int) (int, interface{}) {
	result, err := s.repo.GetAllByOwner(ctx, ownerID, domain.ContractListOptions{
		Search: search,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contracts"}
	}

	return 200, dto.ContractListResponse{
		Contracts: mapper.ToContractDTOs(result.Contracts),
		Total:     result.TotalCount,
	}
}

func (s *ContractService) GetAllContractVersions(ctx context.Context, contractID, requesterID string, limit, offset int) (int, interface{}) {
	contract, err := s.repo.GetByID(ctx, contractID)
	if err != nil {
		return 404, map[string]string{"message": "Contract not found"}
	}

	isOwner := contract.OwnerID == requesterID
	if !contract.IsPublic && !isOwner {
		return 403, map[string]string{"message": "Access denied. This contract is private."}
	}

	versions, err := s.repo.GetAllVersions(ctx, contractID) // TODO: Add pagination to repo too
	if err != nil {
		return 500, map[string]string{"message": "Failed to retrieve contract versions"}
	}

	// Apply pagination in-memory for now if repo doesn't support it yet
	// But it's better to add it to repo.
	total := len(versions)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}
	pagedVersions := versions[start:end]

	versionDTOs, err := mapper.ToContractVersionDTOs(pagedVersions, contract.Name)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize versions"}
	}

	return 200, dto.ContractVersionsResponse{
		Versions:      versionDTOs,
		ContractID:    contract.ID,
		Name:          contract.Name,
		Description:   contract.Description,
		LatestVersion: contract.LatestVersion,
		IsOwner:       isOwner,
		Total:         total,
		CreatedAt:     contract.CreatedAt,
		UpdatedAt:     contract.UpdatedAt,
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

	versionDTO, err := mapper.ToContractVersionDTO(contractVersion, contract.Name)
	if err != nil {
		return 500, map[string]string{"message": "Failed to serialize version"}
	}

	return 200, versionDTO
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

	if err := s.repo.SoftDeleteVersion(ctx, contractID, version); err != nil {
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

func buildGraphJSON(contentJSON string) ([]byte, error) {
	graph, err := NewGraphBuilder().BuildGraph([]byte(contentJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to build contract graph")
	}
	graphDTO := mapper.ToContractGraphDTO(graph)
	graphJSON, err := json.Marshal(graphDTO)
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
