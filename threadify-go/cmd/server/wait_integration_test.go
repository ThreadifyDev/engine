package main

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runWaitSDKSmoke(t *testing.T, baseURL, apiKey string, request func(string, string, string, int) map[string]any) func() {
	t.Helper()
	contract := fmt.Sprintf("wait_smoke_%d", time.Now().UnixNano())
	source := `Feature: ` + contract + `
Rule: Approval
 When step "approval" is submitted
 Then owner must be "processor"
 And this step is an entry point
Rule: Charge
 When step "charge" is submitted
 Then owner must be "processor"
 And step "approval" must succeed before each invocation
 And content "amount" must be a number greater than 0
Rule: Finish
 When step "finish" is submitted
 Then owner must be "processor"
 And step "charge" must have succeeded
 And this step is terminal
`
	request("POST", "/v1/contracts", source, 200)
	script, err := sdkSmokeScript("live-wait.mjs")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("node", script, baseURL, apiKey, contract+":1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("live Node SDK waits: %v\n%s", err, output)
	}
	t.Logf("live Node SDK waits: %s", output)
	connect := func() *websocket.Conn {
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(baseURL, "http")+"/threads", nil)
		if err != nil {
			t.Fatal(err)
		}
		referenceSmokeSend(t, ws, "connect", map[string]any{"apiKey": apiKey, "serviceName": "processor"}, "success")
		return ws
	}
	ws := connect()
	thread := referenceSmokeSend(t, ws, "startThread", map[string]any{"contractName": contract + ":1", "role": "processor"}, "success")["threadId"].(string)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	referenceSmokeSend(t, ws, "recordThreadEvent", map[string]any{"threadId": thread, "stepName": "approval", "status": "success", "startedAt": now, "finishedAt": now, "idempotencyKey": uuid.NewString(), "context": map[string]string{}}, "success")
	// In the shared test the proxy puts consecutive connections on different
	// Engines. The second Engine must see the first Engine's live prerequisite.
	peer := connect()
	defer peer.Close()
	referenceSmokeSend(t, peer, "joinThread", map[string]any{"threadId": thread, "role": "processor"}, "success")
	id := uuid.NewString()
	deadline := time.Now().Add(10 * time.Second)
	for {
		reply := referenceSmokeSend(t, peer, "waitFor", map[string]any{"threadId": thread, "stepName": "charge", "invocationId": id}, "success")
		if reply["decision"] == "allowed" {
			break
		}
		if reply["decision"] != "pending" || time.Now().After(deadline) {
			t.Fatalf("permission did not become ready: %v", reply)
		}
		time.Sleep(20 * time.Millisecond)
	}
	competing := referenceSmokeSend(t, ws, "waitFor", map[string]any{"threadId": thread, "stepName": "charge", "invocationId": uuid.NewString()}, "success")
	if competing["decision"] != "pending" {
		t.Fatalf("another Engine reused the consumed approval: %v", competing)
	}
	ws.Close()
	return func() {
		ws := connect()
		defer ws.Close()
		referenceSmokeSend(t, ws, "joinThread", map[string]any{"threadId": thread, "role": "processor"}, "success")
		reply := referenceSmokeSend(t, ws, "waitFor", map[string]any{"threadId": thread, "stepName": "charge", "invocationId": uuid.NewString()}, "success")
		if reply["decision"] != "pending" {
			t.Fatalf("outstanding grant was lost across restart: %v", reply)
		}
		reply = referenceSmokeSend(t, ws, "waitFor", map[string]any{"threadId": thread, "stepName": "charge", "invocationId": id}, "success")
		if reply["decision"] != "allowed" {
			t.Fatalf("could not recover outstanding grant: %v", reply)
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		reply = referenceSmokeSend(t, ws, "recordThreadEvent", map[string]any{"threadId": thread, "stepName": "charge", "invocationId": id, "idempotencyKey": id, "status": "success", "startedAt": now, "finishedAt": now, "context": map[string]string{"amount": "1"}}, "success")
		eventID := reply["stepId"]
		deadline := time.Now().Add(10 * time.Second)
		for {
			reply = referenceSmokeSend(t, ws, "waitFor", map[string]any{"threadId": thread, "stepName": "charge", "stepId": eventID}, "success")
			if reply["decision"] == "passed" {
				break
			}
			if reply["decision"] != "pending" || time.Now().After(deadline) {
				t.Fatalf("recovered invocation did not validate: %v", reply)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Allows an isolated SDK checkout when the working SDK is being edited.
func sdkSmokeScript(name string) (string, error) {
	root := os.Getenv("THREADIFY_SMOKE_SDK_DIR")
	if root == "" {
		root = "../../../threadify-sdk"
	}
	return filepath.Abs(filepath.Join(root, "tests", name))
}
