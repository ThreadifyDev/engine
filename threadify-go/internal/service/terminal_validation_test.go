package service

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func TestQueuedValidationCannotArchiveStateAfterThreadCloses(t *testing.T) {
	ctrl := gomock.NewController(t)
	states := enginemocks.NewMockStepStateRepository(ctrl)
	states.EXPECT().ValidateAndUpdateStepState(gomock.Any(), gomock.Any()).Return(&domain.StepStateResult{
		Status: "error", HasCriticalViolation: true,
		Violations: []domain.Violation{{Type: "thread_already_terminal", Severity: "critical", Message: "Cannot add steps to completed thread"}},
	}, nil)
	// No archive or cache calls are permitted after the atomic terminal rejection.
	svc := &NotificationService{stepStateRepo: states, activityRepo: enginemocks.NewMockActivityRepository(ctrl), cacheManager: enginemocks.NewMockCacheManager(ctrl), logger: zap.NewNop()}
	req := &domain.RecordEventCmd{ThreadID: "thread", StepName: "charge", Status: "success", IdempotencyKey: "turn"}
	accepted := svc.processValidationNotifications(context.Background(), "thread", "step", "charge", "owner", "turn", nil, &domain.ContractGraph{}, &domain.Thread{}, "success", req)
	require.False(t, accepted, "rejected validation must not schedule transition timeouts")
}
