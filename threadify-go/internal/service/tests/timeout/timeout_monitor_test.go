package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	timeoutmocks "github.com/threadify/engine/internal/service/mocks/timeout"
	"go.uber.org/zap"
)

func TestBuildTimeoutViolationNotification_Table(t *testing.T) {
	thread := &models.Thread{
		ID:           "thread-123",
		OwnerID:      "owner-123",
		ContractName: "product_delivery",
	}

	tests := []struct {
		name               string
		event              models.TimeoutEvent
		wantStepName       string
		wantMessageContain string
	}{
		{
			name: "transition timeout with single target step",
			event: models.TimeoutEvent{
				ID:           "timeout-1",
				ThreadID:     thread.ID,
				Type:         models.TimeoutTypeTransition,
				FromStep:     "order_placed",
				ToStep:       "payment_validation",
				Timeout:      "2m",
				ScheduledAt:  time.Now().UTC(),
				DeadlineAt:   time.Now().UTC().Add(2 * time.Minute),
				ContractName: thread.ContractName,
			},
			wantStepName:       "order_placed",
			wantMessageContain: "Expected one of payment_validation",
		},
		{
			name: "transition timeout with multiple target steps uses bracket formatting",
			event: models.TimeoutEvent{
				ID:           "timeout-2",
				ThreadID:     thread.ID,
				Type:         models.TimeoutTypeTransition,
				FromStep:     "order_placed",
				ToStep:       "payment_validation,manual_review",
				Timeout:      "2m",
				ScheduledAt:  time.Now().UTC(),
				DeadlineAt:   time.Now().UTC().Add(2 * time.Minute),
				ContractName: thread.ContractName,
			},
			wantStepName:       "order_placed",
			wantMessageContain: "[payment_validation,manual_review]",
		},
		{
			name: "max duration timeout maps to global step",
			event: models.TimeoutEvent{
				ID:           "timeout-3",
				ThreadID:     thread.ID,
				Type:         models.TimeoutTypeMaxDuration,
				Timeout:      "72h",
				ScheduledAt:  time.Now().UTC(),
				DeadlineAt:   time.Now().UTC().Add(72 * time.Hour),
				ContractName: thread.ContractName,
			},
			wantStepName:       "global",
			wantMessageContain: "Thread exceeded max_duration of 72h",
		},
		{
			name: "unknown timeout type falls back to generic message",
			event: models.TimeoutEvent{
				ID:           "timeout-4",
				ThreadID:     thread.ID,
				Type:         models.TimeoutType("custom"),
				FromStep:     "order_placed",
				Timeout:      "1m",
				ScheduledAt:  time.Now().UTC(),
				DeadlineAt:   time.Now().UTC().Add(1 * time.Minute),
				ContractName: thread.ContractName,
			},
			wantStepName:       "order_placed",
			wantMessageContain: "Timeout violation: custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := service.BuildTimeoutViolationNotification(tc.event, thread)

			assert.Equal(t, tc.event.ThreadID, n.ThreadID)
			assert.Equal(t, thread.OwnerID, n.OwnerID)
			assert.Equal(t, tc.wantStepName, n.StepName)
			assert.Equal(t, "critical", n.Severity)
			assert.Equal(t, "timeout", n.ViolationType)
			assert.Equal(t, "validation.violated.timeout", n.NotificationType)
			assert.Contains(t, n.Message, tc.wantMessageContain)
			assert.Equal(t, tc.event.ID, n.Details["timeout_id"])
			assert.Equal(t, tc.event.Timeout, n.Details["timeout"])
		})
	}
}

func TestTimeoutMonitor_IsTimeoutCancelled_Table(t *testing.T) {
	tests := []struct {
		name        string
		timeoutID   string
		getErr      error
		want        bool
		wantErrText string
	}{
		{
			name:      "missing key returns false",
			timeoutID: "timeout:abc:def",
			getErr:    jetstream.ErrKeyNotFound,
			want:      false,
		},
		{
			name:      "existing key returns true",
			timeoutID: "timeout:abc:def",
			getErr:    nil,
			want:      true,
		},
		{
			name:        "kv error returns wrapped error",
			timeoutID:   "timeout:abc:def",
			getErr:      errors.New("kv unavailable"),
			wantErrText: "get cancellation flag",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			kv := timeoutmocks.NewMocktimeoutKV(ctrl)
			tm := service.NewTimeoutMonitorForTests(kv, nil, zap.NewNop())

			kv.EXPECT().
				Get(gomock.Any(), service.TimeoutCancellationKey(tc.timeoutID)).
				Return(nil, tc.getErr).
				Times(1)

			got, err := tm.IsTimeoutCancelled(tc.timeoutID)
			if tc.wantErrText != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrText)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestTimeoutMonitor_CancelTimeout_UsesCancellationKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	tm := service.NewTimeoutMonitorForTests(kv, nil, zap.NewNop())

	timeoutID := "timeout:abc:def"
	kv.EXPECT().
		Put(gomock.Any(), service.TimeoutCancellationKey(timeoutID), gomock.Any()).
		Return(uint64(1), nil).
		Times(1)

	require.NoError(t, tm.CancelTimeout(timeoutID, "t1", "because"))
}
