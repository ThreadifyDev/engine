package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
)

// ContractValidationService implements the ContractValidator interface
type ContractValidationService struct {
	graphRepo    interfaces.ContractGraphRepository
	contractRepo *postgres.ContractRepository
	cacheManager interfaces.CacheManager
}

// NewContractValidationService creates a new contract validation service
// contractRepo can be nil for testing (will skip PostgreSQL fallback)
func NewContractValidationService(graphRepo interfaces.ContractGraphRepository, contractRepo *postgres.ContractRepository, cacheManager interfaces.CacheManager) interfaces.ContractValidator {
	return &ContractValidationService{
		graphRepo:    graphRepo,
		contractRepo: contractRepo,
		cacheManager: cacheManager,
	}
}

// ValidateStepInContract checks if a step name exists in the contract graph and validates context
func (v *ContractValidationService) ValidateStepInContract(contractID string, version int, stepName string, context map[string]string, ownerID string) error {
	graph, err := v.GetContractGraph(contractID, version, ownerID)
	if err != nil {
		return err
	}

	// Check if step exists in contract graph nodes
	stepNode, exists := graph.Graph.Nodes[stepName]
	if !exists {
		return fmt.Errorf("step '%s' not found in contract '%s' version %d", stepName, contractID, version)
	}

	return v.ValidateStepContext(stepNode, context)
}

// ValidateStepContext validates the business context for a step node
// This is an internal method that accepts an already-fetched step node to avoid duplicate graph lookups
func (v *ContractValidationService) ValidateStepContext(stepNode models.GraphNode, context map[string]string) error {
	// Validate business context if defined in contract
	if stepNode.BusinessContext == nil {
		return nil
	}

	var requiredFields []string

	// BusinessContext can be either *models.BusinessContext or map[string]interface{} (from JSON unmarshal)
	switch bc := stepNode.BusinessContext.(type) {
	case *models.BusinessContext:
		// Direct struct pointer (from graph builder)
		requiredFields = bc.Required
	case map[string]interface{}:
		// Map from JSON unmarshal (from cache/database)
		if reqFields, ok := bc["required"].([]interface{}); ok {
			for _, field := range reqFields {
				if fieldStr, ok := field.(string); ok {
					requiredFields = append(requiredFields, fieldStr)
				}
			}
		}
	default:
		// Unknown type, skip validation
		return nil
	}

	// Validate required fields are present
	for _, requiredField := range requiredFields {
		if _, exists := context[requiredField]; !exists {
			return fmt.Errorf("required context field '%s' is missing", requiredField)
		}
	}

	return nil
}

// GetContractGraph retrieves contract graph from cache first, then Valkey, then PostgreSQL as fallback
func (v *ContractValidationService) GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, error) {
	// Normalize version: if 0, we need to look up the latest version first
	targetVersion := version
	if version == 0 && v.contractRepo != nil {
		contract, err := v.contractRepo.GetByNameAndCompany(context.Background(), contractName, companyID)
		if err == nil {
			targetVersion = contract.LatestVersion
		}
		// If lookup fails, we'll try with version 0 and let it fail later
	}

	// Tier 1: Check memory cache first (use targetVersion for lookup)
	if graph, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return graph, nil
	}

	// Tier 2: Check Valkey cache (use targetVersion for lookup)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	graph, err := v.graphRepo.Get(ctx, contractName, targetVersion, companyID)
	if err == nil {
		// Cache the graph from Valkey in memory for future use
		v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, graph)
		return graph, nil
	}

	// Tier 3: Load from PostgreSQL if not in Valkey (only if contract repo is available)
	if v.contractRepo != nil {
		// Look up the contract by name and company to get its UUID and latest version
		dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dbCancel()
		contract, err := v.contractRepo.GetByNameAndCompany(dbCtx, contractName, companyID)
		if err != nil {
			// Error is already sanitized by repository layer
			return nil, err
		}

		// If version is 0, use the latest version from the contract
		if version == 0 {
			targetVersion = contract.LatestVersion
		}

		// Get the specific version
		versionCtx, versionCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer versionCancel()
		contractVersion, err := v.contractRepo.GetVersion(versionCtx, contract.ID, targetVersion)
		if err != nil {
			// Error is already sanitized by repository layer
			return nil, err
		}

		// Parse the graph from JSON in the contract version
		if len(contractVersion.Graph) == 0 {
			return nil, fmt.Errorf("no graph found in contract version %s v%d", contractName, version)
		}

		// Unmarshal the entire ContractGraph (including Transitions)
		var loadedGraph models.ContractGraph
		err = json.Unmarshal(contractVersion.Graph, &loadedGraph)
		if err != nil {
			return nil, fmt.Errorf("failed to parse contract graph: %w", err)
		}

		// Store in both Valkey and memory caches for future use
		// Use targetVersion for caching, not the input version (which might be 0)
		saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer saveCancel()
		if err := v.graphRepo.Save(saveCtx, contractName, targetVersion, companyID, &loadedGraph); err != nil {
			// Log error but don't fail - we still have the graph
			fmt.Printf("Warning: failed to cache contract graph in Valkey: %v\n", err)
		}

		v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, &loadedGraph)
		return &loadedGraph, nil
	}

	// No PostgreSQL repository available, return the original error from Valkey
	return nil, fmt.Errorf("contract graph not found in Valkey and no PostgreSQL repository available: %s v%d", contractName, version)
}

// LoadContractGraphIntoCache preloads a contract graph into the cache using three-tier strategy
// Returns the actual version that was loaded (resolves version 0 to latest)
func (v *ContractValidationService) LoadContractGraphIntoCache(contractName string, version int, companyID string) (int, error) {
	// Normalize version: if 0, we need to look up the latest version first
	targetVersion := version
	if version == 0 && v.contractRepo != nil {
		contract, err := v.contractRepo.GetByNameAndCompany(context.Background(), contractName, companyID)
		if err == nil {
			targetVersion = contract.LatestVersion
		}
		// If lookup fails, we'll try with version 0 and let it fail later
	}

	// Check if already cached - don't reload if exists
	if _, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return targetVersion, nil
	}

	// Use GetContractGraph which implements the three-tier caching strategy
	_, err := v.GetContractGraph(contractName, version, companyID)
	return targetVersion, err
}

// GetContractByNameAndCompany retrieves a contract by name and company ID
func (v *ContractValidationService) GetContractByNameAndCompany(contractName string, companyID string) (*models.Contract, error) {
	if v.contractRepo == nil {
		return nil, fmt.Errorf("contract repository not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
}
