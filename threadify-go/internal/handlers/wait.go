package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/dto"
)

const maxConnectionWaits = 32
const maxEngineWaits = 256

type pendingSocketWait struct {
	ctx    context.Context
	cancel context.CancelFunc
	finish func()
}
type socketWaits struct {
	mu      sync.Mutex
	pending map[string]*pendingSocketWait
	wg      sync.WaitGroup
	handler *WebSocketHandler
}

func newSocketWaits(h *WebSocketHandler) *socketWaits {
	return &socketWaits{pending: map[string]*pendingSocketWait{}, handler: h}
}
func (w *socketWaits) reserve(ctx context.Context, id string, timeoutMs int) (*pendingSocketWait, error) {
	return w.reserveAt(ctx, id, timeoutMs, time.Now())
}
func (w *socketWaits) reserveAt(ctx context.Context, id string, timeoutMs int, receivedAt time.Time) (*pendingSocketWait, error) {
	if id == "" || len(id) > 128 {
		return nil, fmt.Errorf("a requestId of 1–128 characters is required for a synchronous wait")
	}
	if timeoutMs == 0 {
		timeoutMs = 10000
	}
	if timeoutMs < 1 || timeoutMs > 300000 {
		return nil, fmt.Errorf("timeoutMs must be between 1 and 300000")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.pending[id]; exists {
		return nil, fmt.Errorf("requestId is already pending")
	}
	if len(w.pending) >= maxConnectionWaits {
		return nil, fmt.Errorf("too many pending waits on this connection")
	}
	w.handler.waitMu.Lock()
	if w.handler.activeWaits >= maxEngineWaits {
		w.handler.waitMu.Unlock()
		return nil, fmt.Errorf("too many pending waits on this Engine")
	}
	w.handler.activeWaits++
	w.handler.waitMu.Unlock()
	waitCtx, cancel := context.WithDeadline(ctx, receivedAt.Add(time.Duration(timeoutMs)*time.Millisecond))
	slot := &pendingSocketWait{ctx: waitCtx, cancel: cancel}
	var once sync.Once
	slot.finish = func() {
		once.Do(func() {
			cancel()
			w.mu.Lock()
			delete(w.pending, id)
			w.mu.Unlock()
			w.handler.waitMu.Lock()
			w.handler.activeWaits--
			w.handler.waitMu.Unlock()
			w.wg.Done()
		})
	}
	w.pending[id] = slot
	w.wg.Add(1)
	return slot, nil
}
func (w *socketWaits) cancel(id string) {
	w.mu.Lock()
	slot := w.pending[id]
	w.mu.Unlock()
	if slot != nil {
		slot.cancel()
	}
}
func (w *socketWaits) cancelAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, slot := range w.pending {
		slot.cancel()
	}
}
func (w *socketWaits) stop() { w.cancelAll(); w.wg.Wait() }

func correlatedResponse(response interface{}, msg map[string]interface{}) interface{} {
	if id, ok := msg["requestId"].(string); ok && len(id) <= 128 {
		raw, err := json.Marshal(response)
		var envelope map[string]interface{}
		if err == nil && json.Unmarshal(raw, &envelope) == nil && envelope != nil {
			envelope["requestId"] = id
			return envelope
		}
	}
	return response
}

// beginSocketWait reserves capacity before accepting an event that requests a
// final validation response. Ordinary event handling remains ordered in the reader.
func (h *WebSocketHandler) beginSocketWait(ctx context.Context, action string, msg map[string]interface{}, raw []byte, waits *socketWaits, receivedAt time.Time) (*pendingSocketWait, error) {
	await, _ := msg["await"].(bool)
	reportWait, _ := msg["waitFor"].(bool)
	cancel, _ := msg["cancel"].(bool)
	if !(action == "waitFor" && await && !cancel || action == ActionRecordThreadEvent && reportWait) {
		return nil, nil
	}
	if _, ok := h.threadService.(domain.AwaitService); !ok {
		return nil, fmt.Errorf("synchronous waits are unavailable")
	}
	var options struct {
		TimeoutMs int `json:"timeoutMs"`
	}
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, fmt.Errorf("invalid wait options")
	}
	id, _ := msg["requestId"].(string)
	return waits.reserveAt(ctx, id, options.TimeoutMs, receivedAt)
}

// deferSocketResponse returns true only when a goroutine owns the final response.
// It captures authentication values while the reader owns the session; it never
// reads mutable session identity from a background goroutine.
func (h *WebSocketHandler) deferSocketResponse(action string, raw []byte, msg map[string]interface{}, response interface{}, session *WSSession, slot *pendingSocketWait) bool {
	var req domain.WaitRequest
	var report *dto.RecordEventResponse
	switch action {
	case "waitFor":
		result, ok := response.(*domain.WaitResult)
		if !ok || result.Decision != "pending" {
			return false
		}
		if json.Unmarshal(raw, &req) != nil {
			return false
		}
	case ActionRecordThreadEvent:
		var ok bool
		report, ok = response.(*dto.RecordEventResponse)
		if !ok || report.Status != StatusSuccess || report.StepID == "" {
			return false
		}
		var event dto.RecordEventRequest
		if json.Unmarshal(raw, &event) != nil {
			return false
		}
		req = domain.WaitRequest{ThreadID: event.ThreadID, StepName: event.StepName, StepID: report.StepID, InvocationID: event.InvocationID, Await: true}
	default:
		return false
	}
	owner, company := session.ownerID, session.companyID
	waiter := h.threadService.(domain.AwaitService)
	go func() {
		defer slot.finish()
		result := waiter.AwaitWaitFor(slot.ctx, req, owner, company)
		var final interface{} = result
		if report != nil {
			report.Validation = result
			final = report
		}
		if err := session.SendMessage(correlatedResponse(final, msg)); err != nil {
			if closer, ok := session.conn.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}()
	return true
}
