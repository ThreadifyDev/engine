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

	requiredFields := v.extractRequiredFields(stepNode.BusinessContext)

	// Validate required fields are present
	for _, requiredField := range requiredFields {
		if _, exists := context[requiredField]; !exists {
			return fmt.Errorf("required context field '%s' is missing", requiredField)
		}
	}

	return nil
}

// extractRequiredFields extracts required fields from BusinessContext regardless of type
func (v *ContractValidationService) extractRequiredFields(businessContext interface{}) []string {
	switch bc := businessContext.(type) {
	case *models.BusinessContext:
		// Direct struct pointer (from graph builder)
		return bc.Required
	case map[string]interface{}:
		// Map from JSON unmarshal (from cache/database)
		var fields []string
		if reqFields, ok := bc["required"].([]interface{}); ok {
			for _, field := range reqFields {
				if fieldStr, ok := field.(string); ok {
					fields = append(fields, fieldStr)
				}
			}
		}
		return fields
	default:
		// Unknown type, return empty
		return []string{}
	}
}

// GetContractGraph retrieves contract graph from cache first, then Valkey, then PostgreSQL as fallback
func (v *ContractValidationService) GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, error) {
	// Normalize version: if 0, resolve to latest version
	targetVersion := v.resolveVersion(contractName, version, companyID)

	// Tier 1: Check memory cache first (use targetVersion for lookup)
	if graph, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return graph, nil
	}

	// Tier 2 & 3: Check Valkey cache, then PostgreSQL (use single context for all operations)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
		contract, err := v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
		if err != nil {
			// Error is already sanitized by repository layer
			return nil, err
		}

		// If version is 0, use the latest version from the contract
		if version == 0 {
			targetVersion = contract.LatestVersion
		}

		// Get the specific version
		contractVersion, err := v.contractRepo.GetVersion(ctx, contract.ID, targetVersion)
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
		// Reuse existing context (still has time remaining from 15s timeout)
		if err := v.graphRepo.Save(ctx, contractName, targetVersion, companyID, &loadedGraph); err != nil {
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
	// Normalize version: if 0, resolve to latest version
	targetVersion := v.resolveVersion(contractName, version, companyID)

	// Check if already cached - don't reload if exists
	if _, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return targetVersion, nil
	}

	// Use GetContractGraph which implements the three-tier caching strategy
	_, err := v.GetContractGraph(contractName, version, companyID)
	return targetVersion, err
}

// resolveVersion resolves version 0 to the latest version, returns version as-is otherwise
func (v *ContractValidationService) resolveVersion(contractName string, version int, companyID string) int {
	if version != 0 {
		return version
	}

	if v.contractRepo == nil {
		return version // Return 0 if no repo available
	}

	contract, err := v.contractRepo.GetByNameAndCompany(context.Background(), contractName, companyID)
	if err != nil {
		return version // Return 0 if lookup fails
	}

	return contract.LatestVersion
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
