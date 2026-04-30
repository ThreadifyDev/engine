package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"github.com/threadify/engine/internal/service/tests/common"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

type mockTimeoutMonitor struct {
	scheduleFn func(ctx context.Context, event domain.TimeoutEvent) error
	cancelFn   func(ctx context.Context, timeoutID, threadID, reason string) error
}

func (m *mockTimeoutMonitor) ScheduleTimeout(ctx context.Context, event domain.TimeoutEvent) error {
	if m.scheduleFn != nil {
		return m.scheduleFn(ctx, event)
	}
	return nil
}

func (m *mockTimeoutMonitor) CancelTimeout(ctx context.Context, id, tid, r string) error {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, id, tid, r)
	}
	return nil
}

func TestNotificationService_ShouldReceiveNotification(t *testing.T) {
	svc := &service.NotificationService{}

	tests := []struct {
		name        string
		userPerms   []string
		required    []string
		userID      string
		stepOwnerID string
		want        bool
	}{
		{
			name:      "exact match",
			userPerms: []string{"notification.step.success.*"},
			required:  []string{"notification.step.success.*"},
			want:      true,
		},
		{
			name:      "wildcard match",
			userPerms: []string{"notification.*"},
			required:  []string{"notification.step.success.*"},
			want:      true,
		},
		{
			name:        "own match success",
			userPerms:   []string{"notification.step.success.own"},
			required:    []string{"notification.step.success.own"},
			userID:      "u1",
			stepOwnerID: "u1",
			want:        true,
		},
		{
			name:        "own match failure (different user)",
			userPerms:   []string{"notification.step.success.own"},
			required:    []string{"notification.step.success.own"},
			userID:      "u1",
			stepOwnerID: "u2",
			want:        false,
		},
		{
			name:      "no match",
			userPerms: []string{"thread.read"},
			required:  []string{"notification.step.success.*"},
			want:      false,
		},
		{
			name:      "empty perms",
			userPerms: []string{},
			required:  []string{"a.b"},
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := svc.ShouldReceiveNotification(tc.userPerms, tc.required, tc.userID, tc.stepOwnerID)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestNotificationService_ScheduleThreadTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tm := &mockTimeoutMonitor{}
	svc := service.NewNotificationService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, tm, zap.NewNop())

	t.Run("success", func(t *testing.T) {
		scheduled := false
		tm.scheduleFn = func(ctx context.Context, event domain.TimeoutEvent) error {
			scheduled = true
			assert.Equal(t, "t1:max_duration", event.ID)
			assert.Equal(t, domain.TimeoutTypeMaxDuration, event.Type)
			return nil
		}

		graph := &domain.ContractGraph{
			Validation: &domain.Validation{MaxDuration: "1h"},
		}
		thread := &domain.Thread{ID: "t1", ContractName: "c1"}

		svc.ScheduleThreadMaxDurationTimeout(context.Background(), "t1", graph, thread, time.Now())
		assert.True(t, scheduled)
	})

	t.Run("missing validation section", func(t *testing.T) {
		svc.ScheduleThreadMaxDurationTimeout(context.Background(), "t1", &domain.ContractGraph{}, &domain.Thread{}, time.Now())
	})

	t.Run("invalid duration", func(t *testing.T) {
		graph := &domain.ContractGraph{
			Validation: &domain.Validation{MaxDuration: "bad"},
		}
		svc.ScheduleThreadMaxDurationTimeout(context.Background(), "t1", graph, &domain.Thread{}, time.Now())
	})
}

func TestNotificationService_CancelThreadTimeout(t *testing.T) {
	tm := &mockTimeoutMonitor{}
	svc := service.NewNotificationService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, tm, zap.NewNop())

	cancelled := false
	tm.cancelFn = func(ctx context.Context, id, tid, r string) error {
		cancelled = true
		assert.Equal(t, "t1:max_duration", id)
		return nil
	}

	svc.CancelThreadMaxDurationTimeout(context.Background(), "t1")
	assert.True(t, cancelled)
}

func TestNotificationService_HandleNoContractStep(t *testing.T) {
	d := common.NewMockDeps(t)
	defer d.Ctrl.Finish()

	pub := enginemocks.NewMockNotificationPublisher(d.Ctrl)
	pool := workerpool.New(workerpool.Config{MinWorkers: 1, MaxWorkers: 1})
	defer pool.Shutdown(context.Background())

	svc := service.NewNotificationService(nil, d.ActivityRepo, nil, nil, nil, pub, nil, nil, nil, nil, pool, nil, d.Logger)

	thread := &domain.Thread{ID: "t1", ContractName: ""}
	req := &domain.RecordEventCmd{
		ThreadID: "t1",
		StepName: "step1",
		Status:   "success",
	}

	d.ActivityRepo.EXPECT().ArchiveStepState(gomock.Any(), gomock.Any()).Return(nil)

	svc.HandleNoContractStep(context.Background(), "t1", "s1", "step1", "o1", req, thread)
}

func TestGetRequiredPermissionsForNotification(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		stepSt    string
		severity  string
		violType  string
		wantCount int
	}{
		{"none success", "none", "success", "", "", 2},
		{"none failed", "none", "failed", "", "", 2},
		{"violated generic", "violated", "", "warning", "", 2},
		{"violated critical", "violated", "", "critical", "", 3},
		{"violated timeout", "violated", "", "", "step_timeout_exceeded", 4},
		{"passed", "passed", "", "", "", 2},
		{"unknown", "other", "", "", "", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			perms := service.GetRequiredPermissionsForNotification(tc.status, tc.stepSt, tc.severity, tc.violType)
			assert.Len(t, perms, tc.wantCount)
		})
	}
}
