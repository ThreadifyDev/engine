package valkey

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/domain"
)

//go:embed lua/claim_step.lua
var claimStepScript string

// WaitRepository keeps decisions in the same store as atomic flow validation.
// Missing state is never treated as permission. Decisions live for seven days;
// outstanding claims and pending validation barriers have no automatic expiry.
type WaitRepository struct{ client *redis.Client }

func NewWaitRepository(client *redis.Client) *WaitRepository { return &WaitRepository{client} }
func waitPrefix(thread string) string                        { return "thread:" + thread + ":" }

func (r *WaitRepository) Begin(ctx context.Context, req *domain.RecordEventCmd, stepID, owner string) error {
	result := domain.WaitResult{Action: "waitFor", Status: "success", ThreadID: req.ThreadID, StepName: req.StepName, StepID: stepID, InvocationID: req.InvocationID, Decision: "pending"}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return r.client.Eval(ctx, `if ARGV[4]~='' then
 local raw=redis.call('GET',KEYS[3]); if not raw then return redis.error_reply('Unknown invocation') end
 local g=cjson.decode(raw)
 if g.owner~=ARGV[5] or g.step~=ARGV[6] or g.decision~='allowed' or g.bound then return redis.error_reply('Invocation is unavailable for this caller') end
 if redis.call('HGET',KEYS[4],ARGV[6])~=ARGV[4] then return redis.error_reply('Invocation is not active') end
 g.bound=ARGV[2];redis.call('SET',KEYS[3],cjson.encode(g))
 end
 redis.call('SET', KEYS[1], ARGV[1], 'EX', 604800); redis.call('HSET', KEYS[2], ARGV[2], ARGV[3]); return 1`, []string{waitPrefix(req.ThreadID) + "validation:" + stepID, waitPrefix(req.ThreadID) + "pending_validation", waitPrefix(req.ThreadID) + "grant:" + req.InvocationID, waitPrefix(req.ThreadID) + "active_invocations"}, string(raw), stepID, time.Now().UnixMilli(), req.InvocationID, owner, req.StepName).Err()
}

// Complete publishes exactly one invocation's result and clears its barrier.
// An unavailable validation retains the barrier: its event still lacks a decision.
func (r *WaitRepository) Complete(ctx context.Context, result domain.WaitResult, clearBarrier bool) error {
	result.Action = "waitFor"
	result.Status = "success"
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	script := `redis.call('SET', KEYS[1], ARGV[1], 'EX', 604800)
 if ARGV[3]=='true' then
 redis.call('HDEL', KEYS[2], ARGV[2])
 local claim=redis.call('HGET',KEYS[3],ARGV[4])
 if claim and claim==ARGV[5] then
 redis.call('HDEL',KEYS[3],ARGV[4])
 local grant=redis.call('GET',KEYS[4])
 if grant then local g=cjson.decode(grant); g.decision='consumed'; redis.call('SET',KEYS[4],cjson.encode(g),'EX',604800) end
 end
 else
 redis.call('HSET',KEYS[2],ARGV[2],'unavailable')
 end
 redis.call('PUBLISH',KEYS[5],'changed')
 return 1`
	p := waitPrefix(result.ThreadID)
	return r.client.Eval(ctx, script, []string{p + "validation:" + result.StepID, p + "pending_validation", p + "active_invocations", p + "grant:" + result.InvocationID, p + "wait_changes"}, string(raw), result.StepID, fmt.Sprint(clearBarrier), result.StepName, result.InvocationID).Err()
}
func (r *WaitRepository) Result(ctx context.Context, threadID, stepID string) (*domain.WaitResult, error) {
	raw, err := r.client.Get(ctx, waitPrefix(threadID)+"validation:"+stepID).Result()
	if err != nil {
		return nil, fmt.Errorf("validation result unavailable or expired")
	}
	var result domain.WaitResult
	if err = json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
func (r *WaitRepository) Claim(ctx context.Context, req domain.WaitRequest, owner string, graph *domain.ContractGraph) (*domain.WaitResult, error) {
	node := graph.Graph.Nodes[req.StepName]
	transitions := map[string][]string{}
	for _, t := range graph.Transitions {
		transitions[t.From] = append(transitions[t.From], t.To...)
	}
	data := map[string]any{"step": req.StepName, "id": req.InvocationID, "owner": owner, "cancel": req.Cancel, "required": node.DependsOn, "fresh": node.FreshDependsOn, "entries": graph.Graph.EntryPoints, "transitions": transitions, "strict": len(graph.Transitions) > 0}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	reply, err := r.client.Eval(ctx, claimStepScript, []string{waitPrefix(req.ThreadID)}, string(raw)).Text()
	if err != nil {
		return nil, err
	}
	var result domain.WaitResult
	if err = json.Unmarshal([]byte(reply), &result); err != nil {
		return nil, err
	}
	result.Action = "waitFor"
	result.Status = "success"
	result.ThreadID = req.ThreadID
	result.StepName = req.StepName
	result.InvocationID = req.InvocationID
	return &result, nil
}

// Watch subscribes before the caller rereads durable decisions, closing the
// check/subscribe race. Re-subscription messages also trigger a reread after a
// reconnect. Each pending wait owns a bounded subscription and closes it on exit.
func (r *WaitRepository) Watch(ctx context.Context, threadID string) (<-chan interface{}, func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	sub := r.client.Subscribe(watchCtx, waitPrefix(threadID)+"wait_changes")
	stop := context.AfterFunc(watchCtx, func() { _ = sub.Close() })
	if _, err := sub.Receive(watchCtx); err != nil {
		cancel()
		stop()
		_ = sub.Close()
		return nil, nil, err
	}
	changes := make(chan interface{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(changes)
		for {
			// Cancellation closes the socket via AfterFunc. Do not also set its
			// read deadline: the network timer can fire just before ctx.Err updates.
			message, err := sub.Receive(context.WithoutCancel(watchCtx))
			if err != nil {
				return
			}
			switch message.(type) {
			case *redis.Message, *redis.Subscription:
				// A signal means reread all relevant state. Coalesce bursts instead of
				// blocking a subscriber goroutine when the caller is evaluating a claim.
				select {
				case changes <- struct{}{}:
				default:
				}
			}
		}
	}()
	closeWatch := func() { cancel(); stop(); _ = sub.Close(); <-done }
	return changes, closeWatch, nil
}
