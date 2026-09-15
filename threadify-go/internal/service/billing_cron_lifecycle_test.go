package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	natsmocks "github.com/threadify/engine/internal/service/mocks/nats"
	"go.uber.org/zap"
)

type cronJetStream struct {
	jetstream.JetStream
	consumer jetstream.Consumer
	err      error
}

func (js *cronJetStream) CreateOrUpdateConsumer(context.Context, string, jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	return js.consumer, js.err
}

type cronConsumer struct {
	jetstream.Consumer
	iterator jetstream.MessagesContext
	err      error
}

func (c *cronConsumer) Messages(...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	return c.iterator, c.err
}

type cronIterator struct {
	messages   chan jetstream.Msg
	nextCalled chan struct{}
	stopped    chan struct{}
	once       sync.Once
}

func (i *cronIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	select {
	case i.nextCalled <- struct{}{}:
	default:
	}
	select {
	case <-i.stopped:
		return nil, jetstream.ErrMsgIteratorClosed
	case m := <-i.messages:
		return m, nil
	}
}
func (i *cronIterator) Stop()  { i.once.Do(func() { close(i.stopped) }) }
func (i *cronIterator) Drain() { i.Stop() }

type cronBillingService struct {
	charge   func(context.Context) error
	rollover func(context.Context) error
}

func (s *cronBillingService) ChargeCreditTopup(ctx context.Context, _ string, _ string, _ time.Time, _ int64) error {
	return s.charge(ctx)
}
func (s *cronBillingService) ProcessRollovers(ctx context.Context) error {
	if s.rollover != nil {
		return s.rollover(ctx)
	}
	return nil
}
func waitCron(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second * 2):
		t.Fatal("timed out waiting for billing worker")
	}
}
func newLifecycleCron(t *testing.T) (*BillingCron, *cronIterator) {
	t.Helper()
	iter := &cronIterator{messages: make(chan jetstream.Msg, 1), nextCalled: make(chan struct{}, 1), stopped: make(chan struct{})}
	js := &cronJetStream{consumer: &cronConsumer{iterator: iter}}
	cron := NewBillingCron(nil, js, zap.NewNop())
	cron.billingService = &cronBillingService{}
	return cron, iter
}

func TestBillingCronStartReportsConsumerAndIteratorFailures(t *testing.T) {
	for _, stage := range []string{"consumer", "iterator"} {
		t.Run(stage, func(t *testing.T) {
			js := &cronJetStream{}
			if stage == "consumer" {
				js.err = errors.New("broker failed")
			} else {
				js.consumer = &cronConsumer{err: errors.New("iterator failed")}
			}
			cron := NewBillingCron(nil, js, zap.NewNop())
			require.Error(t, cron.Start())
			require.NoError(t, cron.Stop())
		})
	}
}
func TestBillingCronStopUnblocksAndJoinsIdleIterator(t *testing.T) {
	cron, iter := newLifecycleCron(t)
	require.NoError(t, cron.Start())
	waitCron(t, iter.nextCalled)
	require.NoError(t, cron.Stop())
	waitCron(t, cron.done)
	waitCron(t, iter.stopped)
	require.NoError(t, cron.Stop())
}
func TestBillingCronStopDeadlineCancelsInFlightProviderAndRollover(t *testing.T) {
	cron, iter := newLifecycleCron(t)
	charging, rolling := make(chan struct{}), make(chan struct{})
	cron.billingService = &cronBillingService{
		charge:   func(ctx context.Context) error { close(charging); <-ctx.Done(); return ctx.Err() },
		rollover: func(ctx context.Context) error { close(rolling); <-ctx.Done(); return ctx.Err() },
	}
	ctrl := gomock.NewController(t)
	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{"company_id":"company","event_id":"event","billing_cycle_start":"2026-04-01T00:00:00Z","amount":"5000"}`))
	msg.EXPECT().Nak().Return(nil)
	require.NoError(t, cron.Start())
	iter.messages <- msg
	waitCron(t, charging)
	waitCron(t, rolling)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, cron.StopContext(ctx), context.DeadlineExceeded)
	waitCron(t, cron.done)
}
func TestBillingCronStopLetsAcceptedTopupFinishBeforeReturning(t *testing.T) {
	cron, iter := newLifecycleCron(t)
	charging, release := make(chan struct{}), make(chan struct{})
	cron.billingService = &cronBillingService{charge: func(ctx context.Context) error { close(charging); <-release; return ctx.Err() }}
	ctrl := gomock.NewController(t)
	msg := natsmocks.NewMockMsg(ctrl)
	msg.EXPECT().Data().Return([]byte(`{"company_id":"company","event_id":"event","billing_cycle_start":"2026-04-01T00:00:00Z","amount":"5000"}`))
	msg.EXPECT().Ack().Return(nil)
	require.NoError(t, cron.Start())
	iter.messages <- msg
	waitCron(t, charging)
	stopped := make(chan error, 1)
	go func() { stopped <- cron.Stop() }()
	waitCron(t, iter.stopped)
	select {
	case <-stopped:
		t.Fatal("Stop returned before accepted provider request finished")
	default:
	}
	close(release)
	require.NoError(t, <-stopped)
}
