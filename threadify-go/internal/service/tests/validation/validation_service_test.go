package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
)

func TestValidationService_GetCurrentSteps(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	threadRepo := enginemocks.NewMockThreadRepository(ctrl)

	ctx := context.Background()
	threadID := "thread-123"
	svc := service.NewValidationService(nil, threadRepo)

	t.Run("successfully parses step names from keys", func(t *testing.T) {
		mockSteps := []string{"order_placed:idemp1", "payment_validation:idemp2", "custom_step"}
		threadRepo.EXPECT().GetCompletedSteps(ctx, threadID, true).Return(mockSteps, nil)

		got := svc.GetCurrentSteps(ctx, threadID)
		assert.Equal(t, []string{"order_placed", "payment_validation", "custom_step"}, got)
	})

	t.Run("returns empty slice on repository error", func(t *testing.T) {
		threadRepo.EXPECT().GetCompletedSteps(ctx, threadID, true).Return(nil, assert.AnError)

		got := svc.GetCurrentSteps(ctx, threadID)
		assert.Equal(t, []string{}, got)
	})
}

func TestValidationService_CheckStepTimeout_Table(t *testing.T) {
	svc := &service.ValidationService{}

	tests := []struct {
		name       string
		node       models.GraphNode
		startedAt  string
		finishedAt string
		wantViol   bool
		wantMsg    string
	}{
		{
			name: "no timeout defined returns nil",
			node: models.GraphNode{Timeout: ""},
		},
		{
			name: "invalid timeout format returns nil",
			node: models.GraphNode{Timeout: "invalid"},
		},
		{
			name:      "invalid startedAt returns nil",
			node:      models.GraphNode{Timeout: "10s"},
			startedAt: "bad-time",
		},
		{
			name:       "invalid finishedAt returns nil",
			node:       models.GraphNode{Timeout: "10s"},
			startedAt:  time.Now().Format(time.RFC3339),
			finishedAt: "bad-time",
		},
		{
			name:       "duration within limit returns nil",
			node:       models.GraphNode{Timeout: "10s"},
			startedAt:  "2026-04-13T10:00:00Z",
			finishedAt: "2026-04-13T10:00:05Z",
			wantViol:   false,
		},
		{
			name:       "duration exceeding limit returns violation",
			node:       models.GraphNode{Timeout: "1s"},
			startedAt:  "2026-04-13T10:00:00Z",
			finishedAt: "2026-04-13T10:00:05Z",
			wantViol:   true,
			wantMsg:    "Step exceeded timeout of 1s",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.CheckStepTimeout(tc.node, tc.startedAt, tc.finishedAt)
			if tc.wantViol {
				require.NotNil(t, got)
				assert.Contains(t, got.Message, tc.wantMsg)
				assert.Equal(t, tc.node.Timeout, got.Limit)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestValidationService_CheckMaxDuration_Table(t *testing.T) {
	svc := &service.ValidationService{}

	tests := []struct {
		name     string
		thread   *models.Thread
		graph    *models.ContractGraph
		wantViol bool
		wantMsg  string
	}{
		{
			name:   "no validation defined returns nil",
			thread: &models.Thread{},
			graph:  &models.ContractGraph{Validation: nil},
		},
		{
			name:   "no max duration defined returns nil",
			thread: &models.Thread{},
			graph:  &models.ContractGraph{Validation: &models.Validation{MaxDuration: ""}},
		},
		{
			name:   "invalid max duration format returns nil",
			thread: &models.Thread{},
			graph:  &models.ContractGraph{Validation: &models.Validation{MaxDuration: "invalid"}},
		},
		{
			name:     "duration within limit returns nil",
			thread:   &models.Thread{StartedAt: time.Now().Add(-10 * time.Minute)},
			graph:    &models.ContractGraph{Validation: &models.Validation{MaxDuration: "1h"}},
			wantViol: false,
		},
		{
			name:     "duration exceeding limit returns violation",
			thread:   &models.Thread{StartedAt: time.Now().Add(-2 * time.Hour)},
			graph:    &models.ContractGraph{Validation: &models.Validation{MaxDuration: "1h"}},
			wantViol: true,
			wantMsg:  "Thread exceeded maximum duration of 1h",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.CheckMaxDuration(tc.thread, tc.graph)
			if tc.wantViol {
				require.NotNil(t, got)
				assert.Contains(t, got.Message, tc.wantMsg)
				assert.Equal(t, tc.graph.Validation.MaxDuration, got.Limit)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}

func TestValidationService_CheckMissingOptionalFields_Table(t *testing.T) {
	svc := &service.ValidationService{}

	tests := []struct {
		name      string
		node      models.GraphNode
		context   map[string]string
		wantViol  bool
		wantCount int
	}{
		{
			name: "no business context defined returns nil",
			node: models.GraphNode{BusinessContext: nil},
		},
		{
			name: "invalid business context type returns nil",
			node: models.GraphNode{BusinessContext: "not-a-context"},
		},
		{
			name: "no optional fields defined returns nil",
			node: models.GraphNode{BusinessContext: &models.BusinessContext{Optional: nil}},
		},
		{
			name: "all optional fields present returns nil",
			node: models.GraphNode{BusinessContext: &models.BusinessContext{Optional: []string{"field1", "field2"}}},
			context: map[string]string{
				"field1": "val1",
				"field2": "val2",
				"other":  "val3",
			},
			wantViol: false,
		},
		{
			name: "missing optional fields returns violation",
			node: models.GraphNode{BusinessContext: &models.BusinessContext{Optional: []string{"field1", "field2", "field3"}}},
			context: map[string]string{
				"field1": "val1",
			},
			wantViol:  true,
			wantCount: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.CheckMissingOptionalFields(tc.node, tc.context)
			if tc.wantViol {
				require.NotNil(t, got)
				assert.Len(t, got.MissingFields, tc.wantCount)
				assert.Contains(t, got.Message, "Missing 2 optional field(s)")
			} else {
				assert.Nil(t, got)
			}
		})
	}
}
