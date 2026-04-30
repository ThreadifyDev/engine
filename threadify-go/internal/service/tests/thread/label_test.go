package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
)

func TestLabelExtraction(t *testing.T) {
	tests := []struct {
		name      string
		req       *domain.StartThreadCmd
		wantLabel string
	}{
		{
			"label from top-level field",
			&domain.StartThreadCmd{
				Label: "my-label",
			},
			"my-label",
		},
		{
			"label from refs map",
			&domain.StartThreadCmd{
				Refs: map[string]string{"label": "ref-label"},
			},
			"ref-label",
		},
		{
			"top-level takes precedence",
			&domain.StartThreadCmd{
				Label: "top-label",
				Refs:  map[string]string{"label": "ref-label"},
			},
			"top-label",
		},
		{
			"no label",
			&domain.StartThreadCmd{},
			"",
		},
		{
			"nil request",
			nil,
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantLabel, service.StartThreadLabel(tc.req))
		})
	}
}
