package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

// ContractValidationService implements the ContractGraphValidator interface.
type ContractValidationService struct {
	graphRepo    interfaces.ContractGraphRepository
	contractRepo *postgres.ContractRepository
	cacheManager interfaces.CacheManager
	logger       *zap.Logger
}

// NewContractValidationService creates a new contract validation service.
func NewContractValidationService(graphRepo interfaces.ContractGraphRepository, contractRepo *postgres.ContractRepository, cacheManager interfaces.CacheManager, logger *zap.Logger) interfaces.ContractGraphValidator {
	return &ContractValidationService{
		graphRepo:    graphRepo,
		contractRepo: contractRepo,
		cacheManager: cacheManager,
		logger:       logger,
	}
}

// ValidateStepInContract checks if a step exists in the contract graph and validates its context.
func (v *ContractValidationService) ValidateStepInContract(contractID string, version int, stepName string, businessCtx map[string]string, ownerID string) error {
	graph, err := v.GetContractGraph(contractID, version, ownerID)
	if err != nil {
		return err
	}

	stepNode, exists := graph.Graph.Nodes[stepName]
	if !exists {
		return fmt.Errorf("step %q not found in contract %q version %d", stepName, contractID, version)
	}

	return v.ValidateStepContext(stepNode, businessCtx)
}

// ValidateStepContext validates the business context for a step node.
// Accepts an already-fetched node to avoid duplicate graph lookups.
func (v *ContractValidationService) ValidateStepContext(stepNode models.GraphNode, businessCtx map[string]string) error {
	if stepNode.BusinessContext == nil {
		return nil
	}

	for _, requiredField := range v.extractRequiredFields(stepNode.BusinessContext) {
		if _, exists := businessCtx[requiredField]; !exists {
			return fmt.Errorf("required context field %q is missing", requiredField)
		}
	}

	return nil
}

// extractRequiredFields extracts required fields from BusinessContext regardless of its runtime type.
func (v *ContractValidationService) extractRequiredFields(businessContext interface{}) []string {
	switch bc := businessContext.(type) {
	case *models.BusinessContext:
		// Direct struct pointer — from graph builder.
		return bc.Required
	case map[string]interface{}:
		// Map — from JSON unmarshal (cache/database).
		reqFields, ok := bc["required"].([]interface{})
		if !ok {
			return nil
		}
		fields := make([]string, 0, len(reqFields))
		for _, field := range reqFields {
			if s, ok := field.(string); ok {
				fields = append(fields, s)
			}
		}
		return fields
	default:
		return nil
	}
}

// GetContractGraph retrieves a contract graph via three-tier lookup:
// memory cache → Valkey → PostgreSQL.
func (v *ContractValidationService) GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	targetVersion, err := v.resolveVersion(ctx, contractName, version, companyID)
	if err != nil {
		return nil, err
	}

	// Tier 1: memory cache.
	if graph, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return graph, nil
	}

	// Tier 2: Valkey.
	graph, err := v.graphRepo.Get(ctx, contractName, targetVersion, companyID)
	if err == nil {
		v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, graph)
		return graph, nil
	}

	// Tier 3: PostgreSQL.
	if v.contractRepo == nil {
		return nil, fmt.Errorf("contract graph not found in Valkey and no PostgreSQL repository available: %s v%d", contractName, targetVersion)
	}

	contract, err := v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
	if err != nil {
		return nil, err
	}

	contractVersion, err := v.contractRepo.GetVersion(ctx, contract.ID, targetVersion)
	if err != nil {
		return nil, err
	}

	if len(contractVersion.Graph) == 0 {
		return nil, fmt.Errorf("no graph found in contract %q v%d", contractName, targetVersion)
	}

	var loadedGraph models.ContractGraph
	if err := json.Unmarshal(contractVersion.Graph, &loadedGraph); err != nil {
		return nil, fmt.Errorf("failed to parse contract graph: %w", err)
	}

	if err := v.graphRepo.Save(ctx, contractName, targetVersion, companyID, &loadedGraph); err != nil {
		v.logger.Warn("failed to cache contract graph in Valkey", zap.Error(err))
	}
	v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, &loadedGraph)

	return &loadedGraph, nil
}

// LoadContractGraphIntoCache preloads a contract graph using the three-tier strategy.
// Returns the resolved version that was loaded (version 0 resolves to latest).
func (v *ContractValidationService) LoadContractGraphIntoCache(contractName string, version int, companyID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	targetVersion, err := v.resolveVersion(ctx, contractName, version, companyID)
	if err != nil {
		return 0, err
	}

	if _, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return targetVersion, nil
	}

	_, err = v.GetContractGraph(contractName, version, companyID)
	return targetVersion, err
}

// resolveVersion resolves version 0 to the contract's latest version via DB lookup.
// Returns version unchanged if it is already non-zero or if no repo is available.
func (v *ContractValidationService) resolveVersion(ctx context.Context, contractName string, version int, companyID string) (int, error) {
	if version != 0 || v.contractRepo == nil {
		return version, nil
	}

	contract, err := v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve latest version for contract %q: %w", contractName, err)
	}

	return contract.LatestVersion, nil
}

// GetContractByNameAndCompany retrieves a contract by name and company ID.
func (v *ContractValidationService) GetContractByNameAndCompany(contractName string, companyID string) (*models.Contract, error) {
	if v.contractRepo == nil {
		return nil, ErrContractRepoNotAvailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
}
