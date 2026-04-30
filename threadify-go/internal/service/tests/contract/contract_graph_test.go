package service_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
)

func TestGraphBuilder_BuildGraph_YAML_DerivesTransitionsEntryAndTerminal(t *testing.T) {
	b := service.NewGraphBuilder()

	content := []byte(`
contract_name: test
parties: [merchant, processor]
steps:
  - id: s1
    owner: merchant
  - id: s2
    role: processor
    depends_on: [s1]
`)

	graph, err := b.BuildGraph(content)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"s1"}, graph.Graph.EntryPoints)
	require.ElementsMatch(t, []string{"s2"}, graph.Graph.TerminalSteps)
	require.Equal(t, "s2", graph.Graph.FinalStep)

	n1 := graph.Graph.Nodes["s1"]
	n2 := graph.Graph.Nodes["s2"]
	require.Empty(t, n1.DependsOn)
	require.ElementsMatch(t, []string{"s2"}, n1.Next)
	require.ElementsMatch(t, []string{"s1"}, n2.DependsOn)
	require.Empty(t, n2.Next)
	require.Equal(t, "merchant", n1.Owner)
	require.Equal(t, "processor", n2.Owner)
}

func TestGraphBuilder_BuildGraph_JSON_Succeeds(t *testing.T) {
	b := service.NewGraphBuilder()

	contract := domain.ContractYAML{
		ContractName: "c",
		Parties:      []string{},
		Steps:        []domain.Step{{ID: "s1"}},
	}
	payload, err := json.Marshal(contract)
	require.NoError(t, err)

	graph, err := b.BuildGraph(payload)
	require.NoError(t, err)
	require.Contains(t, graph.Graph.Nodes, "s1")
}

func TestGraphBuilder_BuildGraph_InvalidInputErrors(t *testing.T) {
	b := service.NewGraphBuilder()

	_, err := b.BuildGraph([]byte(":\n-"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to parse contract")
}

func TestGraphBuilder_BuildGraph_ValidatesParties(t *testing.T) {
	b := service.NewGraphBuilder()

	content := []byte(`
contract_name: test
parties: [merchant]
steps:
  - id: s1
    owner: merchant
  - id: s2
    owner: processor
transitions:
  - from: s1
    to: [s2]
`)

	_, err := b.BuildGraph(content)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not defined in contract parties")
}

func TestGraphBuilder_BuildGraph_GroupsAndParentGroup(t *testing.T) {
	b := service.NewGraphBuilder()

	content := []byte(`
contract_name: test
parties: [merchant]
steps:
  - id: s1
    owner: merchant
  - id: s2
    owner: merchant
  - id: s3
    owner: merchant
groups:
  - id: g1
    steps: [s2, s3]
    rules:
      all_must_succeed: true
      max_combined_duration: 1h
transitions:
  - from: s1
    to: [s2]
  - from: s2
    to: [s3]
  - from: s3
    to: [s1]
`)

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

// NOTE: buildTransitionsFromDependsOn is intentionally unexported; its behaviour is covered
// via BuildGraph tests that validate depends_on transition derivation.
