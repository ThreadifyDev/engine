package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// prepareCrossStepSmoke records real successful history before restart and
// verifies that persisted references still reject mismatches afterward.
func prepareCrossStepSmoke(t *testing.T, baseURL, apiKey string, pool *pgxpool.Pool, request func(string, string, string, int) map[string]any) func() {
	t.Helper()
	contractName := fmt.Sprintf("reference_smoke_%d", time.Now().UnixNano())
	source := `Feature: ` + contractName + `
Version: 1
Description: Verify delivery against the latest successful shipment.
Rule: Record a shipment
 When step "order_shipped" is submitted
 Then owner must be "carrier"
 And content "tracking_number" is optional
 And this step is an entry point
Rule: Confirm delivery
 When step "delivery_confirmed" is submitted
 Then owner must be "carrier"
 And content "tracking_number" must equal order_shipped.tracking_number
 And this step is terminal
`
	request("POST", "/v1/contracts", source, 200)
	connect := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(baseURL, "http")+"/threads", nil)
		if err != nil {
			t.Fatal(err)
		}
		referenceSmokeSend(t, ws, "connect", map[string]any{"apiKey": apiKey, "serviceName": "reference-smoke"}, "success")
		return ws
	}
	ws := connect()
	defer ws.Close()
	start := func() string {
		return referenceSmokeSend(t, ws, "startThread", map[string]any{"contractName": contractName + ":1", "role": "carrier"}, "success")["threadId"].(string)
	}
	a, b, c := start(), start(), start()
	record := func(thread, key, status string, content map[string]string) {
		referenceSmokeSend(t, ws, "recordThreadEvent", map[string]any{"threadId": thread, "stepName": "order_shipped", "status": status, "startedAt": "2000-01-01T00:00:00Z", "finishedAt": "2000-01-01T00:00:00Z", "idempotencyKey": key, "context": content}, "success")
	}
	waitContent := func(thread string, want map[string]string) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for {
			var raw string
			err := pool.QueryRow(context.Background(), "SELECT context::text FROM thread_successful_contexts WHERE thread_id=$1 AND step_name='order_shipped'", thread).Scan(&raw)
			var content map[string]string
			if err == nil {
				err = json.Unmarshal([]byte(raw), &content)
			}
			wantJSON, _ := json.Marshal(want)
			gotJSON, _ := json.Marshal(content)
			if err == nil && string(gotJSON) == string(wantJSON) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("successful context did not persist for %s: got %s err %v", thread, raw, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	record(a, "a1", "success", map[string]string{"tracking_number": "OLD"})
	waitContent(a, map[string]string{"tracking_number": "OLD"})
	record(a, "a2", "success", map[string]string{"tracking_number": "NEW"})
	waitContent(a, map[string]string{"tracking_number": "NEW"})
	record(a, "a3", "failed", map[string]string{"tracking_number": "FAILED"})
	record(b, "b1", "success", map[string]string{"tracking_number": "OTHER"})
	waitContent(b, map[string]string{"tracking_number": "OTHER"})
	record(c, "c1", "success", map[string]string{"tracking_number": "OLDER"})
	waitContent(c, map[string]string{"tracking_number": "OLDER"})
	record(c, "c2", "success", map[string]string{})
	waitContent(c, map[string]string{})
	return func() {
		t.Helper()
		ws := connect()
		defer ws.Close()
		// Rejoin existing threads after reconnect before exercising their write permissions.
		for _, thread := range []string{a, b, c} {
			referenceSmokeSend(t, ws, "joinThread", map[string]any{"threadId": thread, "role": "carrier"}, "success")
		}
		for i, tc := range []struct{ thread, value, want string }{{a, "OLD", "error"}, {a, "FAILED", "error"}, {a, "OTHER", "error"}, {c, "OLDER", "error"}, {a, "NEW", "success"}, {b, "OTHER", "success"}} {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			result := referenceSmokeSend(t, ws, "recordThreadEvent", map[string]any{"threadId": tc.thread, "stepName": "delivery_confirmed", "status": "success", "startedAt": now, "finishedAt": now, "idempotencyKey": fmt.Sprintf("delivery-%d", i), "context": map[string]string{"tracking_number": tc.value}}, tc.want)
			if tc.want == "error" && !strings.Contains(fmt.Sprint(result["message"]), "Step validation failed") {
				t.Fatalf("wrong rejection: %v", result)
			}
		}
	}
}

// Skip notifications while waiting for the response to one explicitly submitted action.
func referenceSmokeSend(t *testing.T, ws *websocket.Conn, action string, payload map[string]any, want string) map[string]any {
	t.Helper()
	payload["action"] = action
	if err := ws.WriteJSON(payload); err != nil {
		t.Fatal(err)
	}
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var result map[string]any
		if err := ws.ReadJSON(&result); err != nil {
			t.Fatal(err)
		}
		if result["action"] != action {
			continue
		}
		if result["status"] != want {
			t.Fatalf("%s: wanted %s, got %v", action, want, result)
		}
		return result
	}
}
