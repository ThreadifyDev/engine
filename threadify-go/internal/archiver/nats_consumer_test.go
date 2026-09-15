package archiver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

func newTestNATSConsumer(js JetStreamPublisher, w ConsumerWriter, cfg *config.Config, logger *zap.Logger) *NATSConsumer {
	return &NATSConsumer{
		js:           js,
		writer:       w,
		batchSize:    1,
		batchTimeout: time.Second,
		consumerName: "test-consumer",
		stopChan:     make(chan struct{}),
		cfg:          cfg,
		logger:       logger,
	}
}

func TestNATSConsumer_parseBatchEvents_EventIDFallbackFromMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)
	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	msg := NewMockMsg(ctrl)
	msg.EXPECT().Metadata().Return(&jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: 123}}, nil).AnyTimes()

	dataArray := []map[string]interface{}{
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-100), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:00Z",
		},
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-200), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:01Z",
		},
	}

	events := c.parseBatchEvents(msg, dataArray)

	if assert.Len(t, events, 2) {
		assert.Equal(t, "nats:usage.sync:123:0", events[0].EventID)
		assert.Equal(t, "nats:usage.sync:123:1", events[1].EventID)
		assert.Equal(t, int64(-100), events[0].Amount)
		assert.Equal(t, int64(-200), events[1].Amount)
	}
}

func TestNATSConsumer_parseBatchEvents_ExplicitEventIDTakesPriority(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)
	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	msg := NewMockMsg(ctrl)
	msg.EXPECT().Metadata().Return(nil, errors.New("metadata unavailable")).AnyTimes()

	dataArray := []map[string]interface{}{
		{
			"event_id":   "explicit-abc",
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-50), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:00Z",
		},
	}

	events := c.parseBatchEvents(msg, dataArray)

	if assert.Len(t, events, 1) {
		assert.Equal(t, "explicit-abc", events[0].EventID)
	}
}

func TestNATSConsumer_parseBatchEvents_SkipsInvalidEntriesAndUnresolvableEventID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)
	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	msg := NewMockMsg(ctrl)
	msg.EXPECT().Metadata().Return(nil, errors.New("metadata unavailable")).AnyTimes()

	dataArray := []map[string]interface{}{
		{"meter": "credit_spend"},
		{"company_id": "c1"},
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": true, "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:00Z",
		},
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-100), "billing_cycle_start": "bad",
			"timestamp": "2026-04-13T00:00:00Z",
		},
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-100), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "bad",
		},
		{
			"company_id": "c1", "meter": "credit_spend",
			"amount": float64(-100), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:00Z",
		},
	}

	assert.Empty(t, c.parseBatchEvents(msg, dataArray))
}

func TestNATSConsumer_processUsageSync_TermsInvalidOrEmptyMessages(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "invalid json", payload: []byte(`not-json`)},
		{name: "empty array", payload: []byte(`[]`)},
		{
			name: "valid json but all events filtered out",
			payload: func() []byte {
				b, _ := json.Marshal([]map[string]interface{}{
					{
						"meter":               "credit_spend",
						"amount":              float64(999),
						"billing_cycle_start": "2026-04-13T00:00:00Z",
						"timestamp":           "2026-04-13T00:00:00Z",
					},
				})
				return b
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			js := NewMockJetStreamPublisher(ctrl)
			writer := NewMockConsumerWriter(ctrl)
			c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())
			msg := NewMockMsg(ctrl)
			msg.EXPECT().Data().Return(tt.payload).AnyTimes()
			msg.EXPECT().Term().Return(nil).Times(1)

			err := c.processUsageSync(context.Background(), []jetstream.Msg{msg})

			assert.NoError(t, err)
		})
	}
}

func TestNATSConsumer_processUsageSync_ValidEventsReachWriter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)

	writer.EXPECT().
		SyncUsageMeters(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, events []UsageSyncEvent) error {
			assert.Len(t, events, 1)
			assert.Equal(t, int64(-100), events[0].Amount)
			return nil
		}).
		Times(1)

	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	payload, _ := json.Marshal([]map[string]interface{}{
		{
			"event_id": "ev-1", "company_id": "c1", "meter": "credit_spend",
			"amount": float64(-100), "billing_cycle_start": "2026-04-13T00:00:00Z",
			"timestamp": "2026-04-13T00:00:00Z",
		},
	})
	msg := NewMockMsg(ctrl)
	msg.EXPECT().Data().Return(payload).AnyTimes()
	msg.EXPECT().Metadata().Return(nil, errors.New("no meta")).AnyTimes()

	err := c.processUsageSync(context.Background(), []jetstream.Msg{msg})

	assert.NoError(t, err)
}

func TestNATSConsumer_processThreadMetadata_PropagatesWriterError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)

	writer.EXPECT().
		WriteThreadMetadata(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("db write failed")).
		Times(1)

	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	data, _ := json.Marshal(map[string]interface{}{
		"type": "thread_created", "threadId": "t3", "companyId": "c1",
	})
	msg := NewMockMsg(ctrl)
	msg.EXPECT().Subject().Return("metadata.thread").AnyTimes()
	msg.EXPECT().Data().Return(data).AnyTimes()

	err := c.processThreadMetadata(context.Background(), []jetstream.Msg{msg})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "db write failed")
}

func TestNATSConsumer_processActivityLog_DropsMissingThreadIDWithoutBlockingValidEvents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	writer := NewMockConsumerWriter(ctrl)

	writer.EXPECT().
		WriteActivityLog(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, events []StreamEvent) error {
			if assert.Len(t, events, 1) {
				assert.Equal(t, "thread-1", events[0].Data["threadId"])
				assert.Equal(t, "step_recorded", events[0].Data["type"])
			}
			return nil
		}).
		Times(1)

	c := newTestNATSConsumer(js, writer, &config.Config{}, zap.NewNop())

	malformedData, _ := json.Marshal(map[string]interface{}{
		"type":  "access_granted",
		"actor": "user-1",
	})
	malformed := NewMockMsg(ctrl)
	malformed.EXPECT().Data().Return(malformedData).AnyTimes()

	validData, _ := json.Marshal(map[string]interface{}{
		"threadId": "thread-1",
		"type":     "step_recorded",
		"hash":     "hmac-sha256-v1:abc",
	})
	valid := NewMockMsg(ctrl)
	valid.EXPECT().Data().Return(validData).AnyTimes()
	valid.EXPECT().Subject().Return("activity.log").AnyTimes()

	err := c.processActivityLog(context.Background(), []jetstream.Msg{malformed, valid})

	assert.NoError(t, err)
}
