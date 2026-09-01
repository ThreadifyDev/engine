package service

import (
	"fmt"
	"slices"

	"github.com/threadify/engine/internal/domain"
)

// EvaluateStepProposal evaluates a candidate step against facts already
// gathered for the thread. successfulSteps must be ordered oldest to newest.
func EvaluateStepProposal(
	threadID string,
	threadStatus domain.ThreadStatus,
	graph *domain.ContractGraph,
	stepName string,
	successfulSteps []string,
) (*domain.StepProposal, error) {
	if graph == nil {
		return nil, fmt.Errorf("contract graph is required")
	}

	node, ok := graph.Graph.Nodes[stepName]
	if !ok || node.Type == "parallel_group" {
		return nil, fmt.Errorf("step %q is not defined in the contract", stepName)
	}

	proposal := &domain.StepProposal{
		ThreadID:       threadID,
		StepName:       stepName,
		RequiredSteps:  append([]string{}, node.DependsOn...),
		SatisfiedSteps: []string{},
		MissingSteps:   []string{},
	}

	completed := make(map[string]bool, len(successfulSteps))
	for _, successfulStep := range successfulSteps {
		if successfulStep != "" {
			completed[successfulStep] = true
		}
	}
	for _, requiredStep := range proposal.RequiredSteps {
		if completed[requiredStep] {
			proposal.SatisfiedSteps = append(proposal.SatisfiedSteps, requiredStep)
		} else {
			proposal.MissingSteps = append(proposal.MissingSteps, requiredStep)
		}
	}
	if len(successfulSteps) > 0 {
		previousStep := successfulSteps[len(successfulSteps)-1]
		proposal.PreviousStep = &previousStep
	}

	if threadStatus != domain.ThreadStatusActive {
		proposal.Reason = fmt.Sprintf("thread is %s", threadStatus)
		return proposal, nil
	}
	if len(proposal.MissingSteps) > 0 {
		proposal.Reason = fmt.Sprintf("missing required prior steps: %v", proposal.MissingSteps)
		return proposal, nil
	}

	if proposal.PreviousStep != nil {
		// Explicit transitions remain immediate-order constraints. Dependency-only
		// contracts have no strict transitions and therefore permit unrelated work
		// between a prerequisite and the proposed step.
		if len(graph.Transitions) > 0 {
			previousStep := *proposal.PreviousStep
			allowedNext := make([]string, 0)
			for _, transition := range graph.Transitions {
				if transition.From == previousStep {
					allowedNext = append(allowedNext, transition.To...)
				}
			}
			if !slices.Contains(allowedNext, stepName) {
				proposal.Reason = fmt.Sprintf("strict transition from %q to %q is not allowed", previousStep, stepName)
				return proposal, nil
			}
		}
	}

	proposal.Allowed = true
	proposal.Reason = "all contract constraints are satisfied"
	return proposal, nil
}
