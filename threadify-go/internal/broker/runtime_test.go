package broker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func embeddedOptions(dir string) Options {
	return Options{Mode: "embedded", StoreDir: dir, MaxMemoryBytes: 16 << 20, MaxStoreBytes: 128 << 20}
}

func TestRuntimeRestartPreservesUnacknowledgedEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opts := embeddedOptions(t.TempDir())
	r, err := Start(ctx, opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close(context.Background())) })
	require.True(t, r.IsHealthy())
	require.Nil(t, r.server.Addr(), "embedded broker must not expose a TCP listener")
	nc, err := nats.Connect(nats.DefaultURL, r.ClientOptions()...)
	require.NoError(t, err)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: "durable", Subjects: []string{"events"}, Storage: jetstream.FileStorage, Retention: jetstream.WorkQueuePolicy, MaxBytes: 1 << 20, Discard: jetstream.DiscardNew})
	require.NoError(t, err)
	_, err = js.CreateOrUpdateConsumer(ctx, "durable", jetstream.ConsumerConfig{Durable: "writer", AckPolicy: jetstream.AckExplicitPolicy})
	require.NoError(t, err)
	_, err = js.Publish(ctx, "events", []byte("pending database write"))
	require.NoError(t, err)
	nc.Close()
	require.NoError(t, r.Close(ctx))
	require.False(t, r.IsHealthy())
	require.NoError(t, r.Close(ctx), "shutdown is idempotent")

	restarted, err := Start(ctx, opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restarted.Close(context.Background())) })
	nc, err = nats.Connect(nats.DefaultURL, restarted.ClientOptions()...)
	require.NoError(t, err)
	defer nc.Close()
	js, err = jetstream.New(nc)
	require.NoError(t, err)
	consumer, err := js.Consumer(ctx, "durable", "writer")
	require.NoError(t, err)
	msg, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
	require.NoError(t, err)
	require.Equal(t, "pending database write", string(msg.Data()))
	require.NoError(t, msg.DoubleAck(ctx))
	stream, err := js.Stream(ctx, "durable")
	require.NoError(t, err)
	require.Zero(t, stream.CachedInfo().State.Msgs, "acknowledgement must reclaim backlog capacity")
}

func TestRuntimeValidatesPersistentStorage(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("not a directory"), 0600))
	for name, opts := range map[string]Options{
		"unknown mode":      {Mode: "unknown"},
		"missing directory": embeddedOptions(""),
		"file as directory": embeddedOptions(file),
		"unbounded limits":  {Mode: "embedded", StoreDir: t.TempDir()},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Start(context.Background(), opts)
			require.Error(t, err)
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Start(ctx, embeddedOptions(t.TempDir()))
	require.ErrorIs(t, err, context.Canceled)
}

func TestExternalRuntimeDoesNotStartServer(t *testing.T) {
	r, err := Start(context.Background(), Options{Mode: "external"})
	require.NoError(t, err)
	require.Nil(t, r.server)
	require.Empty(t, r.ClientOptions())
	require.True(t, r.IsHealthy())
	require.NoError(t, r.Close(context.Background()))
	require.False(t, r.IsHealthy())
}

func TestRuntimeLocksStoreUntilShutdown(t *testing.T) {
	opts := embeddedOptions(t.TempDir())
	r, err := Start(context.Background(), opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close(context.Background())) })
	_, err = Start(context.Background(), opts)
	require.ErrorContains(t, err, "another Threadify process")
	require.NoError(t, r.Close(context.Background()))
	restarted, err := Start(context.Background(), opts)
	require.NoError(t, err, "shutdown must release the storage lock")
	require.NoError(t, restarted.Close(context.Background()))
}
