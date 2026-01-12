package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ContractGraphRepository handles contract graph caching in Valkey (Redis)
type ContractGraphRepository struct {
	valkey interfaces.ValkeyClient
	ttl    int // TTL in seconds
}

// NewContractGraphRepository creates a new contract graph repository
func NewContractGraphRepository(valkey interfaces.ValkeyClient, ttl int) *ContractGraphRepository {
	return &ContractGraphRepository{
		valkey: valkey,
		ttl:    ttl,
	}
}

// Save stores a contract graph in Valkey cache
// contractName and version are passed separately since they're stored in the DB, not in the graph
func (r *ContractGraphRepository) Save(ctx context.Context, contractName string, version int, companyID string, graph *models.ContractGraph) error {
	key := r.getGraphKey(contractName, version, companyID)

	// Serialize graph to JSON
	data, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("failed to serialize contract graph: %w", err)
	}

	// Store in Valkey with TTL
	err = r.valkey.Set(ctx, key, string(data), time.Duration(r.ttl)*time.Second)
	if err != nil {
		return fmt.Errorf("failed to save contract graph: %w", err)
	}

	return nil
}

// Get retrieves a contract graph from Valkey cache
func (r *ContractGraphRepository) Get(ctx context.Context, contractName string, version int, companyID string) (*models.ContractGraph, error) {
	key := r.getGraphKey(contractName, version, companyID)

	// Get from Valkey
	data, err := r.valkey.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get contract graph: %w", err)
	}

	if data == "" {
		return nil, fmt.Errorf("contract graph not found: %s v%d", contractName, version)
	}

	// Deserialize graph
	var graph models.ContractGraph
	err = json.Unmarshal([]byte(data), &graph)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize contract graph: %w", err)
	}

	return &graph, nil
}

// Delete removes a contract graph from Valkey cache
func (r *ContractGraphRepository) Delete(ctx context.Context, contractName string, version int, companyID string) error {
	key := r.getGraphKey(contractName, version, companyID)

	err := r.valkey.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete contract graph: %w", err)
	}

	return nil
}

// Exists checks if a contract graph exists in Valkey cache
func (r *ContractGraphRepository) Exists(ctx context.Context, contractName string, version int, companyID string) (bool, error) {
	key := r.getGraphKey(contractName, version, companyID)

	exists, err := r.valkey.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check contract graph existence: %w", err)
	}

	return exists, nil
}

// ExtendTTL extends the TTL of a contract graph
func (r *ContractGraphRepository) ExtendTTL(ctx context.Context, contractName string, version int, companyID string) error {
	key := r.getGraphKey(contractName, version, companyID)

	err := r.valkey.Expire(ctx, key, time.Duration(r.ttl)*time.Second)
	if err != nil {
		return fmt.Errorf("failed to extend contract graph TTL: %w", err)
	}

	return nil
}

// DeleteByContract removes all versions of a contract graph from cache
func (r *ContractGraphRepository) DeleteByContract(ctx context.Context, contractName string, companyID string) error {
	pattern := r.getContractPattern(contractName, companyID)

	keys, err := r.valkey.Keys(ctx, pattern)
	if err != nil {
		return fmt.Errorf("failed to get contract graph keys: %w", err)
	}

	// Delete all matching keys
	for _, key := range keys {
		if err := r.valkey.Delete(ctx, key); err != nil {
			return fmt.Errorf("failed to delete contract graph key %s: %w", key, err)
		}
	}

	return nil
}

// getGraphKey generates the Redis key for a contract graph
func (r *ContractGraphRepository) getGraphKey(contractName string, version int, companyID string) string {
	return fmt.Sprintf("contract_graph:%s:%s:v%d", companyID, contractName, version)
}

// getContractPattern generates the Redis key pattern for all versions of a contract
func (r *ContractGraphRepository) getContractPattern(contractName string, companyID string) string {
	return fmt.Sprintf("contract_graph:%s:%s:v*", companyID, contractName)
}
