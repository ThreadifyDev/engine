package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphBuilder_BuildGraph_LinearWorkflow(t *testing.T) {
	t.Run("simple linear workflow A -> B -> C", func(t *testing.T) {
		contractYAML := `
contract_name: linear_flow
version: 1
description: Simple linear workflow
parties:
  - service_a
  - service_b
steps:
  - id: step_a
    owner: service_a
  - id: step_b
    owner: service_a
    depends_on:
      - step_a
  - id: step_c
    owner: service_b
    depends_on:
      - step_b
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		require.NotNil(t, graph)

		// Verify graph structure
		assert.Len(t, graph.Graph.Nodes, 3)

		// Verify step_a (root node)
		stepA := graph.Graph.Nodes["step_a"]
		assert.Equal(t, "step_a", stepA.ID)
		assert.Equal(t, "step", stepA.Type)
		assert.Empty(t, stepA.DependsOn)
		assert.Equal(t, []string{"step_b"}, stepA.Next)

		// Verify step_b (middle node)
		stepB := graph.Graph.Nodes["step_b"]
		assert.Equal(t, "step_b", stepB.ID)
		assert.Equal(t, []string{"step_a"}, stepB.DependsOn)
		assert.Equal(t, []string{"step_c"}, stepB.Next)

		// Verify step_c (final node)
		stepC := graph.Graph.Nodes["step_c"]
		assert.Equal(t, "step_c", stepC.ID)
		assert.Equal(t, []string{"step_b"}, stepC.DependsOn)
		assert.Empty(t, stepC.Next)

		// Verify final step
		assert.Equal(t, "step_c", graph.Graph.FinalStep)
	})
}

func TestGraphBuilder_BuildGraph_ParallelSteps(t *testing.T) {
	t.Run("parallel steps A -> [B, C] -> D", func(t *testing.T) {
		contractYAML := `
contract_name: parallel_flow
version: 1
steps:
  - id: step_a
    owner: service_a
  - id: step_b
    owner: service_b
    depends_on:
      - step_a
  - id: step_c
    owner: service_c
    depends_on:
      - step_a
  - id: step_d
    owner: service_d
    depends_on:
      - step_b
      - step_c
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		require.NotNil(t, graph)

		// Verify step_a has two next steps
		stepA := graph.Graph.Nodes["step_a"]
		assert.ElementsMatch(t, []string{"step_b", "step_c"}, stepA.Next)

		// Verify step_b and step_c are parallel (both depend on step_a)
		stepB := graph.Graph.Nodes["step_b"]
		assert.Equal(t, []string{"step_a"}, stepB.DependsOn)
		assert.Equal(t, []string{"step_d"}, stepB.Next)

		stepC := graph.Graph.Nodes["step_c"]
		assert.Equal(t, []string{"step_a"}, stepC.DependsOn)
		assert.Equal(t, []string{"step_d"}, stepC.Next)

		// Verify step_d depends on both parallel steps
		stepD := graph.Graph.Nodes["step_d"]
		assert.ElementsMatch(t, []string{"step_b", "step_c"}, stepD.DependsOn)
		assert.Empty(t, stepD.Next)

		// Verify final step
		assert.Equal(t, "step_d", graph.Graph.FinalStep)
	})
}

func TestGraphBuilder_BuildGraph_WithGroups(t *testing.T) {
	t.Run("workflow with parallel group", func(t *testing.T) {
		contractYAML := `
contract_name: payment_flow
version: 1
steps:
  - id: payment_initiated
    owner: merchant
  - id: fraud_check
    owner: payment_processor
    depends_on:
      - payment_initiated
    timeout: 2s
  - id: risk_assessment
    owner: payment_processor
    depends_on:
      - payment_initiated
    timeout: 2s
  - id: bank_authorization
    owner: bank
    depends_on:
      - fraud_check
      - risk_assessment
    timeout: 5s
groups:
  - id: fraud_validation
    steps:
      - fraud_check
      - risk_assessment
    rules:
      all_must_succeed: true
      max_combined_duration: 3s
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		require.NotNil(t, graph)

		// Verify group node exists
		group := graph.Graph.Nodes["fraud_validation"]
		assert.Equal(t, "fraud_validation", group.ID)
		assert.Equal(t, "parallel_group", group.Type)
		assert.Equal(t, "all_of", group.Mode)
		assert.ElementsMatch(t, []string{"fraud_check", "risk_assessment"}, group.Steps)
		assert.Equal(t, "3s", group.MaxDuration)

		// Verify group dependencies
		assert.Equal(t, []string{"payment_initiated"}, group.DependsOn)
		assert.Equal(t, []string{"bank_authorization"}, group.Next)

		// Verify steps have parent group
		fraudCheck := graph.Graph.Nodes["fraud_check"]
		assert.Equal(t, "fraud_validation", fraudCheck.ParentGroup)

		riskAssessment := graph.Graph.Nodes["risk_assessment"]
		assert.Equal(t, "fraud_validation", riskAssessment.ParentGroup)
	})
}

func TestGraphBuilder_BuildGraph_WithTimeout(t *testing.T) {
	t.Run("steps with timeout", func(t *testing.T) {
		contractYAML := `
contract_name: timeout_flow
version: 1
steps:
  - id: step_a
    owner: service_a
    timeout: 5s
  - id: step_b
    owner: service_b
    depends_on:
      - step_a
    timeout: 10s
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)

		stepA := graph.Graph.Nodes["step_a"]
		assert.Equal(t, "5s", stepA.Timeout)

		stepB := graph.Graph.Nodes["step_b"]
		assert.Equal(t, "10s", stepB.Timeout)
	})
}

