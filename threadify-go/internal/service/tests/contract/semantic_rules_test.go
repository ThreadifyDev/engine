package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	"github.com/threadify/engine/pkg/contractcontent"
	"go.uber.org/zap"
)

type semanticStub struct {
	probability float64
	state       any
}

func (s *semanticStub) EvaluateAssertion(_ context.Context, state any, _ string) (float64, error) {
	s.state = state
	return s.probability, nil
}

func TestSemanticRuleUsesValidatedThreadContextAndFailsClosed(t *testing.T) {
	reader := referenceReader(func(_ context.Context, thread, step string) (map[string]string, error) {
		require.Equal(t, "thread-1", thread)
		require.Equal(t, "identity_verified", step)
		return map[string]string{"customer_id": "customer-7"}, nil
	})
	validator := service.NewContractValidationServiceFromParts(nil, nil, nil, zap.NewNop(), reader)
	node := domain.GraphNode{SemanticRules: []contractcontent.SemanticRule{{Field: "refund_note", Question: "Does this note describe a refund for the verified customer?", ContextSteps: []string{"identity_verified"}, MinProbability: 0.8}}}
	content := map[string]string{"refund_note": "Refund for customer-7"}
	require.ErrorContains(t, validator.ValidateStepContext(context.Background(), node, content, "thread-1"), "classifier is unavailable")
	classifier := &semanticStub{probability: 0.9}
	validator.SetSemanticClassifier(classifier)
	require.NoError(t, validator.ValidateStepContext(context.Background(), node, content, "thread-1"))
	state := classifier.state.(map[string]any)
	require.Equal(t, "customer-7", state["successful_steps"].(map[string]map[string]string)["identity_verified"]["customer_id"])
	classifier.probability = 0.4
	require.ErrorContains(t, validator.ValidateStepContext(context.Background(), node, content, "thread-1"), "did not satisfy")
}

func TestSemanticContextStepBecomesDependency(t *testing.T) {
	data, err := json.Marshal(domain.ContractDefinition{ContractName: "refund", Version: 1, Parties: []string{"agent"}, Steps: []domain.Step{
		{ID: "identity_verified", Owner: "agent"},
		{ID: "issue_refund", Owner: "agent", SemanticRules: []contractcontent.SemanticRule{{Field: "refund_note", Question: "Does the note describe a refund for the verified customer?", ContextSteps: []string{"identity_verified"}}}},
	}})
	require.NoError(t, err)
	graph, err := service.NewGraphBuilder().BuildGraph(data)
	require.NoError(t, err)
	require.Equal(t, []string{"identity_verified"}, graph.Graph.Nodes["issue_refund"].DependsOn)
	require.Len(t, graph.Graph.Nodes["issue_refund"].SemanticRules, 1)
}
