package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	enginemocks "github.com/threadify/engine/internal/service/mocks/engine"
	"go.uber.org/zap"
)

func TestEndThreadDoesNotAcknowledgeFailedStatusWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := enginemocks.NewMockThreadRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), "thread-1").Return(&domain.Thread{ID: "thread-1", Status: domain.ThreadStatusActive}, nil)
	repo.EXPECT().UpdateThreadStatus(gomock.Any(), "thread-1", ThreadStatusCompleted, gomock.Any()).Return(errors.New("cache unavailable"))
	svc := &ThreadService{repo: repo, logger: zap.NewNop()}

	err := svc.EndThread(context.Background(), "thread-1", "actor", "sdk", ThreadStatusCompleted, "done", time.Now())
	require.ErrorContains(t, err, "cache unavailable")
}

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
