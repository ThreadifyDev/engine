package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
)

type decisionClassifierStub struct{ action string }

func (s decisionClassifierStub) MatchAction(context.Context, string, []string) (string, error) {
	return s.action, nil
}
func (s decisionClassifierStub) Recommend(context.Context, string, service.DecisionSnapshot) (string, string, error) {
	return "uncertain", "", nil
}
func TestResolveActionOnlyAcceptsContractActions(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"issue_refund": {ID: "issue_refund", Type: "step"},
		"cancel":       {ID: "cancel", Type: "step"},
	}}}
	action, source, err := service.ResolveAction(context.Background(), graph, "issue_refund", "", nil)
	require.NoError(t, err)
	require.Equal(t, "issue_refund", action)
	require.Equal(t, "exact", source)
	_, _, err = service.ResolveAction(context.Background(), graph, "", "I want to process a refund", nil)
	require.ErrorContains(t, err, "classifier is not configured")
	action, source, err = service.ResolveAction(context.Background(), graph, "", "I want to process a refund", decisionClassifierStub{action: "issue_refund"})
	require.NoError(t, err)
	require.Equal(t, "issue_refund", action)
	require.Equal(t, "classifier", source)
	_, _, err = service.ResolveAction(context.Background(), graph, "", "I want to process a refund", decisionClassifierStub{action: "invented"})
	require.ErrorContains(t, err, "did not identify")
}

func TestNextReturnsMultipleConditionalPaths(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{
		EntryPoints: []string{"verified"},
		Nodes: map[string]domain.GraphNode{
			"verified":                 {ID: "verified", Type: "step", Next: []string{"request_approval", "request_more_information"}},
			"request_approval":         {ID: "request_approval", Type: "step", DependsOn: []string{"verified"}, Next: []string{"issue_refund"}},
			"request_more_information": {ID: "request_more_information", Type: "step", DependsOn: []string{"verified"}},
			"issue_refund":             {ID: "issue_refund", Type: "step", DependsOn: []string{"request_approval"}},
		},
	}, Transitions: []domain.Transition{
		{From: "verified", To: []string{"request_approval", "request_more_information"}},
		{From: "request_approval", To: []string{"issue_refund"}},
	}}
	paths, err := service.EvaluateNextPaths("thread", domain.ThreadStatusActive, graph, []string{"verified"})
	require.NoError(t, err)
	require.Len(t, paths, 2)
	require.Equal(t, []string{"request_approval", "issue_refund"}, paths[0].Actions)
	require.Equal(t, []string{"request_more_information"}, paths[1].Actions)
	for _, path := range paths {
		require.NotEqual(t, "issue_refund", path.Actions[0])
	}
}

func TestCanRejectsNonEntryInitialAction(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{
		EntryPoints: []string{"verify"},
		Nodes:       map[string]domain.GraphNode{"verify": {Type: "step"}, "refund": {Type: "step"}},
	}}
	decision, err := service.EvaluateStepProposal("thread", domain.ThreadStatusActive, graph, "refund", nil)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, "denied", decision.Status)
}

func TestNextExcludesBranchesWhoseOtherPrerequisitesAreMissing(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{
		EntryPoints: []string{"start", "other"},
		Nodes: map[string]domain.GraphNode{
			"start":  {ID: "start", Type: "step", Next: []string{"finish"}},
			"other":  {ID: "other", Type: "step"},
			"finish": {ID: "finish", Type: "step", DependsOn: []string{"start", "other"}},
		},
	}}
	paths, err := service.EvaluateNextPaths("thread", domain.ThreadStatusActive, graph, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"other"}, paths[0].Actions)
	require.Equal(t, []string{"start"}, paths[1].Actions)
}

func TestNextMarksFreshPrerequisiteAsRequiringClaim(t *testing.T) {
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"start": {ID: "start", Type: "step"},
		"again": {ID: "again", Type: "step", DependsOn: []string{"start"}, FreshDependsOn: []string{"start"}},
	}}}
	decision, err := service.EvaluateStepProposal("thread", domain.ThreadStatusActive, graph, "again", []string{"start"})
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.Equal(t, "requires_claim", decision.Status)
	paths, err := service.EvaluateNextPaths("thread", domain.ThreadStatusActive, graph, []string{"start"})
	require.NoError(t, err)
	require.Equal(t, []string{"again"}, paths[0].Actions)
	require.Equal(t, "requires_claim", paths[0].Status)
}
