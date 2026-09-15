package service

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/broker"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type shutdownPublisher func(context.Context, domain.ValidationNotification) error

func (f shutdownPublisher) PublishNotification(ctx context.Context, n domain.ValidationNotification) error {
	return f(ctx, n)
}

type shutdownFetcher func(string, string, time.Duration) (*domain.NATSMessage, error)

func (f shutdownFetcher) FetchMessage(subject, consumer string, timeout time.Duration) (*domain.NATSMessage, error) {
	return f(subject, consumer, timeout)
}

type shutdownHandler func(domain.ValidationNotification) error

func (f shutdownHandler) HandleNotification(n domain.ValidationNotification) error { return f(n) }

func TestTimeoutMonitorStopContextJoinsCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := broker.Start(ctx, broker.Options{Mode: "embedded", StoreDir: t.TempDir(), MaxMemoryBytes: 64 << 20, MaxStoreBytes: 1 << 30})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close(context.Background())) })
	nc, err := nats.Connect(nats.DefaultURL, r.ClientOptions()...)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	tm, err := NewTimeoutMonitor(nc, shutdownPublisher(func(context.Context, domain.ValidationNotification) error {
		close(entered)
		<-release
		return nil
	}), zap.NewNop())
	require.NoError(t, err)
	runResult := make(chan error, 1)
	go func() { runResult <- tm.Start() }()
	payload, err := json.Marshal(timeoutEventModel{ID: "pending", ThreadID: "thread", Type: string(domain.TimeoutTypeMaxDuration), DeadlineAt: time.Now().Add(-time.Second)})
	require.NoError(t, err)
	_, err = tm.js.Publish(ctx, "timeout.max_duration.thread", payload)
	require.NoError(t, err)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("timeout callback did not start")
	}
	deadline, deadlineCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer deadlineCancel()
	require.ErrorIs(t, tm.StopContext(deadline), context.DeadlineExceeded)
	require.ErrorContains(t, tm.ScheduleTimeout(ctx, domain.TimeoutEvent{}), "stopped")
	require.ErrorContains(t, tm.CancelTimeout(ctx, "pending", "thread", "shutdown"), "stopped")
	require.Error(t, tm.Start(), "a stopped monitor must not restart")
	close(release)
	require.NoError(t, tm.StopContext(ctx))
	require.NoError(t, <-runResult)
	require.NoError(t, tm.StopContext(ctx), "shutdown must be idempotent")
}

func TestNotificationConsumerStopContextJoinsHandler(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	consumer := NewNotificationConsumer(shutdownFetcher(func(string, string, time.Duration) (*domain.NATSMessage, error) {
		return &domain.NATSMessage{Data: []byte(`{"notification_id":"pending"}`)}, nil
	}), nil, zap.NewNop())
	require.NoError(t, consumer.Subscribe("thread", "user", "owner", shutdownHandler(func(domain.ValidationNotification) error {
		close(entered)
		<-release
		return nil
	})))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("notification handler did not start")
	}
	deadline, deadlineCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer deadlineCancel()
	require.ErrorIs(t, consumer.StopContext(deadline), context.DeadlineExceeded)
	require.ErrorContains(t, consumer.Subscribe("another", "user", "owner", nil), "stopped")
	close(release)
	require.NoError(t, consumer.StopContext(ctx))
	require.Zero(t, consumer.GetActiveSubscriptions())
}

func TestNotificationConsumerDoesNotDispatchAfterCanceledFetch(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var handled atomic.Int32
	consumer := NewNotificationConsumer(shutdownFetcher(func(string, string, time.Duration) (*domain.NATSMessage, error) {
		close(entered)
		<-release
		return &domain.NATSMessage{Data: []byte(`{}`)}, nil
	}), nil, zap.NewNop())
	require.NoError(t, consumer.Subscribe("thread", "user", "owner", shutdownHandler(func(domain.ValidationNotification) error {
		handled.Add(1)
		return nil
	})))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("notification fetch did not start")
	}
	deadline, deadlineCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer deadlineCancel()
	require.ErrorIs(t, consumer.StopContext(deadline), context.DeadlineExceeded)
	close(release)
	require.NoError(t, consumer.StopContext(ctx))
	require.Zero(t, handled.Load())
}
