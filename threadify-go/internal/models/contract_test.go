package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractVersion_WithGraph(t *testing.T) {
	t.Run("serialization with graph field", func(t *testing.T) {
		graphJSON := json.RawMessage(`{"contract_id":"test","version":1,"graph":{"nodes":{},"final_step":""}}`)

		cv := ContractVersion{
			ID:          "cv_123",
			Version:     1,
			Content:     "contract content",
			ContentHash: "hash123",
			ContractID:  "contract_123",
			CreatedBy:   "user_123",
			Graph:       graphJSON,
		}

		// Test that graph field is included in serialization
		data, err := json.Marshal(cv)
		require.NoError(t, err)

		var result map[string]interface{}
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.NotNil(t, result["graph"])
	})

	t.Run("nil graph is valid", func(t *testing.T) {
		cv := ContractVersion{
			ID:          "cv_123",
			Version:     1,
			Content:     "contract content",
			ContentHash: "hash123",
			ContractID:  "contract_123",
			CreatedBy:   "user_123",
			Graph:       nil, // No graph
		}

		// Should serialize without error
		data, err := json.Marshal(cv)
		require.NoError(t, err)
		assert.NotNil(t, data)
	})

	t.Run("deserialization with graph field", func(t *testing.T) {
		jsonData := `{
			"id": "cv_123",
			"version": 1,
			"content": "contract content",
			"contentHash": "hash123",
			"contractId": "contract_123",
			"createdBy": "user_123",
			"graph": {"contract_id":"test","version":1}
		}`

		var cv ContractVersion
		err := json.Unmarshal([]byte(jsonData), &cv)
		require.NoError(t, err)

		assert.Equal(t, "cv_123", cv.ID)
		assert.NotNil(t, cv.Graph)
	})
}
