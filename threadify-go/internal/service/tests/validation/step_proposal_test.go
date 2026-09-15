package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
)

func TestEvaluateStepProposal_AllowsInterveningSuccessfulSteps(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"authenticated": {ID: "authenticated", Type: "step"},
		"unrelated":     {ID: "unrelated", Type: "step"},
		"charge":        {ID: "charge", Type: "step", DependsOn: []string{"authenticated"}},
	}}}

	proposal, err := service.EvaluateStepProposal(
		"thread-1",
		domain.ThreadStatusActive,
		graph,
		"charge",
		[]string{"authenticated", "unrelated"},
	)

	require.NoError(t, err)
	require.True(t, proposal.Allowed)
	require.Equal(t, []string{"authenticated"}, proposal.SatisfiedSteps)
	require.Empty(t, proposal.MissingSteps)
	require.NotNil(t, proposal.PreviousStep)
	require.Equal(t, "unrelated", *proposal.PreviousStep)
}

func TestEvaluateStepProposal_ReportsMissingDependencies(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"charge": {ID: "charge", Type: "step", DependsOn: []string{"authenticated", "profiled"}},
	}}}

	proposal, err := service.EvaluateStepProposal(
		"thread-1",
		domain.ThreadStatusActive,
		graph,
		"charge",
		[]string{"profiled", "unrelated"},
	)

	require.NoError(t, err)
	require.False(t, proposal.Allowed)
	require.Equal(t, []string{"profiled"}, proposal.SatisfiedSteps)
	require.Equal(t, []string{"authenticated"}, proposal.MissingSteps)
}

func TestEvaluateStepProposal_PreservesImmediateTransitions(t *testing.T) {
	graph := &domain.ContractGraph{
		Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
			"charge": {ID: "charge", Type: "step", DependsOn: []string{"authenticated"}},
		}},
		Transitions: []domain.Transition{{From: "authenticated", To: []string{"charge"}}},
	}

	proposal, err := service.EvaluateStepProposal(
		"thread-1",
		domain.ThreadStatusActive,
		graph,
		"charge",
		[]string{"authenticated", "unrelated"},
	)

	require.NoError(t, err)
	require.False(t, proposal.Allowed)
	require.Contains(t, proposal.Reason, "strict transition")
}

func TestEvaluateStepProposal_RejectsInactiveThreadAndUnknownStep(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"charge": {ID: "charge", Type: "step"},
	}}}

	proposal, err := service.EvaluateStepProposal("thread-1", domain.ThreadStatusCompleted, graph, "charge", nil)
	require.NoError(t, err)
	require.False(t, proposal.Allowed)
	require.Contains(t, proposal.Reason, "completed")

	_, err = service.EvaluateStepProposal("thread-1", domain.ThreadStatusActive, graph, "missing", nil)
	require.Error(t, err)
}
