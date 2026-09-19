package service_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/validator"
	"go.uber.org/zap"
)

// The graph used after database/cache reload must retain every compiled content rule.
func TestGherkinPreviewPersistenceAndRuntimeChecks(t *testing.T) {
	source, err := os.ReadFile("../../../../examples/payment.feature")
	require.NoError(t, err)
	source = append(source, []byte("  And content \"reference\" must match regex \"^r[0-9]+$\"\n")...)
	svc := service.NewContractService(nil, nil, zap.NewNop())
	compiled, preview, result, err := svc.PreviewContract(string(source))
	require.NoError(t, err)
	require.True(t, result.IsValid, "%v", result.Errors)
	_, content, err := validator.NewContractValidator().SerializeContract(compiled)
	require.NoError(t, err)
	persisted, err := service.NewGraphBuilder().BuildGraph([]byte(content))
	require.NoError(t, err)
	require.Equal(t, preview, persisted)
	data, err := json.Marshal(mapper.ToContractGraphDTO(persisted))
	require.NoError(t, err)
	var graphDTO dto.ContractGraphDTO
	require.NoError(t, json.Unmarshal(data, &graphDTO))
	loaded := mapper.FromContractGraphDTO(&graphDTO)
	reloadedJSON, err := json.Marshal(mapper.ToContractGraphDTO(loaded))
	require.NoError(t, err)
	require.JSONEq(t, string(data), string(reloadedJSON))
	validation := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop())
	node := loaded.Graph.Nodes["charge"]
	require.NoError(t, validation.ValidateStepContext(context.Background(), node, map[string]string{"amount": "12.50", "currency": "GBP", "reference": "r1"}))
	for _, content := range []map[string]string{
		{"amount": "12.50", "currency": "GBP", "reference": "invalid-reference"},
		{"amount": "0", "currency": "GBP", "reference": "r1"},
		{"amount": "12.50", "currency": "XXX", "reference": "r1"},
		{"amount": "NaN", "currency": "GBP", "reference": "r1"},
		{"currency": "GBP", "reference": "r1"},
		{"amount": "12.50", "currency": "GBP"},
	} {
		require.Error(t, validation.ValidateStepContext(context.Background(), node, content))
	}
	allowed, err := service.EvaluateStepProposal("thread", domain.ThreadStatusActive, loaded, "charge", []string{"approval", "intervening_step"})
	require.NoError(t, err)
	require.True(t, allowed.Allowed)
	denied, err := service.EvaluateStepProposal("thread", domain.ThreadStatusActive, loaded, "charge", nil)
	require.NoError(t, err)
	require.False(t, denied.Allowed)
	version, err := mapper.ToContractVersionDTO(&domain.ContractVersion{YAMLContent: string(source)}, "payment_processing")
	require.NoError(t, err)
	require.Equal(t, "gherkin", version.SourceFormat)
	require.Equal(t, string(source), version.Source)
	require.Equal(t, version.Source, version.YAMLContent)
}
