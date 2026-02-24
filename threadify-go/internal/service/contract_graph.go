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

	// Build transitions from legacy depends_on format when transitions are not provided.
	transitions := contract.Transitions
	if len(transitions) == 0 {
		transitions = b.buildTransitionsFromDependsOn(contract.Steps)
	}

	// 2. Build nodes map
	nodes := make(map[string]models.GraphNode)

	// 3. Build step nodes using transitions
	for _, step := range contract.Steps {
		owner := step.Owner
		if owner == "" {
			owner = step.Role
		}

		// Build graph from transitions.
		next := b.findNextFromTransitions(step.ID, transitions)
		dependsOn := b.findDependsOnFromTransitions(step.ID, transitions)

		// Check if step belongs to a group
		parentGroup := b.findParentGroup(step.ID, contract.Groups)

		nodes[step.ID] = models.GraphNode{
			ID:              step.ID,
			Owner:           owner,
			Role:            owner,
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
		groupNext := b.findGroupNext(group, transitions)
		groupDependsOn := b.findGroupDependsOn(group, transitions)

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
			DependsOn:   groupDependsOn,
			Steps:       group.Steps,
			Next:        groupNext,
			MaxDuration: maxDuration,
		}
	}

	// 5. Build graph with entry points, terminal steps, and transitions
	fmt.Printf("[BUILD-GRAPH] Building graph with %d transitions\n", len(transitions))
	if len(transitions) > 0 {
		fmt.Printf("[BUILD-GRAPH] First transition: from=%s, to=%v, canRetry=%v, maxRetries=%d\n",
			transitions[0].From, transitions[0].To, transitions[0].CanRetry, transitions[0].MaxRetries)
	}

	// Validate that all step owners exist in the parties array (if parties are defined)
	if len(contract.Parties) > 0 {
		for _, step := range contract.Steps {
			owner := step.Owner
			if owner == "" {
				owner = step.Role
			}
			stepOwnerInParties := false
			for _, party := range contract.Parties {
				if party == owner {
					stepOwnerInParties = true
					break
				}
			}
			if !stepOwnerInParties {
				return nil, fmt.Errorf("step owner '%s' is not defined in contract parties: %v", owner, contract.Parties)
			}
		}
	}

	entryPoints := contract.EntryPoints
	if len(entryPoints) == 0 {
		entryPoints = b.deriveEntryPoints(contract.Steps, transitions)
	}

	terminalSteps := contract.TerminalSteps
	if len(terminalSteps) == 0 {
		terminalSteps = b.deriveTerminalSteps(contract.Steps, transitions)
	}

	finalStep := ""
	if len(terminalSteps) > 0 {
		finalStep = terminalSteps[0]
	}

	return &models.ContractGraph{
		Graph: models.Graph{
			Nodes:         nodes,
			EntryPoints:   entryPoints,
			TerminalSteps: terminalSteps,
			FinalStep:     finalStep,
		},
		Transitions: transitions,
		Validation:  contract.Validation,
		Parties:     contract.Parties,
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

// findGroupDependsOn finds which steps this group depends on (from transitions)
func (b *GraphBuilder) findGroupDependsOn(group models.Group, transitions []models.Transition) []string {
	dependsOn := []string{}
	for _, transition := range transitions {
		// If transition.To includes a step in the group and transition.From is outside the group,
		// then this group depends on transition.From.
		for _, toStep := range transition.To {
			if contains(group.Steps, toStep) && !contains(group.Steps, transition.From) && !contains(dependsOn, transition.From) {
				dependsOn = append(dependsOn, transition.From)
			}
		}
	}
	return dependsOn
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

// findDependsOnFromTransitions finds incoming dependencies for a step.
func (b *GraphBuilder) findDependsOnFromTransitions(stepID string, transitions []models.Transition) []string {
	dependsOn := []string{}
	for _, transition := range transitions {
		if contains(transition.To, stepID) && !contains(dependsOn, transition.From) {
			dependsOn = append(dependsOn, transition.From)
		}
	}
	return dependsOn
}

func (b *GraphBuilder) deriveEntryPoints(steps []models.Step, transitions []models.Transition) []string {
	entryPoints := make([]string, 0, len(steps))
	for _, step := range steps {
		if len(b.findDependsOnFromTransitions(step.ID, transitions)) == 0 {
			entryPoints = append(entryPoints, step.ID)
		}
	}
	return entryPoints
}

func (b *GraphBuilder) deriveTerminalSteps(steps []models.Step, transitions []models.Transition) []string {
	terminalSteps := make([]string, 0, len(steps))
	for _, step := range steps {
		if len(b.findNextFromTransitions(step.ID, transitions)) == 0 {
			terminalSteps = append(terminalSteps, step.ID)
		}
	}
	return terminalSteps
}

func (b *GraphBuilder) buildTransitionsFromDependsOn(steps []models.Step) []models.Transition {
	transitions := make([]models.Transition, 0)
	seen := make(map[string]map[string]struct{})

	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if dep == "" || step.ID == "" {
				continue
			}

			if seen[dep] == nil {
				seen[dep] = make(map[string]struct{})
			}
			if _, exists := seen[dep][step.ID]; exists {
				continue
			}

			seen[dep][step.ID] = struct{}{}
		}
	}

	for from, toSet := range seen {
		to := make([]string, 0, len(toSet))
		for stepID := range toSet {
			to = append(to, stepID)
		}

		transitions = append(transitions, models.Transition{
			From: from,
			To:   to,
		})
	}

	return transitions
}
