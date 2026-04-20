package archiver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func newTestStepStateConsumer(t *testing.T, ctrl *gomock.Controller, db DBExecer) (*StepStateConsumer, *MockJetStreamPublisher) {
	t.Helper()

	js := NewMockJetStreamPublisher(ctrl)

	c, err := NewStepStateConsumer(js, db, 10, time.Hour, "test-id", zap.NewNop())
	assert.NoError(t, err)

	return c, js
}

func TestStepStateConsumer_Flush_DeduplicatesLastWriterWinsAndAcksAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)

	anyArgs := make([]interface{}, 14)
	for i := range anyArgs {
		anyArgs[i] = gomock.Any()
	}
	anyArgs[4] = "finished"

	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), anyArgs...).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(1)

	c, _ := newTestStepStateConsumer(t, ctrl, db)

	msg1 := NewMockMsg(ctrl)
	msg2 := NewMockMsg(ctrl)
	msg1.EXPECT().Ack().Return(nil).Times(1)
	msg2.EXPECT().Ack().Return(nil).Times(1)

	c.mu.Lock()
	c.buffer = []pendingStepState{
		{event: stepStateEvent{ThreadID: "t1", StepName: "s1", IdempotencyKey: "k1", Status: "started"}, msg: msg1},
		{event: stepStateEvent{ThreadID: "t1", StepName: "s1", IdempotencyKey: "k1", Status: "finished"}, msg: msg2},
	}
	c.flushLocked(context.Background())
	c.mu.Unlock()
}

func TestStepStateConsumer_Flush_DBErrorNaksWithDelay(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	anyArgs := make([]interface{}, 14)
	for i := range anyArgs {
		anyArgs[i] = gomock.Any()
	}

	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), anyArgs...).
		Return(pgconn.CommandTag{}, errors.New("db error")).
		Times(1)

	c, _ := newTestStepStateConsumer(t, ctrl, db)

	msg := NewMockMsg(ctrl)
	msg.EXPECT().NakWithDelay(5 * time.Second).Return(nil).Times(1)

	c.mu.Lock()
	c.buffer = []pendingStepState{
		{event: stepStateEvent{ThreadID: "t1", StepName: "s1", IdempotencyKey: "k1"}, msg: msg},
	}
	c.flushLocked(context.Background())
	c.mu.Unlock()
}

func TestStepStateConsumer_Start_SubscribeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	js.EXPECT().
		CreateOrUpdateConsumer(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, assert.AnError).
		Times(1)

	c, err := NewStepStateConsumer(js, nil, 10, time.Millisecond, "test-id", zap.NewNop())
	assert.NoError(t, err)

	err = c.Start(context.Background())
	assert.Error(t, err)
}

func TestStepStateConsumer_Stop_FlushesBufferOnShutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	anyArgs := make([]interface{}, 14)
	for i := range anyArgs {
		anyArgs[i] = gomock.Any()
	}

	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), anyArgs...).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(1)

	c, _ := newTestStepStateConsumer(t, ctrl, db)

	msg := NewMockMsg(ctrl)
	msg.EXPECT().Ack().Return(nil).Times(1)

	c.mu.Lock()
	c.buffer = []pendingStepState{
		{event: stepStateEvent{ThreadID: "t1", StepName: "s1", IdempotencyKey: "k1", Status: "finished"}, msg: msg},
	}
	c.mu.Unlock()

	cons := NewMockConsumer(ctrl)
	consCtx := NewMockConsumeContext(ctrl)
	cons.EXPECT().Consume(gomock.Any(), gomock.Any()).Return(consCtx, nil).AnyTimes()
	consCtx.EXPECT().Stop().AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.wg.Add(1)
	go c.run(ctx, cons)
	time.Sleep(20 * time.Millisecond)
	c.Stop()
}
