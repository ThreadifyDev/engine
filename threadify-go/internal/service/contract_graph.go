package service

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/pkg/contractcontent"
	"github.com/threadify/engine/pkg/validator"
	"gopkg.in/yaml.v3"
)

// GraphBuilder builds contract graphs from YAML/JSON content.
type GraphBuilder struct{}

// NewGraphBuilder creates a new GraphBuilder instance.
func NewGraphBuilder() *GraphBuilder {
	return &GraphBuilder{}
}

// BuildGraph converts contract content (YAML/JSON) to a ContractGraph.
// Only builds the execution graph — metadata (contract_id, version) is stored in DB.
func (b *GraphBuilder) BuildGraph(content []byte) (*domain.ContractGraph, error) {
	contract, err := parseContract(content)
	if err != nil {
		return nil, err
	}

	for i := range contract.Steps {
		for _, dep := range contract.Steps[i].FreshDependsOn {
			if !slices.Contains(contract.Steps[i].DependsOn, dep) {
				contract.Steps[i].DependsOn = append(contract.Steps[i].DependsOn, dep)
			}
		}
		contract.Steps[i].DependsOn = contractcontent.Dependencies(contract.Steps[i].DependsOn, contract.Steps[i].ContentRules)
	}

	// Explicit transitions retain their existing immediate-order semantics. Step
	// dependencies are separate partial-order constraints: the dependency only
	// needs to have succeeded earlier in the thread, not immediately beforehand.
	strictTransitions := contract.Transitions
	dependencyEdges := b.buildTransitionsFromDependsOn(contract.Steps)
	topologyTransitions := b.mergeTransitions(strictTransitions, dependencyEdges)

	if err := b.validateParties(contract); err != nil {
		return nil, err
	}

	nodes := b.buildNodes(contract, topologyTransitions)

	entryPoints := contract.EntryPoints
	if len(entryPoints) == 0 {
		entryPoints = b.deriveEntryPoints(contract.Steps, topologyTransitions)
	}

	terminalSteps := contract.TerminalSteps
	if len(terminalSteps) == 0 {
		terminalSteps = b.deriveTerminalSteps(contract.Steps, topologyTransitions)
	}

	finalStep := ""
	if len(terminalSteps) > 0 {
		finalStep = terminalSteps[0]
	}

	return &domain.ContractGraph{
		SemanticsVersion: domain.CurrentContractGraphSemantics,
		Graph: domain.Graph{
			Nodes:         nodes,
			EntryPoints:   entryPoints,
			TerminalSteps: terminalSteps,
			FinalStep:     finalStep,
		},
		Transitions: strictTransitions,
		Validation:  contract.Validation,
		Parties:     contract.Parties,
	}, nil
}

// parseContract tries JSON first (PascalCase from Go serialization),
// then YAML (snake_case from user input).
func parseContract(content []byte) (*domain.ContractYAML, error) {
	normalized, err := validator.NormalizeSource(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract: %w", err)
	}
	content = []byte(normalized)
	var contract domain.ContractYAML
	if err := json.Unmarshal(content, &contract); err == nil {
		return &contract, nil
	}
	if err := yaml.Unmarshal(content, &contract); err != nil {
		return nil, fmt.Errorf("failed to parse contract: %w", err)
	}
	return &contract, nil
}

// stepOwner returns the resolved owner for a step (Owner takes precedence over Role).
func stepOwner(step domain.Step) string {
	if step.Owner != "" {
		return step.Owner
	}
	return step.Role
}

// validateParties checks that every step owner is declared in the contract's parties list.
func (b *GraphBuilder) validateParties(contract *domain.ContractYAML) error {
	if len(contract.Parties) == 0 {
		return nil
	}
	partySet := make(map[string]struct{}, len(contract.Parties))
	for _, p := range contract.Parties {
		partySet[p] = struct{}{}
	}
	for _, step := range contract.Steps {
		owner := stepOwner(step)
		if _, ok := partySet[owner]; !ok {
			return fmt.Errorf("step owner %q is not defined in contract parties: %v", owner, contract.Parties)
		}
	}
	return nil
}

// buildNodes constructs the full node map for steps and parallel groups.
func (b *GraphBuilder) buildNodes(contract *domain.ContractYAML, transitions []domain.Transition) map[string]domain.GraphNode {
	nodes := make(map[string]domain.GraphNode, len(contract.Steps)+len(contract.Groups))

	for _, step := range contract.Steps {
		nodes[step.ID] = domain.GraphNode{
			ID:              step.ID,
			Owner:           stepOwner(step),
			Role:            stepOwner(step),
			Type:            "step",
			Required:        true,
			FreshDependsOn:  append([]string{}, step.FreshDependsOn...),
			DependsOn:       append([]string{}, step.DependsOn...),
			Next:            b.findNextFromTransitions(step.ID, transitions),
			Timeout:         step.Timeout,
			BusinessContext: step.BusinessContext,
			ContentRules:    step.ContentRules,
			ParentGroup:     b.findParentGroup(step.ID, contract.Groups),
		}
	}

	for _, group := range contract.Groups {
		mode := "any_of"
		maxDuration := ""
		if group.Rules != nil {
			if group.Rules.AllMustSucceed {
				mode = "all_of"
			}
			maxDuration = group.Rules.MaxCombinedDuration
		}
		nodes[group.ID] = domain.GraphNode{
			ID:          group.ID,
			Type:        "parallel_group",
			Mode:        mode,
			Required:    true,
			DependsOn:   b.findGroupDependsOn(group, transitions),
			Steps:       group.Steps,
			Next:        b.findGroupNext(group, transitions),
			MaxDuration: maxDuration,
		}
	}

	return nodes
}

