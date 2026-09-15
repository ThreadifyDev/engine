package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/threadify/engine/internal/dto"
	"github.com/threadify/engine/internal/mapper"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/contractcontent"
	"go.uber.org/zap"
)

type referenceReader func(context.Context, string, string) (map[string]string, error)

func (f referenceReader) GetSuccessfulContent(ctx context.Context, thread, step string) (map[string]string, error) {
	return f(ctx, thread, step)
}

func TestReferencesUseOneSnapshotAndBoundThread(t *testing.T) {
	calls := 0
	reader := referenceReader(func(_ context.Context, thread, step string) (map[string]string, error) {
		calls++
		require.Equal(t, "thread-a", thread)
		require.Equal(t, "shipment", step)
		return map[string]string{"tracking": "T1", "carrier": "C1"}, nil
	})
	svc := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop(), reader)
	node := domain.GraphNode{ContentRules: []contractcontent.Rule{
		{Field: "tracking", Operator: "equals", Reference: &contractcontent.Reference{Step: "shipment", Field: "tracking"}},
		{Field: "carrier", Operator: "equals", Reference: &contractcontent.Reference{Step: "shipment", Field: "carrier"}},
	}}
	require.NoError(t, svc.ValidateStepContext(context.Background(), node, map[string]string{"tracking": "T1", "carrier": "C1"}, "thread-a"))
	require.Equal(t, 1, calls)
	require.Error(t, svc.ValidateStepContext(context.Background(), node, map[string]string{"tracking": "T1", "carrier": "C1"}))
	require.Equal(t, 1, calls)
}

func TestReferencesDoNotSearchOlderSuccessForMissingField(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content map[string]string
		err     error
	}{{"missing field", map[string]string{}, nil}, {"missing step", nil, errors.New("no successful step")}, {"storage down", nil, errors.New("unavailable")}} {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop(), referenceReader(func(context.Context, string, string) (map[string]string, error) { return tc.content, tc.err }))
			node := domain.GraphNode{ContentRules: []contractcontent.Rule{{Field: "tracking", Operator: "equals", Reference: &contractcontent.Reference{Step: "shipment", Field: "tracking"}}}}
			require.Error(t, svc.ValidateStepContext(context.Background(), node, map[string]string{"tracking": "T1"}, "thread-a"))
		})
	}
}

func TestReferenceGraphRoundTrip(t *testing.T) {
	source, err := os.ReadFile("../../../../examples/shipping.feature")
	require.NoError(t, err)
	graph, err := service.NewGraphBuilder().BuildGraph(source)
	require.NoError(t, err)
	require.Equal(t, []string{"order_shipped"}, graph.Graph.Nodes["delivery_confirmed"].DependsOn)
	require.Empty(t, graph.Transitions)
	raw, err := json.Marshal(mapper.ToContractGraphDTO(graph))
	require.NoError(t, err)
	var dtoGraph dto.ContractGraphDTO
	require.NoError(t, json.Unmarshal(raw, &dtoGraph))
	loaded := mapper.FromContractGraphDTO(&dtoGraph)
	require.Equal(t, graph.Graph.Nodes["delivery_confirmed"].ContentRules, loaded.Graph.Nodes["delivery_confirmed"].ContentRules)
}
