package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	usageoutboxmocks "github.com/threadify/engine/internal/service/mocks/usageoutbox"
	"go.uber.org/zap"
)

const (
	streamStartPending = "0"
	streamStartNew     = ">"

	blockDuration = 2 * time.Second
	noBlock       = time.Duration(0)

	errorBackoff = 500 * time.Millisecond
	emptyBackoff = 250 * time.Millisecond
)

func newRelayWithMocks(t *testing.T) (*service.UsageOutboxRelay, *gomock.Controller, *usageoutboxmocks.MockUsageOutboxBatchForwarder, *usageoutboxmocks.MockUsageOutboxSleeper, *usageoutboxmocks.MockUsageOutboxValkey) {
	t.Helper()

	ctrl := gomock.NewController(t)
	valkey := usageoutboxmocks.NewMockUsageOutboxValkey(ctrl)
	publisher := usageoutboxmocks.NewMockUsageOutboxPublisher(ctrl)
	relay := service.NewUsageOutboxRelay(valkey, publisher, zap.NewNop())

	forwarder := usageoutboxmocks.NewMockUsageOutboxBatchForwarder(ctrl)
	sleeper := usageoutboxmocks.NewMockUsageOutboxSleeper(ctrl)
	relay.SetForwarder(forwarder)
	relay.SetSleeper(sleeper)

	return relay, ctrl, forwarder, sleeper, valkey
}

func TestUsageOutboxRelay_Start_RetriesPendingFailuresWithErrorBackoff(t *testing.T) {
	relay, ctrl, forwarder, sleeper, valkey := newRelayWithMocks(t)
	defer ctrl.Finish()

	valkey.EXPECT().XGroupCreateMkStream(gomock.Any(), gomock.Any(), gomock.Any(), "0").Return(nil).Times(1)

	stopNow := make(chan struct{})
	gomock.InOrder(
		forwarder.EXPECT().ForwardBatch(gomock.Any(), streamStartPending, noBlock).Return(0, errors.New("pending read failed")),
		sleeper.EXPECT().Sleep(gomock.Any(), errorBackoff),
		forwarder.EXPECT().ForwardBatch(gomock.Any(), streamStartPending, noBlock).Return(0, nil),
		forwarder.EXPECT().ForwardBatch(gomock.Any(), streamStartNew, blockDuration).DoAndReturn(
			func(_ context.Context, _ string, _ time.Duration) (int, error) {
				close(stopNow)
				return 0, redis.Nil
			},
		),
		sleeper.EXPECT().Sleep(gomock.Any(), emptyBackoff),
	)
	// Allow any additional calls during shutdown.
	forwarder.EXPECT().ForwardBatch(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, context.Canceled).AnyTimes()
	sleeper.EXPECT().Sleep(gomock.Any(), gomock.Any()).AnyTimes()

	require.NoError(t, relay.Start())
	<-stopNow
	require.NoError(t, relay.Stop())
}

func TestUsageOutboxRelay_Start_RetriesNewMessageFailuresWithErrorBackoff(t *testing.T) {
	relay, ctrl, forwarder, sleeper, valkey := newRelayWithMocks(t)
	defer ctrl.Finish()

	valkey.EXPECT().XGroupCreateMkStream(gomock.Any(), gomock.Any(), gomock.Any(), "0").Return(nil).Times(1)

	stopNow := make(chan struct{})
	gomock.InOrder(
		forwarder.EXPECT().ForwardBatch(gomock.Any(), streamStartPending, noBlock).Return(0, nil),
		forwarder.EXPECT().ForwardBatch(gomock.Any(), streamStartNew, blockDuration).DoAndReturn(
			func(_ context.Context, _ string, _ time.Duration) (int, error) {
				close(stopNow)
				return 0, errors.New("publish failed")
			},
		),
		sleeper.EXPECT().Sleep(gomock.Any(), errorBackoff),
	)
	// Allow any additional calls during shutdown.
	forwarder.EXPECT().ForwardBatch(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, context.Canceled).AnyTimes()
	sleeper.EXPECT().Sleep(gomock.Any(), gomock.Any()).AnyTimes()

	require.NoError(t, relay.Start())
	<-stopNow
	require.NoError(t, relay.Stop())
}

func TestMapUsageOutboxEvent_Table(t *testing.T) {
	validBase := func() map[string]interface{} {
		return map[string]interface{}{
			"event_id":            "evt-1",
			"company_id":          "c-1",
			"meter":               service.MeterContractExecution,
			"amount":              "100",
			"billing_cycle_start": time.Now().UTC().Format(time.RFC3339Nano),
			"timestamp":           time.Now().UTC().Format(time.RFC3339Nano),
		}
	}

	tests := []struct {
		name   string
		input  map[string]interface{}
		wantOK bool
	}{
		{
			name:   "valid payload maps successfully",
			input:  validBase(),
			wantOK: true,
		},
		{
			name: "missing amount field rejected",
			input: func() map[string]interface{} {
				m := validBase()
				delete(m, "amount")
				return m
			}(),
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event, ok := service.MapUsageOutboxEvent(tc.input)
			assert.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				assert.Equal(t, tc.input["event_id"], event["event_id"])
			}
		})
	}
}

func TestParseRelayAmount_Table(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    int64
		wantErr bool
	}{
		{name: "string", input: "42", want: 42},
		{name: "string with spaces", input: " 7 ", want: 7},
		{name: "int64", input: int64(9), want: 9},
		{name: "float64 truncates", input: float64(3.9), want: 3},
		{name: "unsupported type fails", input: true, wantErr: true},
		{name: "invalid numeric string fails", input: "abc", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.ParseRelayAmount(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