// findParentGroup returns the ID of the group that contains stepID, or "".
func (b *GraphBuilder) findParentGroup(stepID string, groups []domain.Group) string {
	for _, group := range groups {
		if slices.Contains(group.Steps, stepID) {
			return group.ID
		}
	}
	return ""
}

// findGroupNext returns the steps outside the group that the group transitions into.
func (b *GraphBuilder) findGroupNext(group domain.Group, transitions []domain.Transition) []string {
	next := make([]string, 0)
	for _, transition := range transitions {
		if slices.Contains(group.Steps, transition.From) {
			for _, toStep := range transition.To {
				if !slices.Contains(group.Steps, toStep) && !slices.Contains(next, toStep) {
					next = append(next, toStep)
				}
			}
		}
	}
	return next
}

// findGroupDependsOn returns the steps outside the group that the group depends on.
func (b *GraphBuilder) findGroupDependsOn(group domain.Group, transitions []domain.Transition) []string {
	dependsOn := make([]string, 0)
	for _, transition := range transitions {
		for _, toStep := range transition.To {
			if slices.Contains(group.Steps, toStep) && !slices.Contains(group.Steps, transition.From) && !slices.Contains(dependsOn, transition.From) {
				dependsOn = append(dependsOn, transition.From)
			}
		}
	}
	return dependsOn
}

// findNextFromTransitions returns steps reachable from stepID via outgoing transitions.
func (b *GraphBuilder) findNextFromTransitions(stepID string, transitions []domain.Transition) []string {
	next := make([]string, 0)
	for _, transition := range transitions {
		if transition.From == stepID {
			for _, toStep := range transition.To {
				if !slices.Contains(next, toStep) {
					next = append(next, toStep)
				}
			}
		}
	}
	return next
}

// findDependsOnFromTransitions returns steps that must complete before stepID.
func (b *GraphBuilder) findDependsOnFromTransitions(stepID string, transitions []domain.Transition) []string {
	dependsOn := make([]string, 0)
	for _, transition := range transitions {
		if slices.Contains(transition.To, stepID) && !slices.Contains(dependsOn, transition.From) {
			dependsOn = append(dependsOn, transition.From)
		}
	}
	return dependsOn
}

// deriveEntryPoints returns steps with no incoming transitions.
func (b *GraphBuilder) deriveEntryPoints(steps []domain.Step, transitions []domain.Transition) []string {
	entryPoints := make([]string, 0, len(steps))
	for _, step := range steps {
		if len(b.findDependsOnFromTransitions(step.ID, transitions)) == 0 {
			entryPoints = append(entryPoints, step.ID)
		}
	}
	return entryPoints
}

// deriveTerminalSteps returns steps with no outgoing transitions.
func (b *GraphBuilder) deriveTerminalSteps(steps []domain.Step, transitions []domain.Transition) []string {
	terminalSteps := make([]string, 0, len(steps))
	for _, step := range steps {
		if len(b.findNextFromTransitions(step.ID, transitions)) == 0 {
			terminalSteps = append(terminalSteps, step.ID)
		}
	}
	return terminalSteps
}

// buildTransitionsFromDependsOn creates topology edges for partial-order dependencies.
// These edges are intentionally not returned as ContractGraph.Transitions because doing
// so would turn them into strict immediate-order constraints at runtime.
func (b *GraphBuilder) buildTransitionsFromDependsOn(steps []domain.Step) []domain.Transition {
	// Preserve declaration order so the resulting graph is deterministic.
	seen := make(map[string]map[string]struct{})
	fromOrder := make([]string, 0)
	for _, step := range steps {
		if step.ID == "" {
			continue
		}
		for _, dep := range step.DependsOn {
			if dep == "" {
				continue
			}
			if seen[dep] == nil {
				seen[dep] = make(map[string]struct{})
				fromOrder = append(fromOrder, dep)
			}
			seen[dep][step.ID] = struct{}{}
		}
	}

	transitions := make([]domain.Transition, 0, len(seen))
	for _, from := range fromOrder {
		to := make([]string, 0, len(seen[from]))
		for _, step := range steps {
			if _, ok := seen[from][step.ID]; ok {
				to = append(to, step.ID)
			}
		}
		transitions = append(transitions, domain.Transition{From: from, To: to})
	}
	return transitions
}

// mergeTransitions returns a deduplicated topology view over strict transitions
// and dependency edges without changing the strict transition set itself.
func (b *GraphBuilder) mergeTransitions(sets ...[]domain.Transition) []domain.Transition {
	merged := make([]domain.Transition, 0)
	byFrom := make(map[string]int)

	for _, transitions := range sets {
		for _, transition := range transitions {
			index, ok := byFrom[transition.From]
			if !ok {
				index = len(merged)
				byFrom[transition.From] = index
				merged = append(merged, domain.Transition{From: transition.From})
			}
			for _, to := range transition.To {
				if !slices.Contains(merged[index].To, to) {
					merged[index].To = append(merged[index].To, to)
				}
			}
		}
	}

	return merged
}
