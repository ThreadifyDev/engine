package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type lifecycleStopper struct{ calls atomic.Int32 }

func (s *lifecycleStopper) Stop() { s.calls.Add(1) }

type lifecycleContextStopper struct{ lifecycleStopper }

func (s *lifecycleContextStopper) StopContext(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestThreadServiceShutdownUsesCallerDeadline(t *testing.T) {
	monitor := &lifecycleContextStopper{}
	svc := &ThreadService{timeoutMonitor: monitor}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, svc.StopContext(ctx), context.DeadlineExceeded)
	require.Zero(t, monitor.calls.Load(), "context-aware stop should replace the legacy stop")
}

func TestThreadServiceShutdownWaitsForAcceptedPublications(t *testing.T) {
	monitor := &lifecycleStopper{}
	consumer := NewNotificationConsumer(nil, nil, zap.NewNop())
	svc := &ThreadService{timeoutMonitor: monitor, notificationConsumer: consumer}
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	require.True(t, svc.runBackground(func() {
		close(started)
		<-release
		close(finished)
	}))
	<-started

	svc.Stop()
	svc.Stop()
	require.EqualValues(t, 1, monitor.calls.Load())
	require.ErrorIs(t, consumer.ctx.Err(), context.Canceled)
	require.False(t, svc.runBackground(func() { t.Error("work started after shutdown") }))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, svc.WaitBackground(ctx), context.DeadlineExceeded)
	close(release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, svc.WaitBackground(ctx))
	select {
	case <-finished:
	default:
		t.Fatal("shutdown finished before publication completed")
	}
}

func TestThreadServiceConcurrentSchedulingAndShutdown(t *testing.T) {
	svc := &ThreadService{}
	var callers sync.WaitGroup
	var accepted, finished atomic.Int32
	start := make(chan struct{})
	for range 100 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			<-start
			if svc.runBackground(func() { finished.Add(1) }) {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	svc.Stop()
	callers.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, svc.WaitBackground(ctx))
	require.Equal(t, accepted.Load(), finished.Load())
}
