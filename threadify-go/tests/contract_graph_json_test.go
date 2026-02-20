package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphBuilder_BuildGraph_FromActualJSON(t *testing.T) {
	t.Run("parse actual JSON content from contract creation", func(t *testing.T) {
		// This is the actual JSON content that gets passed to BuildGraph
		actualJSON := `{"ContractName":"product_deliveries_new1","Version":1,"Description":"Payment processing with fraud checks","Parties":["merchant","payment_processor","bank"],"Steps":[{"ID":"payment_initiated","Owner":"merchant","Type":"","DependsOn":null,"Timeout":"","BusinessContext":{"amount":"number","currency":"string","customer_id":"string"}},{"ID":"fraud_check","Owner":"payment_processor","Type":"","DependsOn":["payment_initiated"],"Timeout":"2s","BusinessContext":{"fraud_score":"number","recommendation":"string"}},{"ID":"risk_assessment","Owner":"payment_processor","Type":"","DependsOn":["payment_initiated"],"Timeout":"2s","BusinessContext":{"risk_level":"string","risk_score":"number"}},{"ID":"bank_authorization","Owner":"bank","Type":"","DependsOn":["fraud_check","risk_assessment"],"Timeout":"5s","BusinessContext":{"authorization_code":"string","authorized":"boolean"}},{"ID":"payment_complete","Owner":"merchant","Type":"","DependsOn":["bank_authorization"],"Timeout":"","BusinessContext":{"transaction_id":"string"}}],"Groups":[{"ID":"fraud_validation","Steps":["fraud_check","risk_assessment"],"Rules":{"AllMustSucceed":true,"MaxCombinedDuration":"3s"}}],"Validation":{"MaxDuration":"10s"}}`

		builder := NewGraphBuilder()
		graph, err := builder.BuildGraph([]byte(actualJSON))

		require.NoError(t, err, "Should parse actual JSON content")
		require.NotNil(t, graph)

		// Verify we have the correct number of nodes (5 steps + 1 group)
		assert.Len(t, graph.Graph.Nodes, 6, "Should have 5 step nodes + 1 group node")

		// Verify step nodes exist
		assert.Contains(t, graph.Graph.Nodes, "payment_initiated")
		assert.Contains(t, graph.Graph.Nodes, "fraud_check")
		assert.Contains(t, graph.Graph.Nodes, "risk_assessment")
		assert.Contains(t, graph.Graph.Nodes, "bank_authorization")
		assert.Contains(t, graph.Graph.Nodes, "payment_complete")

		// Verify group node exists
		assert.Contains(t, graph.Graph.Nodes, "fraud_validation")

		// Verify payment_initiated node
		paymentInitiated := graph.Graph.Nodes["payment_initiated"]
		assert.Equal(t, "payment_initiated", paymentInitiated.ID)
		assert.Equal(t, "merchant", paymentInitiated.Role)
		assert.Equal(t, "step", paymentInitiated.Type)
		assert.Empty(t, paymentInitiated.DependsOn, "First step should have no dependencies")
		assert.ElementsMatch(t, []string{"fraud_check", "risk_assessment"}, paymentInitiated.Next)
		assert.NotNil(t, paymentInitiated.BusinessContext)
		if bcMap, ok := paymentInitiated.BusinessContext.(map[string]interface{}); ok {
			assert.Equal(t, "number", bcMap["amount"])
		}

		// Verify fraud_check node
		fraudCheck := graph.Graph.Nodes["fraud_check"]
		assert.Equal(t, "fraud_check", fraudCheck.ID)
		assert.Equal(t, "payment_processor", fraudCheck.Role)
		assert.Equal(t, "2s", fraudCheck.Timeout)
		assert.ElementsMatch(t, []string{"payment_initiated"}, fraudCheck.DependsOn)
		assert.Equal(t, "fraud_validation", fraudCheck.ParentGroup)

		// Verify bank_authorization node (depends on both fraud steps)
		bankAuth := graph.Graph.Nodes["bank_authorization"]
		assert.Equal(t, "bank_authorization", bankAuth.ID)
		assert.ElementsMatch(t, []string{"fraud_check", "risk_assessment"}, bankAuth.DependsOn)
		assert.Equal(t, "5s", bankAuth.Timeout)
		assert.ElementsMatch(t, []string{"payment_complete"}, bankAuth.Next)

		// Verify fraud_validation group
		fraudValidation := graph.Graph.Nodes["fraud_validation"]
		assert.Equal(t, "fraud_validation", fraudValidation.ID)
		assert.Equal(t, "parallel_group", fraudValidation.Type)
		assert.Equal(t, "all_of", fraudValidation.Mode)
		assert.ElementsMatch(t, []string{"fraud_check", "risk_assessment"}, fraudValidation.Steps)
		assert.Equal(t, "3s", fraudValidation.MaxDuration)
		assert.ElementsMatch(t, []string{"payment_initiated"}, fraudValidation.DependsOn)
		assert.ElementsMatch(t, []string{"bank_authorization"}, fraudValidation.Next)

		// Verify final step
		assert.Equal(t, "payment_complete", graph.Graph.FinalStep)

		// Verify graph can be serialized back to JSON
		graphJSON, err := json.Marshal(graph.Graph)
		require.NoError(t, err)
		assert.NotEmpty(t, graphJSON)

		// Verify the serialized graph has the expected structure
		var graphMap map[string]interface{}
		err = json.Unmarshal(graphJSON, &graphMap)
		require.NoError(t, err)
		assert.Contains(t, graphMap, "nodes")
		assert.Contains(t, graphMap, "final_step")
	})
}
