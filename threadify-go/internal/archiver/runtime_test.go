package archiver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

type runtimeIterator struct {
	messages   chan jetstream.Msg
	stopped    chan struct{}
	nextCalled chan struct{}
	once       sync.Once
}

func newRuntimeIterator() *runtimeIterator {
	return &runtimeIterator{messages: make(chan jetstream.Msg, 32), stopped: make(chan struct{}), nextCalled: make(chan struct{}, 64)}
}
func (i *runtimeIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	i.nextCalled <- struct{}{}
	select {
	case <-i.stopped:
		return nil, jetstream.ErrMsgIteratorClosed
	case msg := <-i.messages:
		return msg, nil
	}
}
func (i *runtimeIterator) Stop()  { i.once.Do(func() { close(i.stopped) }) }
func (i *runtimeIterator) Drain() { i.Stop() }

func testRuntime(t *testing.T) (*Runtime, *MockJetStreamPublisher, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	cfg := &config.Config{}
	cfg.Archiver.Streams.BatchSize = 3
	cfg.Archiver.Streams.BlockTimeout = time.Hour
	cfg.Archiver.Streams.StepStateFlushInterval = time.Hour
	cfg.NATS.ArchiverMaxDeliver = -1
	cfg.NATS.ArchiverAckWaitSeconds = 30
	js := NewMockJetStreamPublisher(ctrl)
	r, err := NewRuntime(js, NewMockDBExecer(ctrl), nil, cfg, zap.NewNop())
	require.NoError(t, err)
	return r, js, ctrl
}
func prepareRuntime(t *testing.T, r *Runtime, js *MockJetStreamPublisher, ctrl *gomock.Controller) []*runtimeIterator {
	t.Helper()
	iters := make([]*runtimeIterator, 0, len(r.streams))
	for _, s := range r.streams {
		consumer := NewMockConsumer(ctrl)
		iter := newRuntimeIterator()
		js.EXPECT().CreateOrUpdateConsumer(gomock.Any(), s.name, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
			require.Equal(t, s.durable, cfg.Durable)
			require.Equal(t, s.subject, cfg.FilterSubject)
			require.Equal(t, jetstream.AckExplicitPolicy, cfg.AckPolicy)
			return consumer, nil
		})
		consumer.EXPECT().Messages(gomock.Any()).Return(iter, nil)
		iters = append(iters, iter)
	}
	return iters
}
func awaitRuntimeEvent(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for runtime event")
	}
}

func TestRuntimeStartRollsBackAllIteratorsWhenLastConsumerFails(t *testing.T) {
	for _, failure := range []string{"consumer", "iterator"} {
		t.Run(failure, func(t *testing.T) {
			r, js, ctrl := testRuntime(t)
			var opened []*runtimeIterator
			for index, s := range r.streams {
				if index == len(r.streams)-1 && failure == "consumer" {
					js.EXPECT().CreateOrUpdateConsumer(gomock.Any(), s.name, gomock.Any()).Return(nil, errors.New("unavailable"))
					break
				}
				consumer := NewMockConsumer(ctrl)
				js.EXPECT().CreateOrUpdateConsumer(gomock.Any(), s.name, gomock.Any()).Return(consumer, nil)
				if index == len(r.streams)-1 {
					consumer.EXPECT().Messages(gomock.Any()).Return(nil, errors.New("iterator unavailable"))
				} else {
					iter := newRuntimeIterator()
					opened = append(opened, iter)
					consumer.EXPECT().Messages(gomock.Any()).Return(iter, nil)
				}
			}
			require.Error(t, r.Start(context.Background()))
			require.False(t, r.IsHealthy())
			for _, iter := range opened {
				awaitRuntimeEvent(t, iter.stopped)
				require.Empty(t, iter.nextCalled, "workers must not run before all iterators initialize")
			}
			require.NoError(t, r.Close(context.Background()))
		})
	}
}

