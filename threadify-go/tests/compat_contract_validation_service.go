package tests

import (
	"context"
	"fmt"
	"strings"

	"github.com/threadify/engine/internal/models"
)

type legacyContractGraphRepository interface {
	Get(ctx context.Context, contractID string, version int) (*models.ContractGraph, error)
}

type legacyContractCache interface {
	GetContractGraph(contractID string, version int) (*models.ContractGraph, bool)
	SetContractGraph(contractID string, version int, graph *models.ContractGraph)
}

type ContractValidationService struct {
	graphRepo legacyContractGraphRepository
	cache     legacyContractCache
}

func NewContractValidationService(graphRepo legacyContractGraphRepository, _ interface{}, cache legacyContractCache) *ContractValidationService {
	return &ContractValidationService{
		graphRepo: graphRepo,
		cache:     cache,
	}
}

func (v *ContractValidationService) ValidateStepInContract(contractID string, version int, stepName string, context map[string]string) error {
	graph, err := v.GetContractGraph(contractID, version)
	if err != nil {
		return err
	}

	stepNode, exists := graph.Graph.Nodes[stepName]
	if !exists {
		return fmt.Errorf("step '%s' not found in contract '%s' version %d", stepName, contractID, version)
	}

	return v.validateStepContext(stepNode, context)
}

func (v *ContractValidationService) GetContractGraph(contractID string, version int) (*models.ContractGraph, error) {
	if graph, exists := v.cache.GetContractGraph(contractID, version); exists {
		return graph, nil
	}

	graph, err := v.graphRepo.Get(context.Background(), contractID, version)
	if err != nil {
		return nil, err
	}

	v.cache.SetContractGraph(contractID, version, graph)
	return graph, nil
}

func (v *ContractValidationService) LoadContractGraphIntoCache(contractID string, version int) error {
	if _, exists := v.cache.GetContractGraph(contractID, version); exists {
		return nil
	}

	_, err := v.GetContractGraph(contractID, version)
	return err
}

func (v *ContractValidationService) validateStepContext(stepNode models.GraphNode, context map[string]string) error {
	if stepNode.BusinessContext == nil {
		return nil
	}

	switch bc := stepNode.BusinessContext.(type) {
	case map[string]string:
		for field := range bc {
			if value, exists := context[field]; exists && strings.TrimSpace(value) == "" {
				return fmt.Errorf("context field '%s' must be a non-empty string", field)
			}
		}
	case map[string]interface{}:
		for field := range bc {
			if value, exists := context[field]; exists && strings.TrimSpace(value) == "" {
				return fmt.Errorf("context field '%s' must be a non-empty string", field)
			}
		}
	case *models.BusinessContext:
		for _, field := range bc.Required {
			value, exists := context[field]
			if !exists || strings.TrimSpace(value) == "" {
				return fmt.Errorf("required context field '%s' is missing", field)
			}
		}
	}

	return nil
}
