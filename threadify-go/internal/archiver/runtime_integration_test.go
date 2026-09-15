package archiver

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/broker"
	"github.com/threadify/engine/internal/config"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"go.uber.org/zap"
)

type outageDB struct {
	DBExecer
	unavailable atomic.Bool
	failed      chan struct{}
	written     chan []any
}

func (db *outageDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	if !strings.Contains(query, "INSERT INTO thread_step_states") {
		return pgconn.CommandTag{}, errors.New("unexpected query")
	}
	if db.unavailable.Load() {
		select {
		case db.failed <- struct{}{}:
		default:
		}
		return pgconn.CommandTag{}, errors.New("database temporarily unavailable")
	}
	db.written <- args
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func TestRuntimeEmbeddedBrokerReplaysFailedWritesAfterRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg := &config.Config{}
	cfg.Archiver.Streams.BatchSize = 2
	cfg.Archiver.Streams.BlockTimeout = time.Hour
	cfg.Archiver.Streams.StepStateFlushInterval = time.Hour
	cfg.NATS.ArchiverMaxDeliver = -1
	cfg.NATS.ArchiverAckWaitSeconds = 1
	db := &outageDB{failed: make(chan struct{}, 8), written: make(chan []any, 8)}
	db.unavailable.Store(true)
	opts := broker.Options{Mode: "embedded", StoreDir: t.TempDir(), MaxMemoryBytes: 16 << 20, MaxStoreBytes: 128 << 20}
	var owned *broker.Runtime
	var nc *nats.Conn
	var runtime *Runtime
	var js jetstream.JetStream
	start := func() {
		var err error
		owned, err = broker.Start(ctx, opts)
		require.NoError(t, err)
		nc, err = nats.Connect(nats.DefaultURL, owned.ClientOptions()...)
		require.NoError(t, err)
		js, err = jetstream.New(nc)
		require.NoError(t, err)
		runtime, err = NewRuntime(js, db, nil, cfg, zap.NewNop())
		require.NoError(t, err)
		for _, s := range runtime.streams {
			_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{Name: s.name, Subjects: []string{s.subject}, Storage: jetstream.FileStorage, Retention: jetstream.WorkQueuePolicy, Discard: jetstream.DiscardNew, MaxBytes: 1 << 20})
			require.NoError(t, err)
		}
		require.NoError(t, runtime.Start(ctx))
	}
	stop := func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second*3)
		defer shutdownCancel()
		if runtime != nil {
			require.NoError(t, runtime.Close(shutdownCtx))
			runtime = nil
		}
		if nc != nil {
			require.NoError(t, nc.Flush())
			nc.Close()
			nc = nil
		}
		if owned != nil {
			require.NoError(t, owned.Close(shutdownCtx))
			owned = nil
		}
	}
	t.Cleanup(stop)
	start()
	for _, status := range []string{"started", "finished"} {
		_, err := js.Publish(ctx, natsrepo.SubjectStepState, []byte(`{"stepId":"step-1","threadId":"thread-1","stepName":"work","idempotencyKey":"key-1","status":"`+status+`"}`))
		require.NoError(t, err)
	}
	awaitRuntimeEvent(t, db.failed)
	require.Eventually(t, func() bool { return !runtime.IsHealthy() }, time.Second, time.Millisecond)
	stop()
	db.unavailable.Store(false)
	start()
	select {
	case args := <-db.written:
		require.Len(t, args, 14, "duplicate updates should use one database upsert")
		require.Equal(t, "finished", args[4])
	case <-ctx.Done():
		t.Fatal("failed writes were not recovered after broker restart")
	}
	require.Eventually(t, func() bool {
		stream, err := js.Stream(ctx, natsrepo.StreamStepState)
		return err == nil && stream.CachedInfo().State.Msgs == 0
	}, 3*time.Second, 10*time.Millisecond, "successful replay should acknowledge both durable events")
	require.True(t, runtime.IsHealthy())
}
