package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func TestStepEventService_RecordStepEventDirect_ValidatesRequiredFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	valkeyRepo := enginemocks.NewMockValkeyClient(ctrl)
	ses, err := service.NewStepEventService(
		valkeyRepo,
		nil,
		nil,
		&config.Config{Security: config.SecurityConfig{
			HashChainCurrentVersion: "v1",
			HashChainSecrets:        map[string]string{"v1": "secret"},
		}},
		zap.NewNop(),
	)
	require.NoError(t, err)

	tests := []struct {
		name    string
		event   models.StepEvent
		wantErr error
	}{
		{
			name: "missing step id",
			event: models.StepEvent{
				ThreadID: "t1",
				Context:  map[string]interface{}{"foo": "bar"},
			},
			wantErr: service.ErrStepIdRequired,
		},
		{
			name: "missing thread id",
			event: models.StepEvent{
				StepID:  "s1",
				Context: map[string]interface{}{"foo": "bar"},
			},
			wantErr: service.ErrThreadIdRequired,
		},
		{
			name: "missing context",
			event: models.StepEvent{
				StepID:   "s1",
				ThreadID: "t1",
			},
			wantErr: service.ErrContextRequired,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ses.RecordStepEventDirect(context.Background(), tc.event, "o1", "svc1", nil)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestStepEventService_RecordStepEventDirect_ConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.SecurityConfig
		wantErr error
	}{
		{
			name:    "missing current version",
			cfg:     config.SecurityConfig{},
			wantErr: service.ErrFailedToProcessStep,
		},
		{
			name: "missing secret mapping",
			cfg: config.SecurityConfig{
				HashChainCurrentVersion: "v1",
			},
			wantErr: service.ErrFailedToProcessStep,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			valkeyRepo := enginemocks.NewMockValkeyClient(ctrl)
			ses, err := service.NewStepEventService(
				valkeyRepo,
				nil,
				nil,
				&config.Config{Security: tc.cfg},
				zap.NewNop(),
			)
			require.NoError(t, err)

			event := models.StepEvent{
				ThreadID:    "thread-123",
				StepID:      "step-456",
				StepName:    "process",
				ContentHash: "hash-789",
				Timestamp:   time.Now(),
				Context:     map[string]interface{}{"k": "v"},
			}

			err = ses.RecordStepEventDirect(context.Background(), event, "o1", "svc1", nil)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestStepEventService_RecordStepEventDirect_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	valkeyRepo := enginemocks.NewMockValkeyClient(ctrl)
	ses, err := service.NewStepEventService(
		valkeyRepo,
		nil,
		nil,
		&config.Config{Security: config.SecurityConfig{
			HashChainCurrentVersion: "v1",
			HashChainSecrets:        map[string]string{"v1": "secret"},
		}},
		zap.NewNop(),
	)
	require.NoError(t, err)

	event := models.StepEvent{
		ThreadID:    "thread-123",
		StepID:      "step-456",
		StepName:    "process",
		ContentHash: "hash-789",
		Timestamp:   time.Now(),
		Context:     map[string]interface{}{"k": "v"},
	}

	threadKey := "thread:" + event.ThreadID

	valkeyRepo.EXPECT().
		Eval(gomock.Any(), gomock.Any(), []string{threadKey}).
		Return([]interface{}{"old-hash"}, nil).
		Times(1)

	valkeyRepo.EXPECT().
		Eval(gomock.Any(), gomock.Any(), []string{threadKey}, "old-hash", gomock.Any(), "v1").
		Return([]interface{}{int64(1)}, nil).
		Times(1)

	require.NoError(t, ses.RecordStepEventDirect(context.Background(), event, "owner-1", "service-1", nil))
}
