package service_test

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/validator"
	"strings"
	"testing"
)

const freshFeature = `Feature: repeated_charges
Rule: Approval
 When step "approval" is submitted
 Then owner must be "processor"
 And this step is an entry point
Rule: Charge
 When step "charge" is submitted
 Then owner must be "processor"
 And step "approval" must succeed before each invocation
Rule: Finish
 When step "finish" is submitted
 Then owner must be "processor"
 And step "charge" must have succeeded
 And this step is terminal
`

func TestFreshPrerequisiteContractRoundTrip(t *testing.T) {
	contract, result := validator.NewContractValidator().Validate(freshFeature)
	require.True(t, result.IsValid, "%v", result.Errors)
	require.Equal(t, []string{"approval"}, contract.Steps[1].FreshDependsOn)
	_, compiled, err := validator.NewContractValidator().SerializeContract(contract)
	require.NoError(t, err)
	graph, err := service.NewGraphBuilder().BuildGraph([]byte(freshFeature))
	require.NoError(t, err)
	fromJSON, err := service.NewGraphBuilder().BuildGraph([]byte(compiled))
	require.NoError(t, err)
	require.Equal(t, graph.Graph.Nodes["charge"].FreshDependsOn, fromJSON.Graph.Nodes["charge"].FreshDependsOn)
	raw, err := json.Marshal(mapper.ToContractGraphDTO(graph))
	require.NoError(t, err)
	var wire dto.ContractGraphDTO
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.Equal(t, []string{"approval"}, mapper.FromContractGraphDTO(&wire).Graph.Nodes["charge"].FreshDependsOn)
	for _, bad := range []string{"unknown", "charge"} {
		_, result = validator.NewContractValidator().Validate(strings.Replace(freshFeature, `step "approval" must succeed`, `step "`+bad+`" must succeed`, 1))
		require.False(t, result.IsValid)
	}
}
