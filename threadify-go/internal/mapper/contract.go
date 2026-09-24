package mapper

import (
	"encoding/json"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/pkg/validator"
)

func ToContractDTO(d *domain.Contract) *dto.Contract {
	if d == nil {
		return nil
	}
	return &dto.Contract{
		ID:            d.ID,
		Name:          d.Name,
		CompanyID:     d.CompanyID,
		Description:   d.Description,
		ContentHash:   d.ContentHash,
		LatestVersion: d.LatestVersion,
		OwnerID:       d.OwnerID,
		IsPublic:      d.IsPublic,
		IsDeleted:     d.IsDeleted,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

func ToContractVersionDTO(d *domain.ContractVersion, contractName string) (*dto.ContractVersion, error) {
	if d == nil {
		return nil, nil
	}

	sourceFormat := "yaml"
	if validator.IsGherkin(d.YAMLContent) {
		sourceFormat = "gherkin"
	}
	var normalizedGraph json.RawMessage = d.Graph

	return &dto.ContractVersion{
		Source:             d.YAMLContent,
		SourceFormat:       sourceFormat,
		ID:                 d.ID,
		Version:            d.Version,
		Content:            d.Content,
		YAMLContent:        d.YAMLContent,
		ContentHash:        d.ContentHash,
		ContractID:         d.ContractID,
		ContractName:       contractName,
		CreatedBy:          d.CreatedBy,
		Graph:              normalizedGraph,
		ExpectedDurationMs: d.ExpectedDurationMs,
		IsDeleted:          d.IsDeleted,
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
	}, nil
}

func ToContractDTOs(domainContracts []*domain.Contract) []*dto.Contract {
	if domainContracts == nil {
		return nil
	}
	contracts := make([]*dto.Contract, len(domainContracts))
	for i, c := range domainContracts {
		contracts[i] = ToContractDTO(c)
	}
	return contracts
}

func ToContractVersionDTOs(domainVersions []*domain.ContractVersion, contractName string) ([]*dto.ContractVersion, error) {
	if domainVersions == nil {
		return nil, nil
	}
	versions := make([]*dto.ContractVersion, len(domainVersions))
	for i, v := range domainVersions {
		versionDTO, err := ToContractVersionDTO(v, contractName)
		if err != nil {
			return nil, err
		}
		versions[i] = versionDTO
	}
	return versions, nil
}

func ToContractGraphDTO(d *domain.ContractGraph) *dto.ContractGraphDTO {
	if d == nil {
		return nil
	}
	dtoGraph := &dto.ContractGraphDTO{
		SemanticsVersion: d.SemanticsVersion,
		Graph: dto.GraphDTO{
			Nodes:         make(map[string]dto.GraphNode, len(d.Graph.Nodes)),
			EntryPoints:   d.Graph.EntryPoints,
			TerminalSteps: d.Graph.TerminalSteps,
			FinalStep:     d.Graph.FinalStep,
		},
		Parties: d.Parties,
	}

	if d.Graph.Nodes != nil {
		for k, v := range d.Graph.Nodes {
			nodeDTO := dto.GraphNode{
				ID:             v.ID,
				Owner:          v.Owner,
				Role:           v.Role,
				Type:           v.Type,
				Mode:           v.Mode,
				Required:       v.Required,
				DependsOn:      v.DependsOn,
				FreshDependsOn: v.FreshDependsOn,
				Next:           v.Next,
				Steps:          v.Steps,
				Timeout:        v.Timeout,
				MaxDuration:    v.MaxDuration,
				ParentGroup:    v.ParentGroup,
				ContentRules:   v.ContentRules,
				SemanticRules:  v.SemanticRules,
			}

			if v.BusinessContext != nil {
				nodeDTO.BusinessContext = &dto.BusinessContext{
					Required: v.BusinessContext.Required,
					Optional: v.BusinessContext.Optional,
				}
			}

			dtoGraph.Graph.Nodes[k] = nodeDTO
		}
	}

	if d.Transitions != nil {
		dtoGraph.Transitions = make([]dto.Transition, len(d.Transitions))
		for i, t := range d.Transitions {
			dtoGraph.Transitions[i] = dto.Transition{
				From:       t.From,
				To:         t.To,
				Timeout:    t.Timeout,
				MaxRetries: t.MaxRetries,
			}
		}
	}

	if d.Validation != nil {
		dtoGraph.Validation = &dto.Validation{
			MaxDuration:               d.Validation.MaxDuration,
			AllowMultipleTerminals:    d.Validation.AllowMultipleTerminals,
			MultipleTerminalsSeverity: d.Validation.MultipleTerminalsSeverity,
		}
	}

	if d.NotificationConfig != nil {
		dtoGraph.NotificationConfig = &dto.NotificationConfig{
			DefaultScope: d.NotificationConfig.DefaultScope,
			RoleDefaults: d.NotificationConfig.RoleDefaults,
		}
	}

	return dtoGraph
}
func FromContractGraphDTO(d *dto.ContractGraphDTO) *domain.ContractGraph {
	if d == nil {
		return nil
	}

	graph := &domain.ContractGraph{
		SemanticsVersion: d.SemanticsVersion,
		Graph: domain.Graph{
			Nodes:         make(map[string]domain.GraphNode, len(d.Graph.Nodes)),
			EntryPoints:   d.Graph.EntryPoints,
			TerminalSteps: d.Graph.TerminalSteps,
			FinalStep:     d.Graph.FinalStep,
		},
		Parties: d.Parties,
	}

	for k, v := range d.Graph.Nodes {
		node := domain.GraphNode{
			ID:             v.ID,
			Owner:          v.Owner,
			Role:           v.Role,
			Type:           v.Type,
			Mode:           v.Mode,
			Required:       v.Required,
			DependsOn:      v.DependsOn,
			FreshDependsOn: v.FreshDependsOn,
			Next:           v.Next,
			Steps:          v.Steps,
			Timeout:        v.Timeout,
			MaxDuration:    v.MaxDuration,
			ParentGroup:    v.ParentGroup,
			ContentRules:   v.ContentRules,
			SemanticRules:  v.SemanticRules,
		}

		if v.BusinessContext != nil {
			node.BusinessContext = &domain.BusinessContext{
				Required: v.BusinessContext.Required,
				Optional: v.BusinessContext.Optional,
			}
		}

		graph.Graph.Nodes[k] = node
	}

	if d.Transitions != nil {
		graph.Transitions = make([]domain.Transition, len(d.Transitions))
		for i, t := range d.Transitions {
			graph.Transitions[i] = domain.Transition{
				From:       t.From,
				To:         t.To,
				Timeout:    t.Timeout,
				MaxRetries: t.MaxRetries,
			}
		}
	}

	if d.Validation != nil {
		graph.Validation = &domain.Validation{
			MaxDuration:               d.Validation.MaxDuration,
			AllowMultipleTerminals:    d.Validation.AllowMultipleTerminals,
			MultipleTerminalsSeverity: d.Validation.MultipleTerminalsSeverity,
		}
	}

	if d.NotificationConfig != nil {
		graph.NotificationConfig = &domain.NotificationConfig{
			DefaultScope: d.NotificationConfig.DefaultScope,
			RoleDefaults: d.NotificationConfig.RoleDefaults,
		}
	}

	return graph
}
