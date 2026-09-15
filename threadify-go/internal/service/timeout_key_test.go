package service

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/broker"
)

func TestTimeoutCancellationKey_BranchedTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := broker.Start(ctx, broker.Options{Mode: "embedded", StoreDir: t.TempDir(), MaxMemoryBytes: 16 << 20, MaxStoreBytes: 64 << 20})
	require.NoError(t, err)
	defer runtime.Close(context.Background())
	nc, err := nats.Connect(nats.DefaultURL, runtime.ClientOptions()...)
	require.NoError(t, err)
	defer nc.Close()
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "cancellations", Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	ids := []string{"thread:received:stock_reserved,label_printed:transition", "thread:a_b:c:transition", "thread:a:b_c:transition", "thread:received:customer order/ready:transition"}
	seen := map[string]bool{}
	for _, id := range ids {
		key := TimeoutCancellationKey(id)
		require.False(t, seen[key], "distinct timeout IDs must not collide")
		seen[key] = true
		_, err := kv.Put(ctx, key, []byte(id))
		require.NoError(t, err)
		entry, err := kv.Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, id, string(entry.Value()))
	}
}