func TestRuntimeCloseFlushesWithLiveContextAfterApplicationCancellation(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	iters := prepareRuntime(t, r, js, ctrl)
	processed := make(chan struct{}, 1)
	r.streams[0].processor = func(ctx context.Context, msgs []jetstream.Msg) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := ctx.Deadline(); !ok {
			return errors.New("shutdown writes must have a deadline")
		}
		require.Len(t, msgs, 1)
		processed <- struct{}{}
		return nil
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	require.NoError(t, r.Start(requestCtx))
	require.True(t, r.IsHealthy())
	awaitRuntimeEvent(t, iters[0].nextCalled)
	msg := NewMockMsg(ctrl)
	msg.EXPECT().Ack().Return(nil)
	iters[0].messages <- msg
	awaitRuntimeEvent(t, iters[0].nextCalled)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	require.NoError(t, r.Close(shutdownCtx))
	awaitRuntimeEvent(t, processed)
	require.False(t, r.IsHealthy())
	require.NoError(t, r.Close(context.Background()))
	for _, iter := range iters {
		awaitRuntimeEvent(t, iter.stopped)
	}
}

func TestRuntimeFailedBatchIsRetriedAndAckedOnlyAfterSuccess(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	r.batchSize = 1
	iters := prepareRuntime(t, r, js, ctrl)
	attempt := 0
	r.streams[0].processor = func(context.Context, []jetstream.Msg) error {
		attempt++
		if attempt == 1 {
			return ErrThreadNotFound
		}
		return nil
	}
	nacked, acked := make(chan struct{}), make(chan struct{})
	msg := NewMockMsg(ctrl)
	gomock.InOrder(
		msg.EXPECT().NakWithDelay(5*time.Second).DoAndReturn(func(time.Duration) error { close(nacked); return nil }),
		msg.EXPECT().Ack().DoAndReturn(func() error { close(acked); return nil }),
	)
	require.NoError(t, r.Start(context.Background()))
	iters[0].messages <- msg
	awaitRuntimeEvent(t, nacked)
	iters[0].messages <- msg
	awaitRuntimeEvent(t, acked)
	require.NoError(t, r.Close(context.Background()))
	require.Equal(t, 2, attempt)
}

func TestRuntimeCloseDeadlineCancelsInFlightDatabaseWrite(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	r.batchSize = 1
	iters := prepareRuntime(t, r, js, ctrl)
	writing := make(chan struct{})
	r.streams[0].processor = func(ctx context.Context, _ []jetstream.Msg) error {
		close(writing)
		<-ctx.Done()
		return ctx.Err()
	}
	msg := NewMockMsg(ctrl)
	msg.EXPECT().Nak().Return(nil)
	require.NoError(t, r.Start(context.Background()))
	iters[0].messages <- msg
	awaitRuntimeEvent(t, writing)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, r.Close(shutdownCtx), context.DeadlineExceeded)
	awaitRuntimeEvent(t, r.done)
}

func TestRuntimeShutdownKeepsBatchesBounded(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	iters := prepareRuntime(t, r, js, ctrl)
	writing, release := make(chan struct{}), make(chan struct{})
	sizes := make(chan int, 10)
	var first sync.Once
	r.streams[0].processor = func(ctx context.Context, msgs []jetstream.Msg) error {
		first.Do(func() { close(writing); <-release })
		sizes <- len(msgs)
		return ctx.Err()
	}
	require.NoError(t, r.Start(context.Background()))
	awaitRuntimeEvent(t, iters[0].nextCalled)
	// Fill a processing batch and then the entire pending queue.
	for n := 0; n < 6; n++ {
		msg := NewMockMsg(ctrl)
		msg.EXPECT().Ack().Return(nil)
		iters[0].messages <- msg
		awaitRuntimeEvent(t, iters[0].nextCalled)
	}
	awaitRuntimeEvent(t, writing)
	closed := make(chan error, 1)
	go func() { closed <- r.Close(context.Background()) }()
	awaitRuntimeEvent(t, iters[0].stopped)
	close(release)
	require.NoError(t, <-closed)
	close(sizes)
	count := 0
	for size := range sizes {
		require.LessOrEqual(t, size, 3)
		count += size
	}
	require.Equal(t, 6, count)
}
