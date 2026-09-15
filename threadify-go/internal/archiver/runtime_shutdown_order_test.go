package archiver

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// The live failure was a reference batch flushed concurrently with the metadata
// which creates its thread. Received metadata must finish before reference flush.
func TestRuntimeShutdownFlushesMetadataBeforeReferences(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	iters := prepareRuntime(t, r, js, ctrl)
	metadataStarted, releaseMetadata := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseMetadata) }) }
	defer release()
	var metadataWritten atomic.Bool
	r.streams[1].processor = func(ctx context.Context, _ []jetstream.Msg) error {
		close(metadataStarted)
		select {
		case <-releaseMetadata:
			metadataWritten.Store(true)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.streams[0].processor = func(context.Context, []jetstream.Msg) error {
		if !metadataWritten.Load() {
			return ErrThreadNotFound
		}
		return nil
	}
	require.NoError(t, r.Start(context.Background()))
	for _, index := range []int{0, 1} {
		awaitRuntimeEvent(t, iters[index].nextCalled)
		msg := NewMockMsg(ctrl)
		msg.EXPECT().Ack().Return(nil)
		iters[index].messages <- msg
		awaitRuntimeEvent(t, iters[index].nextCalled)
	}
	closed := make(chan error, 1)
	go func() { closed <- r.Close(context.Background()) }()
	awaitRuntimeEvent(t, metadataStarted)
	release()
	require.NoError(t, <-closed)
}

// A reference write already running when Close begins must be retried once
// after metadata completes; this covers the exact observed shutdown race.
func TestRuntimeShutdownRetriesInFlightMissingMetadataAfterDependencyCompletes(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	r.batchSize = 1
	iters := prepareRuntime(t, r, js, ctrl)
	metadataStarted, releaseMetadata := make(chan struct{}), make(chan struct{})
	referenceStarted, releaseReference := make(chan struct{}), make(chan struct{})
	var releaseMetaOnce, releaseRefOnce sync.Once
	releaseMeta := func() { releaseMetaOnce.Do(func() { close(releaseMetadata) }) }
	releaseRef := func() { releaseRefOnce.Do(func() { close(releaseReference) }) }
	defer releaseMeta()
	defer releaseRef()
	var metadataWritten atomic.Bool
	var attempts atomic.Int64
	r.streams[1].processor = func(ctx context.Context, _ []jetstream.Msg) error {
		close(metadataStarted)
		select {
		case <-releaseMetadata:
			metadataWritten.Store(true)
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r.streams[0].processor = func(ctx context.Context, _ []jetstream.Msg) error {
		if attempts.Add(1) == 1 {
			close(referenceStarted)
			select {
			case <-releaseReference:
				return ErrThreadNotFound
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if !metadataWritten.Load() {
			return ErrThreadNotFound
		}
		return nil
	}
	require.NoError(t, r.Start(context.Background()))
	metadata := NewMockMsg(ctrl)
	metadata.EXPECT().Ack().Return(nil)
	iters[1].messages <- metadata
	awaitRuntimeEvent(t, metadataStarted)
	reference := NewMockMsg(ctrl)
	reference.EXPECT().Ack().Return(nil)
	iters[0].messages <- reference
	awaitRuntimeEvent(t, referenceStarted)
	closed := make(chan error, 1)
	go func() { closed <- r.Close(context.Background()) }()
	awaitRuntimeEvent(t, iters[0].stopped)
	releaseRef()
	releaseMeta()
	require.NoError(t, <-closed)
	require.Equal(t, int64(2), attempts.Load())
}

func TestRuntimeShutdownMetadataDependencyWaitHonorsDeadline(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	iters := prepareRuntime(t, r, js, ctrl)
	var referenceCalls atomic.Int64
	r.streams[1].processor = func(ctx context.Context, _ []jetstream.Msg) error { <-ctx.Done(); return ctx.Err() }
	r.streams[0].processor = func(context.Context, []jetstream.Msg) error { referenceCalls.Add(1); return nil }
	require.NoError(t, r.Start(context.Background()))
	for _, index := range []int{0, 1} {
		awaitRuntimeEvent(t, iters[index].nextCalled)
		msg := NewMockMsg(ctrl)
		msg.EXPECT().Nak().Return(nil)
		iters[index].messages <- msg
		awaitRuntimeEvent(t, iters[index].nextCalled)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, r.Close(ctx), context.DeadlineExceeded)
	awaitRuntimeEvent(t, r.done)
	require.Zero(t, referenceCalls.Load())
}

func TestRuntimeShutdownStillReportsRealReferenceWriteFailure(t *testing.T) {
	r, js, ctrl := testRuntime(t)
	iters := prepareRuntime(t, r, js, ctrl)
	failure := errors.New("database write unavailable")
	r.streams[0].processor = func(context.Context, []jetstream.Msg) error { return failure }
	require.NoError(t, r.Start(context.Background()))
	awaitRuntimeEvent(t, iters[0].nextCalled)
	msg := NewMockMsg(ctrl)
	msg.EXPECT().Nak().Return(nil)
	iters[0].messages <- msg
	awaitRuntimeEvent(t, iters[0].nextCalled)
	require.ErrorIs(t, r.Close(context.Background()), failure)
}
