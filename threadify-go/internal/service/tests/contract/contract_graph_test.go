package service_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/validator"
)

func graphInput(t *testing.T, contract domain.ContractDefinition) []byte {
	t.Helper()
	content, err := json.Marshal(contract)
	require.NoError(t, err)
	return content
}

func TestGraphBuilder_BuildGraph_DerivesTransitionsEntryAndTerminal(t *testing.T) {
	b := service.NewGraphBuilder()
	content := graphInput(t, domain.ContractDefinition{ContractName: "test", Parties: []string{"merchant", "processor"}, Steps: []domain.Step{
		{ID: "s1", Owner: "merchant"}, {ID: "s2", Role: "processor", DependsOn: []string{"s1"}},
	}})

	graph, err := b.BuildGraph(content)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"s1"}, graph.Graph.EntryPoints)
	require.ElementsMatch(t, []string{"s2"}, graph.Graph.TerminalSteps)
	require.Equal(t, "s2", graph.Graph.FinalStep)
	require.Empty(t, graph.Transitions, "depends_on must not become a strict immediate transition")

	n1 := graph.Graph.Nodes["s1"]
	n2 := graph.Graph.Nodes["s2"]
	require.Empty(t, n1.DependsOn)
	require.ElementsMatch(t, []string{"s2"}, n1.Next)
	require.ElementsMatch(t, []string{"s1"}, n2.DependsOn)
	require.Empty(t, n2.Next)
	require.Equal(t, "merchant", n1.Owner)
	require.Equal(t, "processor", n2.Owner)
}

func TestGraphBuilder_BuildGraph_KeepsStrictTransitionsSeparateFromDependencies(t *testing.T) {
	b := service.NewGraphBuilder()

	content := graphInput(t, domain.ContractDefinition{ContractName: "test", Parties: []string{"agent"}, Steps: []domain.Step{
		{ID: "authenticated", Owner: "agent"}, {ID: "unrelated", Owner: "agent"},
		{ID: "charge", Owner: "agent", DependsOn: []string{"authenticated"}},
	}, Transitions: []domain.Transition{{From: "unrelated", To: []string{"charge"}}}})

	graph, err := b.BuildGraph(content)
	require.NoError(t, err)
	require.Equal(t, []domain.Transition{{From: "unrelated", To: []string{"charge"}}}, graph.Transitions)
	require.Equal(t, []string{"authenticated"}, graph.Graph.Nodes["charge"].DependsOn)
	require.ElementsMatch(t, []string{"charge"}, graph.Graph.Nodes["authenticated"].Next)
}

func TestGraphBuilder_BuildGraph_JSON_Succeeds(t *testing.T) {
	b := service.NewGraphBuilder()

	contract := domain.ContractDefinition{
		ContractName: "c",
		Parties:      []string{},
		Steps: []domain.Step{
			{ID: "s1"},
			{ID: "s2", DependsOn: []string{"s1"}},
		},
	}
	payload, err := json.Marshal(contract)
	require.NoError(t, err)

	graph, err := b.BuildGraph(payload)
	require.NoError(t, err)
	require.Contains(t, graph.Graph.Nodes, "s1")
	require.Equal(t, []string{"s1"}, graph.Graph.Nodes["s2"].DependsOn)
	require.Empty(t, graph.Transitions)
}

func TestGraphBuilder_PreservesStepDescriptionInStoredGraph(t *testing.T) {
	content := []byte(`Feature: checkout
Version: 1
Description: Checkout workflow
Rule: Authorize payment
  When step "payment_authorized" is submitted
  Then owner must be "merchant"
  And step description is "The customer's payment has been approved by the payment provider."
  And this step is an entry point
`)
	contract, result := validator.NewContractValidator().Validate(string(content))
	require.True(t, result.IsValid, "%+v", result.Errors)
	_, serializedContent, err := validator.NewContractValidator().SerializeContract(contract)
	require.NoError(t, err)
	graph, err := service.NewGraphBuilder().BuildGraph([]byte(serializedContent))
	require.NoError(t, err)
	require.Equal(t, "The customer's payment has been approved by the payment provider.", graph.Graph.Nodes["payment_authorized"].Description)

	stored, err := json.Marshal(mapper.ToContractGraphDTO(graph))
	require.NoError(t, err)
	var restored dto.ContractGraphDTO
	require.NoError(t, json.Unmarshal(stored, &restored))
	require.Equal(t, graph.Graph.Nodes["payment_authorized"].Description, mapper.FromContractGraphDTO(&restored).Graph.Nodes["payment_authorized"].Description)
}

func TestGraphBuilder_BuildGraph_InvalidInputErrors(t *testing.T) {
	b := service.NewGraphBuilder()

	_, err := b.BuildGraph([]byte(":\n-"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse contract")
}

func TestGraphBuilderRejectsYamlContract(t *testing.T) {
	_, err := service.NewGraphBuilder().BuildGraph([]byte("contract_name: old_contract\nversion: 1\nsteps: []\n"))
	require.Error(t, err)
}

func TestGraphBuilder_BuildGraph_ValidatesParties(t *testing.T) {
	b := service.NewGraphBuilder()

	content := graphInput(t, domain.ContractDefinition{ContractName: "test", Parties: []string{"merchant"}, Steps: []domain.Step{
		{ID: "s1", Owner: "merchant"}, {ID: "s2", Owner: "processor"},
	}, Transitions: []domain.Transition{{From: "s1", To: []string{"s2"}}}})

	_, err := b.BuildGraph(content)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not defined in contract parties")
}

func TestGraphBuilder_BuildGraph_GroupsAndParentGroup(t *testing.T) {
	b := service.NewGraphBuilder()

	content := graphInput(t, domain.ContractDefinition{ContractName: "test", Parties: []string{"merchant"}, Steps: []domain.Step{
		{ID: "s1", Owner: "merchant"}, {ID: "s2", Owner: "merchant"}, {ID: "s3", Owner: "merchant"},
	}, Groups: []domain.Group{{ID: "g1", Steps: []string{"s2", "s3"}, Rules: &domain.GroupRules{AllMustSucceed: true, MaxCombinedDuration: "1h"}}},
		Transitions: []domain.Transition{{From: "s1", To: []string{"s2"}}, {From: "s2", To: []string{"s3"}}, {From: "s3", To: []string{"s1"}}}})

	graph, err := b.BuildGraph(content)
	require.NoError(t, err)

	g1, ok := graph.Graph.Nodes["g1"]
	require.True(t, ok)
	require.Equal(t, "parallel_group", g1.Type)
	require.Equal(t, "all_of", g1.Mode)
	require.Equal(t, "1h", g1.MaxDuration)

	// Parent group should be set for steps in the group.
	require.Equal(t, "g1", graph.Graph.Nodes["s2"].ParentGroup)
	require.Equal(t, "g1", graph.Graph.Nodes["s3"].ParentGroup)
	require.Empty(t, graph.Graph.Nodes["s1"].ParentGroup)
}

// NOTE: dependency topology helpers are intentionally unexported; their behaviour is
// covered through BuildGraph tests.
