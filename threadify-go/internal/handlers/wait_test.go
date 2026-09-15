package handlers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/domain"
)

type deferredWaitService struct {
	domain.ThreadService
	entered         chan domain.WaitRequest
	release         chan struct{}
	once            sync.Once
	releaseOnRecord bool
}

func (s *deferredWaitService) HandleWaitFor(_ context.Context, req domain.WaitRequest, _, _ string) *domain.WaitResult {
	return &domain.WaitResult{Action: "waitFor", Status: "success", Decision: "pending", ThreadID: req.ThreadID, StepName: req.StepName}
}
func (s *deferredWaitService) AwaitWaitFor(ctx context.Context, req domain.WaitRequest, _, _ string) *domain.WaitResult {
	s.entered <- req
	result := &domain.WaitResult{Action: "waitFor", Status: "success", ThreadID: req.ThreadID, StepName: req.StepName, StepID: req.StepID, InvocationID: req.InvocationID}
	select {
	case <-ctx.Done():
		result.Decision = "cancelled"
		if ctx.Err() == context.DeadlineExceeded {
			result.Decision = "timed_out"
		}
	case <-s.release:
		result.Decision = "allowed"
		if req.StepID != "" {
			result.Decision = "passed"
		}
	}
	return result
}
func (s *deferredWaitService) HandleRecordEvent(_ context.Context, req *domain.RecordEventCmd, _, _ string) *domain.RecordEventResponse {
	if s.releaseOnRecord {
		s.once.Do(func() { close(s.release) })
	}
	return &domain.RecordEventResponse{Action: ActionRecordThreadEvent, Status: "success", ThreadID: req.ThreadID, StepID: "event-" + req.StepName}
}
func deferredFixture(t *testing.T) (*WebSocketHandler, *deferredWaitService, *websocket.Conn) {
	t.Helper()
	svc := &deferredWaitService{entered: make(chan domain.WaitRequest, 64), release: make(chan struct{})}
	h, server := lifecycleWebSocketServer(t, svc)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/threads", nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, h.Shutdown(ctx))
	})
	return h, svc, conn
}
func awaitEntered(t *testing.T, svc *deferredWaitService) {
	t.Helper()
	select {
	case <-svc.entered:
	case <-time.After(time.Second):
		t.Fatal("server never registered pending wait")
	}
}
func readWaitResponse(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	var response map[string]any
	require.NoError(t, conn.ReadJSON(&response))
	return response
}

func TestSynchronousWaitAllowsPrerequisiteOnSameSocket(t *testing.T) {
	_, svc, conn := deferredFixture(t)
	svc.releaseOnRecord = true
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "waitFor", "threadId": "thread", "stepName": "charge", "invocationId": "invocation", "await": true, "requestId": "permission"}))
	awaitEntered(t, svc)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "recordThreadEvent", "threadId": "thread", "stepName": "approval", "requestId": "record"}))
	responses := map[string]map[string]any{}
	for i := 0; i < 2; i++ {
		r := readWaitResponse(t, conn)
		responses[r["requestId"].(string)] = r
	}
	require.Equal(t, "allowed", responses["permission"]["decision"])
	require.Equal(t, "event-approval", responses["record"]["stepId"])
}
func TestSynchronousReportReturnsValidationInOriginalResponse(t *testing.T) {
	_, svc, conn := deferredFixture(t)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "recordThreadEvent", "threadId": "thread", "stepName": "charge", "waitFor": true, "requestId": "report"}))
	awaitEntered(t, svc)
	// Heartbeat overtakes the deferred report; an early record acknowledgement
	// would be the first response and fail this assertion.
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "heartbeat", "requestId": "heartbeat"}))
	require.Equal(t, "heartbeat", readWaitResponse(t, conn)["requestId"])
	close(svc.release)
	response := readWaitResponse(t, conn)
	require.Equal(t, "report", response["requestId"])
	require.Equal(t, "recordThreadEvent", response["action"])
	validation := response["validation"].(map[string]any)
	require.Equal(t, "passed", validation["decision"])
	require.Equal(t, "event-charge", validation["stepId"])
}
func TestCancelAndDisconnectReleaseServerWaits(t *testing.T) {
	h, svc, conn := deferredFixture(t)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "waitFor", "threadId": "thread", "stepName": "charge", "await": true, "requestId": "permission"}))
	awaitEntered(t, svc)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "cancelWait", "targetRequestId": "permission"}))
	var decision any
	for i := 0; i < 2; i++ {
		r := readWaitResponse(t, conn)
		if r["action"] == "waitFor" {
			decision = r["decision"]
		}
	}
	require.Equal(t, "cancelled", decision)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "waitFor", "threadId": "thread", "stepName": "charge", "await": true, "requestId": "second"}))
	awaitEntered(t, svc)
	conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, h.Shutdown(ctx))
	h.waitMu.Lock()
	defer h.waitMu.Unlock()
	require.Zero(t, h.activeWaits)
}
func TestSynchronousReportTimeoutRetainsAcceptedEventID(t *testing.T) {
	_, _, conn := deferredFixture(t)
	require.NoError(t, conn.WriteJSON(map[string]any{"action": "recordThreadEvent", "threadId": "thread", "stepName": "charge", "waitFor": true, "timeoutMs": 20, "requestId": "report"}))
	response := readWaitResponse(t, conn)
	require.Equal(t, "event-charge", response["stepId"])
	require.Equal(t, "timed_out", response["validation"].(map[string]any)["decision"])
}
func TestWaitCapacityAndDuplicateIDs(t *testing.T) {
	h := &WebSocketHandler{}
	waits := newSocketWaits(h)
	slots := []*pendingSocketWait{}
	for i := 0; i < maxConnectionWaits; i++ {
		slot, err := waits.reserve(context.Background(), fmt.Sprint(i), 10000)
		require.NoError(t, err)
		slots = append(slots, slot)
	}
	_, err := waits.reserve(context.Background(), "over-limit", 10000)
	require.Error(t, err)
	_, err = waits.reserve(context.Background(), "0", 10000)
	require.Error(t, err)
	waits.cancelAll()
	for _, slot := range slots {
		require.ErrorIs(t, slot.ctx.Err(), context.Canceled)
		slot.finish()
		slot.finish()
	}
	waits.stop()
	require.Zero(t, h.activeWaits)
}

func TestWaitDeadlineIncludesInputAccountingTime(t *testing.T) {
	h := &WebSocketHandler{}
	waits := newSocketWaits(h)
	// Admission delayed longer than the budget must not start a fresh timer.
	received := time.Now().Add(-time.Second)
	slot, err := waits.reserveAt(context.Background(), "delayed", 100, received)
	require.NoError(t, err)
	deadline, ok := slot.ctx.Deadline()
	require.True(t, ok)
	require.Equal(t, received.Add(100*time.Millisecond), deadline)
	require.ErrorIs(t, slot.ctx.Err(), context.DeadlineExceeded)
	slot.finish()
	waits.stop()
	require.Zero(t, h.activeWaits)
}
