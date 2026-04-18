package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	natsmocks "github.com/threadify/engine/internal/service/mocks/nats"
	timeoutmocks "github.com/threadify/engine/internal/service/mocks/timeout"
	"go.uber.org/zap"
)

func TestTimeoutMonitor_HandleTimeoutEvent_AcksMalformedPayload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	pub := enginemocks.NewMockNotificationPublisher(ctrl)
	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte("this-is-not-json")).Times(1)
	msg.EXPECT().Ack().Return(nil).Times(1)

	tm := service.NewTimeoutMonitorForTests(kv, pub, zap.NewNop())
	err := tm.HandleTimeoutEvent(msg)
	require.Error(t, err)
}

func TestTimeoutMonitor_HandleTimeoutEvent_CancelledSkipsPublishAndAcks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	pub := enginemocks.NewMockNotificationPublisher(ctrl)
	msg := natsmocks.NewMockMsg(ctrl)

	event := models.TimeoutEvent{
		ID:           "timeout-1",
		ThreadID:     "thread-1",
		Type:         models.TimeoutTypeTransition,
		FromStep:     "step-a",
		ToStep:       "step-b",
		Timeout:      "2m",
		ContractName: "contract-a",
		ScheduledAt:  time.Now().UTC(),
		DeadlineAt:   time.Now().UTC().Add(2 * time.Minute),
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	msg.EXPECT().Data().Return(data).Times(1)
	kv.EXPECT().Get(gomock.Any(), service.TimeoutCancellationKey(event.ID)).Return(nil, nil).Times(1)
	msg.EXPECT().Ack().Return(nil).Times(1)

	tm := service.NewTimeoutMonitorForTests(kv, pub, zap.NewNop())
	err = tm.HandleTimeoutEvent(msg)
	require.NoError(t, err)
	assert.Equal(t, uint64(0), tm.GetMetrics()["fired"])
}

func TestTimeoutMonitor_HandleTimeoutEvent_PublishErrorDoesNotAck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	pub := enginemocks.NewMockNotificationPublisher(ctrl)
	msg := natsmocks.NewMockMsg(ctrl)

	event := models.TimeoutEvent{
		ID:           "timeout-2",
		ThreadID:     "thread-2",
		Type:         models.TimeoutTypeTransition,
		FromStep:     "step-a",
		ToStep:       "step-b",
		Timeout:      "2m",
		ContractName: "contract-b",
		ScheduledAt:  time.Now().UTC(),
		DeadlineAt:   time.Now().UTC().Add(2 * time.Minute),
		Metadata:     map[string]interface{}{"owner_id": "owner-1"},
	}
	data, err := json.Marshal(event)
	require.NoError(t, err)

	msg.EXPECT().Data().Return(data).Times(1)
	kv.EXPECT().Get(gomock.Any(), service.TimeoutCancellationKey(event.ID)).Return(nil, jetstream.ErrKeyNotFound).Times(1)
	pub.EXPECT().PublishNotification(gomock.Any(), gomock.AssignableToTypeOf(models.ValidationNotification{})).
		Return(errors.New("publish failed")).Times(1)

	tm := service.NewTimeoutMonitorForTests(kv, pub, zap.NewNop())
	err = tm.HandleTimeoutEvent(msg)
	require.Error(t, err)
	assert.Equal(t, uint64(1), tm.GetMetrics()["fired"])
	assert.Equal(t, uint64(0), tm.GetMetrics()["violations"])
}

func TestTimeoutMonitor_HandleTimeoutEvent_ConcurrentSuccess_IsRaceSafe(t *testing.T) {
	const count = 64

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	pub := enginemocks.NewMockNotificationPublisher(ctrl)
	kv.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, jetstream.ErrKeyNotFound).Times(count)
	pub.EXPECT().PublishNotification(gomock.Any(), gomock.AssignableToTypeOf(models.ValidationNotification{})).Return(nil).Times(count)

	tm := service.NewTimeoutMonitorForTests(kv, pub, zap.NewNop())

	var wg sync.WaitGroup
	wg.Add(count)
	errCh := make(chan error, count)

	for i := 0; i < count; i++ {
		event := models.TimeoutEvent{
			ID:           fmt.Sprintf("timeout-%d", i),
			ThreadID:     "thread-1",
			Type:         models.TimeoutTypeTransition,
			FromStep:     "step-a",
			ToStep:       "step-b",
			Timeout:      "2m",
			ContractName: "contract-c",
			ScheduledAt:  time.Now().UTC(),
			DeadlineAt:   time.Now().UTC().Add(2 * time.Minute),
			Metadata:     map[string]interface{}{"owner_id": "owner-1"},
		}
		data, err := json.Marshal(event)
		require.NoError(t, err)

		msg := natsmocks.NewMockMsg(ctrl)
		msg.EXPECT().Data().Return(data).Times(1)
		msg.EXPECT().Ack().Return(nil).Times(1)

		go func(m *natsmocks.MockMsg) {
			defer wg.Done()
			errCh <- tm.HandleTimeoutEvent(m)
		}(msg)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	metrics := tm.GetMetrics()
	assert.Equal(t, uint64(count), metrics["fired"])
	assert.Equal(t, uint64(count), metrics["violations"])
}

func TestTimeoutMonitor_CancelTimeout_SanitizesKeyAndIncrementsMetric(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	kv := timeoutmocks.NewMocktimeoutKV(ctrl)
	pub := enginemocks.NewMockNotificationPublisher(ctrl)

	var captured models.TimeoutCancellation
	timeoutID := "timeout:abc"

	kv.EXPECT().Put(gomock.Any(), service.TimeoutCancellationKey(timeoutID), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, value []byte) (uint64, error) {
			if err := json.Unmarshal(value, &captured); err != nil {
				return 0, err
			}
			return 1, nil
		}).
		Times(1)

	tm := service.NewTimeoutMonitorForTests(kv, pub, zap.NewNop())
	err := tm.CancelTimeout(timeoutID, "thread-123", "step-started")
	require.NoError(t, err)
	assert.Equal(t, timeoutID, captured.TimeoutID)
	assert.Equal(t, "thread-123", captured.ThreadID)
	assert.Equal(t, "step-started", captured.Reason)
	assert.Equal(t, uint64(1), tm.GetMetrics()["cancelled"])
}

func TestTimeoutMonitor_Stop_IsIdempotentUnderConcurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tm := service.NewTimeoutMonitorForTests(timeoutmocks.NewMocktimeoutKV(ctrl), enginemocks.NewMockNotificationPublisher(ctrl), zap.NewNop())

	var wg sync.WaitGroup
	const callers = 32
	wg.Add(callers)

	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			tm.Stop()
		}()
	}

	wg.Wait()
}
