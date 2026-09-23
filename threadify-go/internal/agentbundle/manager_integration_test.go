package agentbundle

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercises the shipped runtime across authenticated frontend suspension and server-tool resumption.
// The gateway and Engine are deterministic local fixtures; no model account is used.
func TestPortableRuntimeClientToolContinuation(t *testing.T) {
	archive := os.Getenv("THREADIFY_TEST_AGENT_ARCHIVE")
	if archive == "" {
		t.Skip("portable runtime archive not supplied")
	}
	const bearer = "Bearer local-agent-integration-user"
	const contractID = "b0c3aa89-d65e-5241-a3ba-b507c222a9ad"
	var identities, contractReads, modelCalls atomic.Int32
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != bearer {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/agent/identity":
			identities.Add(1)
			io.WriteString(w, `{"user_id":"integration-user","company_id":"integration-company"}`)
		case "/v1/contracts/" + contractID:
			contractReads.Add(1)
			io.WriteString(w, `{"id":"`+contractID+`","name":"Integration contract","content":"Feature: Checkout"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer engine.Close()
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			w.WriteHeader(404)
			return
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Errorf("decode model request: %v", err)
			w.WriteHeader(400)
			return
		}
		number := modelCalls.Add(1)
		var message map[string]any
		finish := "tool_calls"
		switch number {
		case 1:
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"index": 0, "id": "page-call", "type": "function", "function": map[string]any{"name": "get_page_context", "arguments": "{}"}}}}
		case 2:
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"index": 0, "id": "contract-call", "type": "function", "function": map[string]any{"name": "get_contract", "arguments": `{"id":"` + contractID + `"}`}}}}
		default:
			finish = "stop"
			message = map[string]any{"role": "assistant", "content": "The current page shows the Integration contract."}
		}
		response := map[string]any{"id": "chat-fixture", "object": "chat.completion", "created": 1234567, "model": "threadify-test", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}}
		if input["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			response["object"] = "chat.completion.chunk"
			response["choices"] = []any{map[string]any{"index": 0, "delta": message, "finish_reason": nil}}
			data, _ := json.Marshal(response)
			fmt.Fprintf(w, "data: %s\n\n", data)
			response["choices"] = []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}
			data, _ = json.Marshal(response)
			fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
		} else {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer gateway.Close()
	config := filepath.Join(t.TempDir(), "engine.yaml")
	os.WriteFile(config, []byte("ai:\n  agent: {}\n  gateway:\n    base_url: "+gateway.URL+"/v1\n    model: threadify-test\n"), 0600)
	manager, err := New(Options{EngineURL: engine.URL, ConfigPath: config, CacheDir: t.TempDir(), ArchivePath: archive})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start(context.Background())
	defer manager.Close()
	deadline := time.After(2 * time.Minute)
	for {
		state := manager.Status()
		if state.State == "ready" {
			break
		}
		if state.State == "unavailable" {
			diagnostics, _ := os.ReadFile(manager.DiagnosticsPath())
			t.Fatalf("%s; %s", state.Message, diagnostics)
		}
		select {
		case <-deadline:
			t.Fatal("agent did not become ready")
		case <-time.After(200 * time.Millisecond):
		}
	}
	endpoint, transport := manager.Endpoint()
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	request := func(path string, payload any) *http.Response {
		t.Helper()
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, endpoint+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", bearer)
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode >= 300 {
			data, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("%s: HTTP %d %s", path, response.StatusCode, data)
		}
		return response
	}
	response := request("/sessions", map[string]any{"state": map[string]string{"title": "Portable runtime integration"}})
	var session map[string]any
	json.NewDecoder(response.Body).Decode(&session)
	response.Body.Close()
	if session["id"] == nil {
		t.Fatal("session ID missing")
	}
	response = request("/responses", map[string]any{"input": "Read my page and its contract.", "sessionId": session["id"], "stream": true})
	var pending map[string]any
	var frames []string
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		frames = append(frames, line)
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.completed" && event["status"] == "requires_action" {
			pending, _ = event["requiredAction"].(map[string]any)
		}
	}
	response.Body.Close()
	if scanner.Err() != nil {
		t.Fatal(scanner.Err())
	}
	if pending == nil || pending["name"] != "get_page_context" {
		diagnostics, _ := os.ReadFile(manager.DiagnosticsPath())
		t.Fatalf("missing frontend tool pause: %v; %s", frames, diagnostics)
	}
	response = request("/client-tools/"+pending["id"].(string), map[string]any{"output": map[string]any{"route": "/u/contracts", "contractId": contractID}})
	var result map[string]any
	json.NewDecoder(response.Body).Decode(&result)
	response.Body.Close()
	if result["status"] != "completed" || !strings.Contains(fmt.Sprint(result["outputText"]), "Integration contract") {
		t.Fatalf("continuation failed: %#v", result)
	}
	if identities.Load() < 3 || contractReads.Load() != 1 || modelCalls.Load() != 3 {
		t.Fatalf("missing authenticated resumption: identity=%d contract=%d model=%d", identities.Load(), contractReads.Load(), modelCalls.Load())
	}
}
