package valkey

import (
	"context"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
	"os"
	"testing"
	"time"
)

func TestWaitWatchPublishesDurableResultAndTerminalChanges(t *testing.T) {
	addr := os.Getenv("THREADIFY_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("requires disposable Valkey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	writerClient := redis.NewClient(&redis.Options{Addr: addr})
	defer writerClient.Close()
	reader, writer := NewWaitRepository(client), NewWaitRepository(writerClient)
	thread, id := uuid.NewString(), uuid.NewString()
	p := waitPrefix(thread)
	defer func() {
		keys, _ := client.Keys(context.Background(), p+"*").Result()
		if len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
	}()
	req := &domain.RecordEventCmd{ThreadID: thread, StepName: "approval"}
	require.NoError(t, writer.Begin(ctx, req, id, "owner"))
	changes, closeWatch, err := reader.Watch(ctx, thread)
	require.NoError(t, err)
	defer closeWatch()
	require.NoError(t, writer.Complete(ctx, domain.WaitResult{ThreadID: thread, StepName: "approval", StepID: id, Decision: "passed"}, true))
	select {
	case <-changes:
	case <-ctx.Done():
		t.Fatal("completion did not wake subscriber")
	}
	result, err := reader.Result(ctx, thread, id)
	require.NoError(t, err)
	require.Equal(t, "passed", result.Decision)
	// A completion that precedes subscription remains visible on the mandatory
	// reread. A different Redis client simulates a separate Engine process.
	late, closeLate, err := reader.Watch(ctx, thread)
	require.NoError(t, err)
	defer closeLate()
	result, err = reader.Result(ctx, thread, id)
	require.NoError(t, err)
	require.Equal(t, "passed", result.Decision)
	require.NoError(t, client.HSet(ctx, p+"meta", "status", "active").Err())
	require.NoError(t, writerClient.Eval(ctx, checkAndUpdateThreadStatusScript, []string{p + "meta", "thread:" + thread}, "cancelled", time.Now().Format(time.RFC3339Nano), 600).Err())
	select {
	case <-late:
	case <-ctx.Done():
		t.Fatal("terminal state did not wake subscriber")
	}
	cancel()
	closeLate()
	closeWatch()
	require.Eventually(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), p+"wait_changes").Result()
		return err == nil && counts[p+"wait_changes"] == 0
	}, time.Second, 5*time.Millisecond)
}
