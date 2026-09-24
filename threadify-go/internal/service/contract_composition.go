package service

import (
	"context"
	"fmt"
	"math/big"
	"regexp"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/pkg/contractcontent"
	"github.com/threadify/engine/pkg/validator"
)

var contractDuration = regexp.MustCompile(`^(\d+(?:\.\d+)?)(ms|us|s|m|h|d)$`)

func sameContractDuration(a, b string) bool {
	if a == b {
		return true
	}
	parse := func(value string) (*big.Rat, bool) {
		parts := contractDuration.FindStringSubmatch(value)
		if parts == nil {
			return nil, false
		}
		amount, ok := new(big.Rat).SetString(parts[1])
		if !ok {
			return nil, false
		}
		units := map[string]*big.Rat{
			"us": big.NewRat(1, 1000000), "ms": big.NewRat(1, 1000),
			"s": big.NewRat(1, 1), "m": big.NewRat(60, 1),
			"h": big.NewRat(3600, 1), "d": big.NewRat(86400, 1),
		}
		return amount.Mul(amount, units[parts[2]]), true
	}
	left, leftOK := parse(a)
	right, rightOK := parse(b)
	return leftOK && rightOK && left.Cmp(right) == 0
}

// CompositionReviewer supplies an optional semantic review for preview only.
// Its findings are advisory; deterministic validation controls publication.
type CompositionReviewer interface {
	ReviewComposition(context.Context, string, *validator.Contract) (bool, error)
}

func (s *ContractService) SetCompositionReviewer(reviewer CompositionReviewer) {
	s.compositionReviewer = reviewer
}

// validateComposition checks whether an expanded contract has a feasible path
// for every step. It only rejects contradictions that follow from deterministic
// rules; arbitrary regex intersections and external facts remain runtime checks.
func validateComposition(contract *validator.Contract, graph *domain.ContractGraph) []validator.ValidationError {
	var errors []validator.ValidationError
	for _, step := range contract.Steps {
		for _, contradiction := range contractcontent.Contradictions(step.ContentRules) {
			errors = append(errors, validator.ValidationError{
				Field:   fmt.Sprintf("steps.%s.content_rules", step.ID),
				Message: contradiction.Error(),
			})
		}
	}
	if len(contract.Steps) == 0 {
		return append(errors, validator.ValidationError{Field: "steps", Message: "Combined contract has no steps"})
	}
	if len(graph.Graph.EntryPoints) == 0 {
		return append(errors, validator.ValidationError{Field: "entry_points", Message: "Combined contract has no entry point"})
	}
	if len(graph.Graph.TerminalSteps) == 0 {
		errors = append(errors, validator.ValidationError{Field: "terminal_steps", Message: "Combined contract has no terminal step"})
	}
	terminal := make(map[string]bool, len(graph.Graph.TerminalSteps))
	for _, id := range graph.Graph.TerminalSteps {
		terminal[id] = true
	}
	steps := make(map[string]validator.Step, len(contract.Steps))
	for _, step := range contract.Steps {
		steps[step.ID] = step
	}
	for _, step := range contract.Steps {
		for _, dep := range step.DependsOn {
			if terminal[dep] {
				errors = append(errors, validator.ValidationError{
					Field:   fmt.Sprintf("steps.%s.depends_on", step.ID),
					Message: fmt.Sprintf("Step %q depends on terminal step %q, which completes the thread", step.ID, dep),
				})
			}
		}
	}
	if len(contract.Transitions) == 0 {
		return append(errors, unconstrainedReachability(contract, graph, steps, terminal)...)
	}
	if len(contract.Steps) <= 18 {
		if reached, complete := exactReachability(contract, graph, terminal); complete {
			return append(errors, unreachableErrors(contract, reached)...)
		}
	}
	return append(errors, structuralReachability(contract, graph, terminal)...)
}

