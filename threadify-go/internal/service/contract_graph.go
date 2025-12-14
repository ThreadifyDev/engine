package service

import (
	"encoding/json"
	"fmt"

	"github.com/threadify/engine/internal/models"
	"gopkg.in/yaml.v3"
)

// GraphBuilder builds contract graphs from YAML/JSON content
type GraphBuilder struct{}

// NewGraphBuilder creates a new GraphBuilder instance
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// BuildGraph converts contract content (YAML/JSON) to ContractGraph
// Only builds the execution graph without metadata (contract_id, version stored in DB)
func (b *GraphBuilder) BuildGraph(content []byte) (*models.ContractGraph, error) {
	// 1. Parse YAML/JSON content into ContractYAML struct
	var contract models.ContractYAML

	// Try JSON first (for PascalCase from Go serialization), then YAML (for snake_case from user input)
	err := json.Unmarshal(content, &contract)
	if err != nil {
		// If JSON fails, try YAML (which also handles JSON with snake_case)
		err = yaml.Unmarshal(content, &contract)
		if err != nil {
			return nil, fmt.Errorf("failed to parse contract: %w", err)
		}
	}

	// 2. Build nodes map
	nodes := make(map[string]models.GraphNode)
	stepMap := make(map[string]models.Step)

	// Index all steps
	for _, step := range contract.Steps {
		stepMap[step.ID] = step
	}

	// 3. Build step nodes with dependencies
	for _, step := range contract.Steps {
		dependsOn := step.DependsOn
		if dependsOn == nil {
			dependsOn = []string{}
		}

		// Find which steps depend on THIS step (reverse lookup for "next")
		next := b.findNextSteps(step.ID, contract.Steps)

		// Check if step belongs to a group
		parentGroup := b.findParentGroup(step.ID, contract.Groups)

		nodes[step.ID] = models.GraphNode{
			ID:              step.ID,
			Owner:           step.Owner,
			Type:            "step",
			Required:        true, // Default, can be overridden
			DependsOn:       dependsOn,
			Next:            next,
			Timeout:         step.Timeout,
			BusinessContext: step.BusinessContext,
			ParentGroup:     parentGroup,
		}
	}

	// 4. Build parallel group nodes
	for _, group := range contract.Groups {
		groupDependsOn := b.findGroupDependencies(group, contract.Steps)
		groupNext := b.findGroupNext(group, contract.Steps)

		mode := "any_of"
		if group.Rules != nil && group.Rules.AllMustSucceed {
			mode = "all_of"
		}

		maxDuration := ""
		if group.Rules != nil {
			maxDuration = group.Rules.MaxCombinedDuration
		}

		nodes[group.ID] = models.GraphNode{
			ID:          group.ID,
			Type:        "parallel_group",
			Mode:        mode,
			Required:    true,
			Steps:       group.Steps,
			DependsOn:   groupDependsOn,
			Next:        groupNext,
			MaxDuration: maxDuration,
		}
	}

	// 5. Find final step (step with no "next")
	finalStep := b.findFinalStep(nodes)

	return &models.ContractGraph{
		Graph: models.Graph{
			Nodes:     nodes,
			FinalStep: finalStep,
		},
	}, nil
}

// findNextSteps finds which steps depend on the given step
func (b *GraphBuilder) findNextSteps(stepID string, steps []models.Step) []string {
	next := []string{}
	for _, otherStep := range steps {
		if otherStep.DependsOn != nil {
			for _, dep := range otherStep.DependsOn {
				if dep == stepID {
					next = append(next, otherStep.ID)
					break
				}
			}
		}
	}
	return next
}

// findParentGroup finds the parent group for a step
func (b *GraphBuilder) findParentGroup(stepID string, groups []models.Group) string {
	for _, group := range groups {
		if contains(group.Steps, stepID) {
			return group.ID
		}
	}
	return ""
}

// findGroupDependencies finds which steps a group depends on
func (b *GraphBuilder) findGroupDependencies(group models.Group, steps []models.Step) []string {
	dependsOn := []string{}
	for _, stepID := range group.Steps {
		for _, step := range steps {
			if step.ID == stepID && step.DependsOn != nil {
				// Take union of all dependencies (excluding steps within the group)
				for _, dep := range step.DependsOn {
					if !contains(dependsOn, dep) && !contains(group.Steps, dep) {
						dependsOn = append(dependsOn, dep)
					}
				}
			}
		}
	}
	return dependsOn
}

// findGroupNext finds which steps depend on this group
func (b *GraphBuilder) findGroupNext(group models.Group, steps []models.Step) []string {
	next := []string{}
	for _, step := range steps {
		if step.DependsOn != nil {
			for _, dep := range step.DependsOn {
				// If step depends on any step in the group, it depends on the group
				if contains(group.Steps, dep) && !contains(next, step.ID) {
					next = append(next, step.ID)
					break
				}
			}
		}
	}
	return next
}

// findFinalStep finds the final step (step with no next)
func (b *GraphBuilder) findFinalStep(nodes map[string]models.GraphNode) string {
	for id, node := range nodes {
		if node.Type == "step" && len(node.Next) == 0 {
			return id
		}
	}
	return ""
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