func TestGraphBuilder_BuildGraph_WithBusinessContext(t *testing.T) {
	t.Run("steps with business context", func(t *testing.T) {
		contractYAML := `
contract_name: context_flow
version: 1
steps:
  - id: payment_initiated
    owner: merchant
    business_context:
      amount: number
      currency: string
      customer_id: string
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)

		step := graph.Graph.Nodes["payment_initiated"]
		assert.NotNil(t, step.BusinessContext)
		assert.Equal(t, "number", step.BusinessContext["amount"])
		assert.Equal(t, "string", step.BusinessContext["currency"])
		assert.Equal(t, "string", step.BusinessContext["customer_id"])
	})
}

func TestGraphBuilder_BuildGraph_NoGroups(t *testing.T) {
	t.Run("contract without groups", func(t *testing.T) {
		contractYAML := `
contract_name: simple_flow
version: 1
steps:
  - id: step_a
    owner: service_a
  - id: step_b
    owner: service_b
    depends_on:
      - step_a
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)

		// Should only have step nodes, no group nodes
		assert.Len(t, graph.Graph.Nodes, 2)
		for _, node := range graph.Graph.Nodes {
			assert.Equal(t, "step", node.Type)
		}
	})
}

func TestGraphBuilder_BuildGraph_ErrorCases(t *testing.T) {
	t.Run("invalid YAML", func(t *testing.T) {
		invalidYAML := `invalid: yaml: content:`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(invalidYAML))

		assert.Error(t, err)
		assert.Nil(t, graph)
	})

	t.Run("empty content", func(t *testing.T) {
		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(""))

		// Empty YAML is valid, creates empty graph
		require.NoError(t, err)
		assert.NotNil(t, graph)
		assert.Empty(t, graph.Graph.Nodes)
	})

	t.Run("missing contract name", func(t *testing.T) {
		contractYAML := `
version: 1
steps:
  - id: step_a
    owner: service_a
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		// Should still work, contract_name is optional in YAML
		require.NoError(t, err)
		assert.NotNil(t, graph)
	})
}

func TestGraphBuilder_BuildGraph_FinalStepDetection(t *testing.T) {
	t.Run("detect final step correctly", func(t *testing.T) {
		contractYAML := `
contract_name: final_step_test
version: 1
steps:
  - id: start
    owner: service_a
  - id: middle
    owner: service_b
    depends_on:
      - start
  - id: end
    owner: service_c
    depends_on:
      - middle
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		assert.Equal(t, "end", graph.Graph.FinalStep)
	})

	t.Run("multiple final steps - pick first", func(t *testing.T) {
		contractYAML := `
contract_name: multi_final
version: 1
steps:
  - id: start
    owner: service_a
  - id: end_a
    owner: service_b
    depends_on:
      - start
  - id: end_b
    owner: service_c
    depends_on:
      - start
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		// Should pick one of the final steps
		assert.NotEmpty(t, graph.Graph.FinalStep)
		assert.Contains(t, []string{"end_a", "end_b"}, graph.Graph.FinalStep)
	})
}

func TestGraphBuilder_BuildGraph_GroupModes(t *testing.T) {
	t.Run("group with all_of mode", func(t *testing.T) {
		contractYAML := `
contract_name: all_of_test
version: 1
steps:
  - id: step_a
    owner: service_a
  - id: step_b
    owner: service_b
groups:
  - id: group_1
    steps:
      - step_a
      - step_b
    rules:
      all_must_succeed: true
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		group := graph.Graph.Nodes["group_1"]
		assert.Equal(t, "all_of", group.Mode)
	})

	t.Run("group with any_of mode (default)", func(t *testing.T) {
		contractYAML := `
contract_name: any_of_test
version: 1
steps:
  - id: step_a
    owner: service_a
  - id: step_b
    owner: service_b
groups:
  - id: group_1
    steps:
      - step_a
      - step_b
    rules:
      all_must_succeed: false
`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(contractYAML))

		require.NoError(t, err)
		group := graph.Graph.Nodes["group_1"]
		assert.Equal(t, "any_of", group.Mode)
	})
}
