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

	// 3. Build step nodes using transitions
	for _, step := range contract.Steps {
		// Build graph from transitions - only need Next (outgoing transitions)
		next := b.findNextFromTransitions(step.ID, contract.Transitions)

		// Check if step belongs to a group
		parentGroup := b.findParentGroup(step.ID, contract.Groups)

		nodes[step.ID] = models.GraphNode{
			ID:              step.ID,
			Owner:           step.Owner,
			Type:            "step",
			Required:        true, // Default, can be overridden
			Next:            next,
			Timeout:         step.Timeout,
			BusinessContext: step.BusinessContext,
			ParentGroup:     parentGroup,
		}
	}

	// 4. Build parallel group nodes
	for _, group := range contract.Groups {
		groupNext := b.findGroupNext(group, contract.Transitions)

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
			Next:        groupNext,
			MaxDuration: maxDuration,
		}
	}

	// 5. Build graph with entry points, terminal steps, and transitions
	fmt.Printf("[BUILD-GRAPH] Building graph with %d transitions\n", len(contract.Transitions))
	if len(contract.Transitions) > 0 {
		fmt.Printf("[BUILD-GRAPH] First transition: from=%s, to=%v, canRetry=%v, maxRetries=%d\n",
			contract.Transitions[0].From, contract.Transitions[0].To, contract.Transitions[0].CanRetry, contract.Transitions[0].MaxRetries)
	}

	return &models.ContractGraph{
		Graph: models.Graph{
			Nodes:         nodes,
			EntryPoints:   contract.EntryPoints,
			TerminalSteps: contract.TerminalSteps,
		},
		Transitions: contract.Transitions,
	}, nil
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

// findGroupNext finds which steps depend on this group (from transitions)
func (b *GraphBuilder) findGroupNext(group models.Group, transitions []models.Transition) []string {
	next := []string{}
	for _, transition := range transitions {
		// If transition.From is in the group, then all transition.To steps are next
		if contains(group.Steps, transition.From) {
			for _, toStep := range transition.To {
				if !contains(group.Steps, toStep) && !contains(next, toStep) {
					// This step is outside the group and depends on the group
					next = append(next, toStep)
				}
			}
		}
	}
	return next
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

// findNextFromTransitions finds steps that can be transitioned to from this step (outgoing transitions)
func (b *GraphBuilder) findNextFromTransitions(stepID string, transitions []models.Transition) []string {
	next := []string{}
	for _, transition := range transitions {
		// If this step is the "from", all "to" steps are next
		if transition.From == stepID {
			for _, toStep := range transition.To {
				if !contains(next, toStep) {
					next = append(next, toStep)
				}
			}
		}
	}
	return next
}