// With no strict transitions, any nonterminal step may follow another. A
// dependency-free entry point is therefore enough to begin a valid topological
// order, provided no step depends on a terminal step.
func unconstrainedReachability(contract *validator.Contract, graph *domain.ContractGraph, steps map[string]validator.Step, terminal map[string]bool) []validator.ValidationError {
	started, continuing := false, false
	usableEntry := map[string]bool{}
	for _, entry := range graph.Graph.EntryPoints {
		if len(steps[entry].DependsOn) == 0 {
			started = true
			usableEntry[entry] = true
			if !terminal[entry] {
				continuing = true
			}
		}
	}
	if !started {
		return []validator.ValidationError{{Field: "entry_points", Message: "Every entry point requires a step that cannot have succeeded before the thread starts"}}
	}
	if !continuing {
		for _, step := range contract.Steps {
			if !usableEntry[step.ID] {
				return []validator.ValidationError{{Field: "entry_points", Message: "All usable entry points complete the thread before other steps can run"}}
			}
		}
	}
	return nil
}

type compositionState struct {
	at   int
	seen uint64
}

// exactReachability explores successful step sequences while tracking which
// prerequisites have succeeded. Repetition is allowed. Freshness, content
// values, and retry limits are ignored, making this an over-approximation:
// rejecting an unreachable step is sound, while some conflicts may remain.
func exactReachability(contract *validator.Contract, graph *domain.ContractGraph, terminal map[string]bool) (map[string]bool, bool) {
	ids := make([]string, len(contract.Steps))
	index := make(map[string]int, len(ids))
	for i, step := range contract.Steps {
		ids[i] = step.ID
		index[step.ID] = i
	}
	deps := make([]uint64, len(ids))
	for i, step := range contract.Steps {
		for _, dep := range step.DependsOn {
			deps[i] |= uint64(1) << index[dep]
		}
	}
	adjacent := make([][]int, len(ids))
	for _, transition := range contract.Transitions {
		from := index[transition.From]
		for _, to := range transition.To {
			adjacent[from] = append(adjacent[from], index[to])
		}
	}
	queue := make([]compositionState, 0, len(ids))
	visited := make(map[compositionState]bool)
	for _, entry := range graph.Graph.EntryPoints {
		i := index[entry]
		if deps[i] == 0 {
			state := compositionState{at: i, seen: uint64(1) << i}
			if !visited[state] {
				visited[state] = true
				queue = append(queue, state)
			}
		}
	}
	reached := make(map[string]bool, len(ids))
	for head := 0; head < len(queue); head++ {
		if len(visited) > 100000 {
			return nil, false
		}
		state := queue[head]
		reached[ids[state.at]] = true
		if terminal[ids[state.at]] {
			continue
		}
		for _, next := range adjacent[state.at] {
			if deps[next]&state.seen != deps[next] {
				continue
			}
			nextState := compositionState{at: next, seen: state.seen | uint64(1)<<next}
			if !visited[nextState] {
				visited[nextState] = true
				queue = append(queue, nextState)
			}
		}
	}
	return reached, true
}

// structuralReachability is the bounded fallback for large or highly cyclic
// graphs. It ignores dependencies, so it only reports impossible topology.
func structuralReachability(contract *validator.Contract, graph *domain.ContractGraph, terminal map[string]bool) []validator.ValidationError {
	adjacent := map[string][]string{}
	for _, transition := range contract.Transitions {
		adjacent[transition.From] = append(adjacent[transition.From], transition.To...)
	}
	queue := append([]string(nil), graph.Graph.EntryPoints...)
	reached := map[string]bool{}
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		if reached[id] {
			continue
		}
		reached[id] = true
		if !terminal[id] {
			queue = append(queue, adjacent[id]...)
		}
	}
	return unreachableErrors(contract, reached)
}

func unreachableErrors(contract *validator.Contract, reached map[string]bool) []validator.ValidationError {
	var errors []validator.ValidationError
	for _, step := range contract.Steps {
		if reached[step.ID] {
			continue
		}
		message := fmt.Sprintf("Step %q cannot be reached from an entry point before the thread completes", step.ID)
		if len(step.DependsOn) > 0 {
			message += "; check its prerequisites and strict transitions"
		}
		errors = append(errors, validator.ValidationError{Field: fmt.Sprintf("steps.%s", step.ID), Message: message})
		if len(errors) == 20 {
			break
		}
	}
	return errors
}
