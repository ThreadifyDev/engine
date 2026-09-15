package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"github.com/threadify/engine/pkg/contractcontent"

	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

// SuccessfulContentReader resolves the latest validated success within a thread.
type SuccessfulContentReader interface {
	GetSuccessfulContent(context.Context, string, string) (map[string]string, error)
}

// ContractValidationService also resolves thread-bound successful content snapshots.
type ContractValidationService struct {
	references   SuccessfulContentReader
	graphRepo    domain.ContractGraphRepository
	contractRepo contractRepo
	cacheManager domain.CacheManager
	logger       *zap.Logger
}

type contractRepo interface {
	GetByNameAndCompany(ctx context.Context, name, companyID string) (*domain.Contract, error)
	GetVersion(ctx context.Context, contractID string, version int) (*domain.ContractVersion, error)
}

// NewContractValidationServiceFromParts creates a ContractValidationService from explicit parts.
// This is primarily intended for tests.
func NewContractValidationServiceFromParts(
	graphRepo domain.ContractGraphRepository,
	contractRepo contractRepo,
	cacheManager domain.CacheManager,
	logger *zap.Logger,
	readers ...SuccessfulContentReader,
) *ContractValidationService {
	var reader SuccessfulContentReader
	if len(readers) > 0 {
		reader = readers[0]
	}
	return &ContractValidationService{
		references:   reader,
		graphRepo:    graphRepo,
		contractRepo: contractRepo,
		cacheManager: cacheManager,
		logger:       logger,
	}
}

// NewContractValidationService creates a new contract validation service.
func NewContractValidationService(graphRepo domain.ContractGraphRepository, contractRepo contractRepo, cacheManager domain.CacheManager, logger *zap.Logger, readers ...SuccessfulContentReader) domain.ContractGraphValidator {
	return NewContractValidationServiceFromParts(graphRepo, contractRepo, cacheManager, logger, readers...)
}

// ValidateStepInContract checks if a step exists in the contract graph and validates its context.
func (v *ContractValidationService) ValidateStepInContract(ctx context.Context, contractID string, version int, stepName string, businessCtx map[string]string, ownerID string) error {
	graph, err := v.GetContractGraph(ctx, contractID, version, ownerID)
	if err != nil {
		return err
	}

	stepNode, exists := graph.Graph.Nodes[stepName]
	if !exists {
		return fmt.Errorf("step %q not found in contract %q version %d", stepName, contractID, version)
	}

	return v.ValidateStepContext(ctx, stepNode, businessCtx)
}

// ValidateStepContext validates the business context for a step node.
// Accepts an already-fetched node to avoid duplicate graph lookups.
func (v *ContractValidationService) ValidateStepContext(ctx context.Context, stepNode domain.GraphNode, businessCtx map[string]string, threadID ...string) error {
	// Resolve a step at most once per submission so its fields come from one snapshot.
	snapshots := map[string]map[string]string{}
	resolve := func(ref contractcontent.Reference) (string, error) {
		if len(threadID) != 1 || threadID[0] == "" || v.references == nil {
			return "", fmt.Errorf("step reference %s.%s requires thread context", ref.Step, ref.Field)
		}
		content, ok := snapshots[ref.Step]
		if !ok {
			var err error
			content, err = v.references.GetSuccessfulContent(ctx, threadID[0], ref.Step)
			if err != nil {
				return "", fmt.Errorf("cannot resolve step reference %s.%s: %w", ref.Step, ref.Field, err)
			}
			snapshots[ref.Step] = content
		}
		value, ok := content[ref.Field]
		if !ok {
			return "", fmt.Errorf("referenced field %s.%s is missing from the latest successful step", ref.Step, ref.Field)
		}
		return value, nil
	}
	if err := contractcontent.CheckWithReferences(stepNode.ContentRules, businessCtx, resolve); err != nil {
		return err
	}
	if stepNode.BusinessContext == nil {
		return nil
	}

	for _, field := range stepNode.BusinessContext.Required {
		if _, exists := businessCtx[field]; !exists {
			return fmt.Errorf("required context field %q is missing", field)
		}
	}

	return nil
}

