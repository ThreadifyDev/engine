package service

import (
	"context"
	"encoding/json"
	"fmt"

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
func (v *ContractValidationService) ValidateStepInContract(contractID string, version int, stepName string, context map[string]string) error {
	graph, err := v.GetContractGraph(contractID, version)
	if err != nil {
		return err
	}

	// Check if step exists in contract graph nodes
	stepNode, exists := graph.Graph.Nodes[stepName]
	if !exists {
		return fmt.Errorf("step '%s' not found in contract '%s' version %d", stepName, contractID, version)
	}

	// Validate business context if defined in contract
	if stepNode.BusinessContext != nil {
		for key, expectedType := range stepNode.BusinessContext {
			if value, exists := context[key]; exists {
				// Basic type validation - could be extended
				switch expectedType {
				case "string":
					if value == "" {
						return fmt.Errorf("context field '%s' must be a non-empty string", key)
					}
				case "number":
					// Could add numeric validation here
					if value == "" {
						return fmt.Errorf("context field '%s' must be a number", key)
					}
				}
			}
		}
	}

	return nil
}

// GetContractGraph retrieves contract graph from cache first, then Valkey, then PostgreSQL as fallback
func (v *ContractValidationService) GetContractGraph(contractID string, version int) (*models.ContractGraph, error) {
	// Tier 1: Check memory cache first
	if graph, exists := v.cacheManager.GetContractGraph(contractID, version); exists {
		return graph, nil
	}

	// Tier 2: Check Valkey cache
	graph, err := v.graphRepo.Get(context.Background(), contractID, version)
	if err == nil {
		// Cache the graph from Valkey in memory for future use
		v.cacheManager.SetContractGraph(contractID, version, graph)
		return graph, nil
	}

	// Tier 3: Load from PostgreSQL if not in Valkey (only if contract repo is available)
	if v.contractRepo != nil {
		contractVersion, err := v.contractRepo.GetVersion(context.Background(), contractID, version)
		if err != nil {
			return nil, fmt.Errorf("failed to load contract from PostgreSQL: %w", err)
		}

		// Parse the graph from JSON in the contract version
		if len(contractVersion.Graph) == 0 {
			return nil, fmt.Errorf("no graph found in contract version %s v%d", contractID, version)
		}

		err = json.Unmarshal(contractVersion.Graph, &graph)
		if err != nil {
			return nil, fmt.Errorf("failed to parse contract graph: %w", err)
		}

		// Store in both Valkey and memory caches for future use
		if err := v.graphRepo.Save(context.Background(), contractID, version, graph); err != nil {
			// Log error but don't fail - we still have the graph
			fmt.Printf("Warning: failed to cache contract graph in Valkey: %v\n", err)
		}

		v.cacheManager.SetContractGraph(contractID, version, graph)
		return graph, nil
	}

	// No PostgreSQL repository available, return the original error from Valkey
	return nil, fmt.Errorf("contract graph not found in Valkey and no PostgreSQL repository available: %s v%d", contractID, version)
}

// LoadContractGraphIntoCache preloads a contract graph into the cache using three-tier strategy
func (v *ContractValidationService) LoadContractGraphIntoCache(contractID string, version int) error {
	// Check if already cached - don't reload if exists
	if _, exists := v.cacheManager.GetContractGraph(contractID, version); exists {
		return nil
	}

	// Use GetContractGraph which implements the three-tier caching strategy
	_, err := v.GetContractGraph(contractID, version)
	return err
}
