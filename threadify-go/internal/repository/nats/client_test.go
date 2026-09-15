package nats

import (
	"context"
	"strings"
	"testing"
	"time"

	gonats "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/broker"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

func testBroker(t *testing.T) *broker.Runtime {
	t.Helper()
	r, err := broker.Start(context.Background(), broker.Options{Mode: "embedded", StoreDir: t.TempDir(), MaxMemoryBytes: 64 << 20, MaxStoreBytes: 8 << 30})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, r.Close(ctx))
	})
	return r
}

func testClient(t *testing.T, r *broker.Runtime, cfg *config.NATSConfig) *Client {
	t.Helper()
	c, err := NewClient(cfg, zap.NewNop(), r.ClientOptions()...)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	return c
}

func TestArchivalStreamsRetainBacklogAndReclaimAcknowledgedSpace(t *testing.T) {
	r := testBroker(t)
	c := testClient(t, r, &config.NATSConfig{URL: gonats.DefaultURL, StreamName: "NOTIFICATIONS", ArchivalMaxBytes: 256})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, c.InitStreams(ctx))
	for _, name := range []string{StreamActivityLog, StreamThreadMetadata, StreamThreadAccess, StreamThreadValidations, StreamThreadNotifications, StreamStepState, StreamUsageSync} {
		stream, err := c.js.Stream(ctx, name)
		require.NoError(t, err)
		cfg := stream.CachedInfo().Config
		require.Equal(t, jetstream.WorkQueuePolicy, cfg.Retention)
		require.Equal(t, jetstream.DiscardNew, cfg.Discard)
		require.Zero(t, cfg.MaxAge)
		require.EqualValues(t, 256, cfg.MaxBytes)
	}
	payload := []byte(strings.Repeat("x", 150))
	_, err := c.js.Publish(ctx, SubjectThreadNotifications, payload)
	require.NoError(t, err)
	_, err = c.js.Publish(ctx, SubjectThreadNotifications, payload)
	require.Error(t, err, "a full stream must reject new data instead of discarding pending writes")
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, StreamThreadNotifications, jetstream.ConsumerConfig{Durable: "writer", AckPolicy: jetstream.AckExplicitPolicy})
	require.NoError(t, err)
	msg, err := consumer.Next(jetstream.FetchMaxWait(time.Second))
	require.NoError(t, err)
	require.Equal(t, payload, msg.Data())
	require.NoError(t, msg.DoubleAck(ctx))
	_, err = c.js.Publish(ctx, SubjectThreadNotifications, payload)
	require.NoError(t, err, "committed events must free capacity")
}

func TestArchivalStreamMigrationDoesNotDestroyOldData(t *testing.T) {
	r := testBroker(t)
	c := testClient(t, r, &config.NATSConfig{URL: gonats.DefaultURL})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.js.CreateStream(ctx, jetstream.StreamConfig{Name: StreamActivityLog, Subjects: []string{SubjectActivityLog}, Retention: jetstream.LimitsPolicy, Storage: jetstream.FileStorage})
	require.NoError(t, err)
	_, err = c.js.Publish(ctx, SubjectActivityLog, []byte("pending"))
	require.NoError(t, err)
	err = c.initializeArchivalStreams(ctx)
	require.ErrorContains(t, err, "explicit migration")
	stream, err := c.js.Stream(ctx, StreamActivityLog)
	require.NoError(t, err)
	require.Equal(t, jetstream.LimitsPolicy, stream.CachedInfo().Config.Retention)
	require.EqualValues(t, 1, stream.CachedInfo().State.Msgs)
}

func TestPoolUsesInProcessConnectionsAndDrains(t *testing.T) {
	r := testBroker(t)
	pool, err := NewPool(&config.NATSConfig{URL: gonats.DefaultURL, StreamName: "NOTIFICATIONS"}, 2, zap.NewNop(), r.ClientOptions()...)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.True(t, pool.IsHealthy())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = pool.GetClient().JetStream().Publish(ctx, SubjectThreadMetadata, []byte("queued"))
	require.NoError(t, err)
	require.NoError(t, pool.Drain(ctx))
	require.False(t, pool.IsHealthy())
	for i := 0; i < pool.Size(); i++ {
		require.True(t, pool.GetClientByIndex(i).Conn().IsClosed())
	}
}
