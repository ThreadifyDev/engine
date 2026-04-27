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

func newTestIntelConsumer(t *testing.T, ctrl *gomock.Controller, db DBExecer) (*IntelligenceConsumer, *MockJetStreamPublisher) {
	t.Helper()

	js := NewMockJetStreamPublisher(ctrl)

	c, err := NewIntelligenceConsumer(js, db, 2, 100*time.Millisecond, "test-id", zap.NewNop())
	assert.NoError(t, err)

	return c, js
}

func TestIntelligenceConsumer_Flush_SuccessDeduplicatesAndAcks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.InAnyOrder([]string{"p1", "p2", "p3"})).
		Return(pgconn.NewCommandTag("INSERT 0 3"), nil).
		Times(2)

	c, _ := newTestIntelConsumer(t, ctrl, db)

	msg1 := NewMockMsg(ctrl)
	msg2 := NewMockMsg(ctrl)
	msg1.EXPECT().Ack().Return(nil).Times(1)
	msg2.EXPECT().Ack().Return(nil).Times(1)

	c.consecutiveFailures.Store(3)
	c.flush(context.Background(), []pendingIntelEvent{
		{profileIDs: []string{"p1", "p2"}, msg: msg1},
		{profileIDs: []string{"p2", "p3"}, msg: msg2},
	})

	assert.Equal(t, int32(0), c.consecutiveFailures.Load())
}

func TestIntelligenceConsumer_Flush_DBErrorNaksAndIncrementsFailures(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.InAnyOrder([]string{"p1"})).
		Return(pgconn.NewCommandTag(""), errors.New("db error")).
		Times(1)

	c, _ := newTestIntelConsumer(t, ctrl, db)

	msg := NewMockMsg(ctrl)
	msg.EXPECT().NakWithDelay(2 * time.Second).Return(nil).Times(1)

	c.flush(context.Background(), []pendingIntelEvent{
		{profileIDs: []string{"p1"}, msg: msg},
	})

	assert.Equal(t, int32(1), c.consecutiveFailures.Load())
}

func TestIntelligenceConsumer_Start_SubscribeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	js := NewMockJetStreamPublisher(ctrl)
	js.EXPECT().
		CreateOrUpdateConsumer(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("subscribe failed")).
		Times(1)

	c, err := NewIntelligenceConsumer(js, nil, 1, time.Millisecond, "test-id", zap.NewNop())
	assert.NoError(t, err)

	err = c.Start(context.Background())
	assert.Error(t, err)
}

func TestIntelligenceConsumer_CircuitBreaker_BacksOff(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cons := NewMockConsumer(ctrl)
	consCtx := NewMockConsumeContext(ctrl)
	cons.EXPECT().Consume(gomock.Any(), gomock.Any()).Return(consCtx, nil).AnyTimes()
	consCtx.EXPECT().Stop().AnyTimes()

	c, _ := newTestIntelConsumer(t, ctrl, nil)
	c.consecutiveFailures.Store(999)

	ctx, cancel := context.WithCancel(context.Background())
	c.wg.Add(1)
	go c.run(ctx, cons)

	time.Sleep(20 * time.Millisecond)
	cancel()
	c.wg.Wait()
}

func TestIntelligenceConsumer_PartnerCompatFailureIsNonFatal(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	db := NewMockDBExecer(ctrl)
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.InAnyOrder([]string{"p1"})).
		Return(pgconn.NewCommandTag("INSERT 0 1"), nil).
		Times(1)
	db.EXPECT().
		Exec(gomock.Any(), gomock.Any(), gomock.InAnyOrder([]string{"p1"})).
		Return(pgconn.NewCommandTag(""), errors.New("compat error")).
		Times(1)

	c, _ := newTestIntelConsumer(t, ctrl, db)

	msg := NewMockMsg(ctrl)
	msg.EXPECT().Ack().Return(nil).Times(1)

	c.flush(context.Background(), []pendingIntelEvent{
		{profileIDs: []string{"p1"}, msg: msg},
	})

	assert.Equal(t, int32(0), c.consecutiveFailures.Load())
	assert.Equal(t, int32(1), c.partnerCompatFailures.Load())
}

func TestIntelligenceConsumer_Stop_EmptyBuffer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cons := NewMockConsumer(ctrl)
	consCtx := NewMockConsumeContext(ctrl)
	cons.EXPECT().Consume(gomock.Any(), gomock.Any()).Return(consCtx, nil).AnyTimes()
	consCtx.EXPECT().Stop().AnyTimes()

	c, _ := newTestIntelConsumer(t, ctrl, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.wg.Add(1)
	go c.run(ctx, cons)
	time.Sleep(20 * time.Millisecond)
	c.Stop()
	c.wg.Wait()
}
