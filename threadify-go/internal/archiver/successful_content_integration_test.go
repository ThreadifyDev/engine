package archiver

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/repository/valkey"
	"go.uber.org/zap"
)

// Real Lua + PostgreSQL prove success ordering, retry behavior and fallback agree.
func TestSuccessfulContentLifecycle(t *testing.T) {
	dsn, addr := os.Getenv("THREADIFY_TEST_DATABASE_URL"), os.Getenv("THREADIFY_TEST_VALKEY_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("requires disposable PostgreSQL and Valkey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "reference_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE TABLE thread_successful_contexts(thread_id text,step_name text,step_id text,validation_order bigint,context jsonb,PRIMARY KEY(thread_id,step_name));`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `CREATE TABLE thread_step_states (
 id text, thread_id text, step_name text, idempotency_key text, status text,
 retry_count integer, first_seen_at timestamptz, last_updated_at timestamptz,
 started_at timestamptz, finished_at timestamptz, previous_step text, actor text,
 actor_service text, latest_context jsonb, created_at timestamptz,
 UNIQUE(thread_id, step_name, idempotency_key));`)
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	adapter := &database.ValkeyService{Client: client}
	state := valkey.NewStepStateRepository(adapter, 60, zap.NewNop())
	require.NoError(t, state.LoadScripts(ctx))
	reader := valkey.NewSuccessfulContentRepository(adapter, pool)
	thread := "reference-" + uuid.NewString()
	meta := "thread:" + thread + ":meta"
	hot := "thread:" + thread + ":successful_contexts"
	defer func() {
		keys, _ := client.Keys(context.Background(), "thread:"+thread+":*").Result()
		if len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
	}()
	require.NoError(t, client.HSet(ctx, meta, "status", "active").Err())
	record := func(key, status, raw string, violated bool) StreamEvent {
		t.Helper()
		params := domain.ValidateStepParams{ThreadID: thread, StepName: "shipment", StepID: uuid.NewString(), IdempotencyKey: key, Status: status, Timestamp: "2000-01-01T00:00:00Z", RawContext: raw}
		if violated {
			params.RequiredSteps = []string{"missing"}
		}
		result, err := state.ValidateAndUpdateStepState(ctx, params)
		require.NoError(t, err)
		event := StreamEvent{Data: map[string]string{"threadId": thread, "stepName": "shipment", "stepId": params.StepID, "status": result.Status, "idempotencyKey": key}}
		if result.SuccessOrder > 0 {
			event.Data["successOrder"] = strconv.FormatInt(result.SuccessOrder, 10)
			event.Data["successContext"] = raw
		}
		return event
	}
	first := record("one", "success", `{"tracking":"OLD","raw":"1.00"}`, false)
	got, err := reader.GetSuccessfulContent(ctx, thread, "shipment")
	require.NoError(t, err)
	require.Equal(t, "OLD", got["tracking"])
	second := record("two", "success", `{"tracking":"NEW","raw":"1.00"}`, false)
	require.Greater(t, second.Data["successOrder"], first.Data["successOrder"])
	failed := record("two", "failed", `{"tracking":"FAILED"}`, false)
	violated := record("three", "success", `{"tracking":"VIOLATED"}`, true)
	require.Empty(t, failed.Data["successOrder"])
	require.Empty(t, violated.Data["successOrder"])
	got, err = reader.GetSuccessfulContent(ctx, thread, "shipment")
	require.NoError(t, err)
	require.Equal(t, "NEW", got["tracking"])
	_, err = reader.GetSuccessfulContent(ctx, thread+"-other", "shipment")
	require.Error(t, err)
	writer := NewPostgresWriter(pool, zap.NewNop())
	// A later failure in the same batch must not erase an earlier successful attempt.
	consumer, err := NewStepStateConsumer(nil, pool, 10, time.Second, "references-test", zap.NewNop())
	require.NoError(t, err)
	// Decode the actual wire fields and exercise the runtime consumer, including
	// success followed by failure for the same idempotency key in one batch.
	var decodedEvents []stepStateEvent
	for _, event := range []StreamEvent{first, second, failed, violated} {
		wire, err := json.Marshal(event.Data)
		require.NoError(t, err)
		var decoded stepStateEvent
		require.NoError(t, json.Unmarshal(wire, &decoded))
		decodedEvents = append(decodedEvents, decoded)
	}
	require.NoError(t, consumer.writeBatch(ctx, decodedEvents))
	var mutableStatus string
	require.NoError(t, pool.QueryRow(ctx, "SELECT status FROM thread_step_states WHERE thread_id=$1 AND idempotency_key='two'", thread).Scan(&mutableStatus))
	require.Equal(t, "failed", mutableStatus)
	require.NoError(t, writer.writeSuccessfulContexts(ctx, []StreamEvent{first})) // delayed replay
	require.NoError(t, client.Del(ctx, hot).Err())
	got, err = reader.GetSuccessfulContent(ctx, thread, "shipment")
	require.NoError(t, err)
	require.Equal(t, "NEW", got["tracking"])
	require.Equal(t, "1.00", got["raw"])
	// Missing content on the newest success must not retrieve a field from an older one.
	missing := record("four", "success", `{}`, false)
	require.NoError(t, writer.writeSuccessfulContexts(ctx, []StreamEvent{missing}))
	require.NoError(t, client.Del(ctx, hot).Err())
	got, err = reader.GetSuccessfulContent(ctx, thread, "shipment")
	require.NoError(t, err)
	require.NotContains(t, got, "tracking")
	// A terminal-thread rejection cannot publish a new successful snapshot.
	require.NoError(t, client.HSet(ctx, meta, "status", "completed").Err())
	rejected := record("five", "success", `{"tracking":"AFTER_END"}`, false)
	require.Empty(t, rejected.Data["successOrder"])
	// Database JSON stores strings without interpreting numbers or embedded JSON.
	var stored string
	require.NoError(t, pool.QueryRow(ctx, "SELECT context::text FROM thread_successful_contexts WHERE thread_id=$1", thread).Scan(&stored))
	var decoded map[string]string
	require.NoError(t, json.Unmarshal([]byte(stored), &decoded))
	require.Empty(t, decoded)
}
