package valkey

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

func TestWaitInvocationLifecycle(t *testing.T) {
	addr := os.Getenv("THREADIFY_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("requires disposable Valkey")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()
	thread := uuid.NewString()
	p := waitPrefix(thread)
	defer func() {
		keys, _ := client.Keys(ctx, p+"*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
	}()
	require.NoError(t, client.HSet(ctx, p+"meta", "status", "active").Err())
	repo := NewWaitRepository(client)
	state := NewStepStateRepository(&database.ValkeyService{Client: client}, 604800, zap.NewNop())
	require.NoError(t, state.LoadScripts(ctx))
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{
		"approval": {ID: "approval"}, "charge": {ID: "charge", DependsOn: []string{"approval"}, FreshDependsOn: []string{"approval"}},
	}, EntryPoints: []string{"approval"}}}
	claim := func(id string) *domain.WaitResult {
		r, err := repo.Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "owner", graph)
		require.NoError(t, err)
		return r
	}
	record := func(step, status, inv string) *domain.StepStateResult {
		id := uuid.NewString()
		key := uuid.NewString()
		req := &domain.RecordEventCmd{ThreadID: thread, StepName: step, Status: status, InvocationID: inv}
		require.NoError(t, repo.Begin(ctx, req, id, "owner"))
		node := graph.Graph.Nodes[step]
		result, err := state.ValidateAndUpdateStepState(ctx, domain.ValidateStepParams{ThreadID: thread, StepID: id, StepName: step, IdempotencyKey: key, Status: status, Timestamp: time.Now().Format(time.RFC3339Nano), Actor: "owner", RequiredSteps: node.DependsOn, FreshRequiredSteps: node.FreshDependsOn, InvocationID: inv, RawContext: `{}`})
		require.NoError(t, err)
		decision := "passed"
		if len(result.Violations) > 0 {
			decision = "violated"
		}
		require.NoError(t, repo.Complete(ctx, domain.WaitResult{ThreadID: thread, StepName: step, StepID: id, InvocationID: inv, Decision: decision}, true))
		journal, err := repo.Result(ctx, thread, id)
		require.NoError(t, err)
		require.Equal(t, decision, journal.Decision)
		_, err = repo.Result(ctx, uuid.NewString(), id)
		require.Error(t, err)
		return result
	}
	// Go-side duration violations must not become successful prerequisites in Lua.
	rejected, err := state.ValidateAndUpdateStepState(ctx, domain.ValidateStepParams{ThreadID: thread, StepID: uuid.NewString(), StepName: "approval", IdempotencyKey: uuid.NewString(), Status: "success", Timestamp: time.Now().Format(time.RFC3339Nano), RawContext: `{}`, ExistingViolations: []domain.Violation{{Type: "max_duration_exceeded", Severity: "critical", Message: "Thread deadline passed"}}})
	require.NoError(t, err)
	require.True(t, rejected.HasCriticalViolation)
	require.Zero(t, rejected.SuccessOrder)
	require.Equal(t, "max_duration_exceeded", rejected.Violations[0].Type)
	require.Equal(t, "pending", claim(uuid.NewString()).Decision)
	require.False(t, record("approval", "success", "").HasCriticalViolation)
	// All concurrent callers see one atomic claim, never two approvals for one occurrence.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var granted []string
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := uuid.NewString()
			r, err := repo.Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "owner", graph)
			mu.Lock()
			defer mu.Unlock()
			require.NoError(t, err)
			if r.Decision == "allowed" {
				granted = append(granted, id)
			}
		}()
	}
	wg.Wait()
	require.Len(t, granted, 1)
	id := granted[0]
	require.Equal(t, "allowed", claim(id).Decision) // idempotent retry
	wrong, err := repo.Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "other-owner", graph)
	require.NoError(t, err)
	require.Equal(t, "denied", wrong.Decision)
	require.Error(t, repo.Begin(ctx, &domain.RecordEventCmd{ThreadID: thread, StepName: "charge", InvocationID: id}, uuid.NewString(), "other-owner"))
	require.False(t, record("charge", "failed", id).HasCriticalViolation)
	require.Equal(t, "denied", claim(id).Decision)
	require.Equal(t, "pending", claim(uuid.NewString()).Decision)         // failure still consumes the approval
	require.True(t, record("charge", "success", "").HasCriticalViolation) // observation detects bypass
	require.False(t, record("approval", "success", "").HasCriticalViolation)
	id = uuid.NewString()
	require.Equal(t, "allowed", claim(id).Decision)
	cancelled, err := repo.Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id, Cancel: true}, "owner", graph)
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Decision)
	require.Equal(t, "pending", claim(uuid.NewString()).Decision) // cancellation never restores consumed approval
	require.False(t, record("approval", "success", "").HasCriticalViolation)
	pendingID := uuid.NewString()
	require.NoError(t, repo.Begin(ctx, &domain.RecordEventCmd{ThreadID: thread, StepName: "approval"}, pendingID, "owner"))
	require.Equal(t, "pending", claim(uuid.NewString()).Decision)
	require.NoError(t, repo.Complete(ctx, domain.WaitResult{ThreadID: thread, StepName: "approval", StepID: pendingID, Decision: "unavailable"}, false))
	require.Equal(t, "unavailable", claim(uuid.NewString()).Decision) // unavailable is not an approval
	require.NoError(t, client.HDel(ctx, p+"pending_validation", pendingID).Err())
	id = uuid.NewString()
	require.Equal(t, "allowed", claim(id).Decision)
	require.NoError(t, client.HSet(ctx, p+"meta", "status", "completed").Err())
	require.Equal(t, "denied", claim(id).Decision)
	require.NoError(t, client.Del(ctx, p+"meta").Err())
	require.Equal(t, "unavailable", claim(uuid.NewString()).Decision)
}
