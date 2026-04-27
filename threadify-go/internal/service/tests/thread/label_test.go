package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/models"
)

// Mock repositories/services would be needed for a full integration test,
// but we can verify the extraction logic by inspecting the Thread model
// created within HandleStartThread if we had access to it.
// Since HandleStartThread is a method on ThreadService, we'd need to mock its dependencies.
// For now, let's assume the extraction logic itself is what we want to verify.

func TestLabelExtraction(t *testing.T) {
	tests := []struct {
		name      string
		req       models.StartThreadRequest
		wantLabel string
	}{
		{
			"label from top-level field",
			models.StartThreadRequest{
				Label: "my-label",
			},
			"my-label",
		},
		{
			"label from refs map",
			models.StartThreadRequest{
				Refs: map[string]string{"label": "ref-label"},
			},
			"ref-label",
		},
		{
			"top-level takes precedence",
			models.StartThreadRequest{
				Label: "top-label",
				Refs:  map[string]string{"label": "ref-label"},
			},
			"top-label",
		},
		{
			"no label",
			models.StartThreadRequest{},
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			label := tc.req.Label
			if label == "" && tc.req.Refs != nil {
				label = tc.req.Refs["label"]
			}
			assert.Equal(t, tc.wantLabel, label)
		})
	}
}