// GetContractGraph retrieves a contract graph via three-tier lookup:
// memory cache → Valkey → PostgreSQL.
func (v *ContractValidationService) GetContractGraph(ctx context.Context, contractName string, version int, companyID string) (*domain.ContractGraph, error) {
	targetVersion, err := v.resolveVersion(ctx, contractName, version, companyID)
	if err != nil {
		return nil, err
	}

	// Tier 1: memory cache.
	if graph, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		if !requiresContractGraphMigration(graph) {
			return graph, nil
		}
		v.logger.Info("rebuilding cached contract graph with partial-order semantics",
			zap.String("contract_name", contractName),
			zap.Int("version", targetVersion))
	}

	// Tier 2: Valkey.
	graph, err := v.graphRepo.Get(ctx, contractName, targetVersion, companyID)
	if err == nil {
		if !requiresContractGraphMigration(graph) {
			v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, graph)
			return graph, nil
		}
		v.logger.Info("rebuilding Valkey contract graph with partial-order semantics",
			zap.String("contract_name", contractName),
			zap.Int("version", targetVersion))
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

	var graphDTO dto.ContractGraphDTO
	if err := json.Unmarshal(contractVersion.Graph, &graphDTO); err != nil {
		return nil, fmt.Errorf("failed to parse contract graph: %w", err)
	}
	loadedGraph := mapper.FromContractGraphDTO(&graphDTO)
	if requiresContractGraphMigration(loadedGraph) {
		source := contractVersion.YAMLContent
		if source == "" {
			source = contractVersion.Content
		}
		if source == "" {
			return nil, fmt.Errorf("contract %q v%d requires partial-order migration but has no source content", contractName, targetVersion)
		}
		loadedGraph, err = NewGraphBuilder().BuildGraph([]byte(source))
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild contract %q v%d with partial-order semantics: %w", contractName, targetVersion, err)
		}
	}

	if err := v.graphRepo.Save(ctx, contractName, targetVersion, companyID, loadedGraph); err != nil {
		v.logger.Warn("failed to cache contract graph in Valkey", zap.Error(err))
	}
	v.cacheManager.SetContractGraph(contractName, targetVersion, companyID, loadedGraph)

	return loadedGraph, nil
}

// requiresContractGraphMigration detects graphs persisted before dependency and
// transition semantics were separated. Graphs with no dependency-like edges are
// already behaviorally equivalent and can keep using the fast cache path.
func requiresContractGraphMigration(graph *domain.ContractGraph) bool {
	if graph == nil || graph.SemanticsVersion >= domain.CurrentContractGraphSemantics {
		return false
	}
	for _, node := range graph.Graph.Nodes {
		if len(node.DependsOn) > 0 {
			return true
		}
	}
	return false
}

// LoadContractGraphIntoCache preloads a contract graph using the three-tier strategy.
// Returns the resolved version that was loaded (version 0 resolves to latest).
func (v *ContractValidationService) LoadContractGraphIntoCache(ctx context.Context, contractName string, version int, companyID string) (int, error) {
	targetVersion, err := v.resolveVersion(ctx, contractName, version, companyID)
	if err != nil {
		return 0, err
	}

	if _, exists := v.cacheManager.GetContractGraph(contractName, targetVersion, companyID); exists {
		return targetVersion, nil
	}

	_, err = v.GetContractGraph(ctx, contractName, targetVersion, companyID)
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
func (v *ContractValidationService) GetContractByNameAndCompany(ctx context.Context, contractName string, companyID string) (*domain.Contract, error) {
	if v.contractRepo == nil {
		return nil, ErrContractRepoNotAvailable
	}
	return v.contractRepo.GetByNameAndCompany(ctx, contractName, companyID)
}
