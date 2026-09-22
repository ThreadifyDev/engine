package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

// TestLiveRegistryFailureScenarios is opt-in and operates only on explicitly
// supplied disposable infrastructure. The Engine is a separately compiled
// process; Registry control changes real product plans/status through its fixture.
// The fixture may accelerate heartbeat/grace to 1s/3s; authentication, product
// handlers, usage persistence, and all Engine HTTP/WS behavior remain real.
func TestLiveRegistryFailureScenarios(t *testing.T) {
	names := []string{"BINARY", "POSTGRES_URL", "VALKEY_ADDR", "REGISTRY_URL", "CONTROL_URL", "CONTROL_TOKEN", "ACCOUNT_ID", "LICENSE_KEY"}
	env := map[string]string{}
	for _, name := range names {
		env[name] = os.Getenv("THREADIFY_LIVE_" + name)
		if env[name] == "" {
			t.Skip("requires all explicitly supplied THREADIFY_LIVE_* fixture settings")
		}
	}
	binary, err := filepath.Abs(env["BINARY"])
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, env["POSTGRES_URL"])
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	limits := func() map[string]int64 {
		return map[string]int64{"input_bandwidth_bytes": -1, "output_bandwidth_bytes": -1, "input_requests_per_second": -1, "entity_profile_limit": -1}
	}
	control := func(changes map[string]any) {
		t.Helper()
		changes["account_id"] = env["ACCOUNT_ID"]
		data, e := json.Marshal(changes)
		if e != nil {
			t.Fatal(e)
		}
		req, e := http.NewRequestWithContext(ctx, "POST", env["CONTROL_URL"], bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+env["CONTROL_TOKEN"])
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatalf("fixture control status%d: %s", res.StatusCode, body)
		}
	}
	reset := func() {
		control(map[string]any{"suspended": false, "registry_unavailable": false, "usage_unavailable": false, "entitlements": limits(), "heartbeat_interval_seconds": 1, "grace_period_seconds": 3})
	}
	reset()
	defer reset()
	redisDB := 12
	if configured := os.Getenv("THREADIFY_LIVE_VALKEY_DB"); configured != "" {
		redisDB, err = strconv.Atoi(configured)
		if err != nil || redisDB < 0 {
			t.Fatal("invalid THREADIFY_LIVE_VALKEY_DB")
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	v := viper.New()
	v.SetConfigFile("../../config/config.selfhost.yaml")
	if err = v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]any{
		"registry.url": env["REGISTRY_URL"], "registry.license_key": env["LICENSE_KEY"], "registry.company_id": env["ACCOUNT_ID"],
		"server.host": "127.0.0.1", "server.port": httpPort, "postgres.url": env["POSTGRES_URL"],
		"redis.url": fmt.Sprintf("redis://%s/%d", env["VALKEY_ADDR"], redisDB),
		"nats.mode": "embedded", "nats.store_dir": filepath.Join(work, "jetstream"), "rate_limit.enabled": false, "rate_limit.ip_rate_limit_enabled": false,
		"jwks.url": "http://127.0.0.1:1/unused-jwks", "supabase.url": "",
		"security.hash_chain_secrets.v1":    "isolated-live-test-not-a-production-secret",
		"archiver.streams.block_timeout_ms": 100, "archiver.streams.step_state_flush_interval_ms": 100,
	} {
		v.Set(key, value)
	}
	configPath := filepath.Join(work, "config.yaml")
	if err = v.WriteConfigAs(configPath); err != nil {
		t.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	start := func() func() {
		t.Helper()
		logs, e := os.CreateTemp(work, "live-engine-*.log")
		if e != nil {
			t.Fatal(e)
		}
		cmd := exec.Command(binary, "--config", configPath)
		cmd.Dir = work
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GO_ENV=production"}
		cmd.Stdout = logs
		cmd.Stderr = logs
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); close(done) }()
		stopped := false
		t.Cleanup(func() {
			if !stopped {
				_ = cmd.Process.Kill()
				<-done
			}
			logs.Close()
		})
		deadline := time.Now().Add(30 * time.Second)
		for {
			select {
			case e := <-done:
				stopped = true
				data, _ := os.ReadFile(logs.Name())
				t.Fatalf("compiled Engine exited: %v\n%s", e, data)
			default:
			}
			res, e := client.Get(baseURL + "/health")
			if e == nil {
				var health map[string]any
				_ = json.NewDecoder(res.Body).Decode(&health)
				res.Body.Close()
				if res.StatusCode == 200 && health["persistence"] == "ok" {
					break
				}
			}
			if time.Now().After(deadline) {
				data, _ := os.ReadFile(logs.Name())
				t.Fatalf("compiled Engine readiness timeout\n%s", data)
			}
			time.Sleep(50 * time.Millisecond)
		}
		return func() {
			t.Helper()
			if stopped {
				return
			}
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case e := <-done:
				stopped = true
				if e != nil {
					data, _ := os.ReadFile(logs.Name())
					t.Fatalf("compiled Engine shutdown: %v\n%s", e, data)
				}
			case <-time.After(35 * time.Second):
				t.Fatal("compiled Engine shutdown timeout")
			}
		}
	}
	stop := start()
	defer func() { stop() }()
	owner, keyID, apiKey := uuid.NewString(), uuid.NewString(), "tf_"+uuid.NewString()
	sum := sha256.Sum256([]byte(apiKey))
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,'Live test')", []any{owner, env["ACCOUNT_ID"]}},
		{"INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix) VALUES($1,$2,$3,$4,'tf_')", []any{keyID, owner, env["ACCOUNT_ID"], hex.EncodeToString(sum[:])}},
		{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','owner',$1)", []any{owner}},
		{"INSERT INTO entity_profile_type(id,company_id,name,slug,type) VALUES($1,$2,'Live profiles','live-profiles',ARRAY['live_customer'])", []any{uuid.NewString(), env["ACCOUNT_ID"]}},
	} {
		if _, err = pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if e := pool.QueryRow(ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	wait := func(description string, timeout time.Duration, condition func() bool) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatal(description)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	request := func(path string, body []byte) (int, []byte, http.Header) {
		t.Helper()
		method := "GET"
		if body != nil {
			method = "POST"
		}
		req, e := http.NewRequestWithContext(ctx, method, baseURL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil {
			t.Fatalf("read HTTP response: %v", e)
		}
		return res.StatusCode, data, res.Header
	}
	checkHealth := func() {
		t.Helper()
		status, body, _ := request("/health", nil)
		var health map[string]any
		if (status != 200 && status != 503) || json.Unmarshal(body, &health) != nil || health["nats"] == nil || health["persistence"] == nil {
			t.Fatalf("quota/suspension hid health diagnostics: HTTP%d %s", status, body)
		}
		if status == 503 {
			t.Logf("health diagnostics available while persistence settles: %s", body)
		}
	}
	graphql := []byte(`{"query":"{__typename}"}`)
	waitHealthy := func() {
		wait("Registry policy did not restore all unrestricted limits", 8*time.Second, func() bool {
			status, body, _ := request("/v1/pricing", nil)
			if status != 200 {
				return false
			}
			var pricing struct {
				Limits map[string]json.RawMessage `json:"limits"`
			}
			if json.Unmarshal(body, &pricing) != nil {
				return false
			}
			for name, expected := range limits() {
				var got int64
				if json.Unmarshal(pricing.Limits[name], &got) != nil || got != expected {
					return false
				}
			}
			return true
		})
	}
	setLimit := func(name string, value int64) {
		policy := limits()
		policy[name] = value
		control(map[string]any{"entitlements": policy})
	}
	waitLimit := func(name string, value int64) {
		wait("Engine did not refresh limit "+name, 8*time.Second, func() bool {
			status, body, _ := request("/v1/pricing", nil)
			if status != 200 {
				return false
			}
			var pricing struct {
				Limits map[string]json.RawMessage `json:"limits"`
			}
			if json.Unmarshal(body, &pricing) != nil {
				return false
			}
			var got int64
			if json.Unmarshal(pricing.Limits[name], &got) != nil {
				return false
			}
			return got == value
		})
	}
	sendWS := func(conn *websocket.Conn, action string, payload map[string]any) map[string]any {
		t.Helper()
		payload["action"] = action
		if e := conn.WriteJSON(payload); e != nil {
			t.Fatal(e)
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		for {
			var response map[string]any
			if e := conn.ReadJSON(&response); e != nil {
				t.Fatal(e)
			}
			if response["action"] != action {
				continue
			}
			if response["status"] != "success" {
				t.Fatalf("live %s rejected: %v", action, response)
			}
			return response
		}
	}
	connectWS := func() *websocket.Conn {
		t.Helper()
		conn, _, e := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/threads", httpPort), nil)
		if e != nil {
			t.Fatal(e)
		}
		sendWS(conn, "connect", map[string]any{"apiKey": apiKey, "serviceName": "live-test"})
		return conn
	}
	rejectWSMutation := func(conn *websocket.Conn, label string) {
		t.Helper()
		_ = conn.WriteJSON(map[string]any{"action": "startThread", "label": label, "role": "owner"})
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var response map[string]any
		e := conn.ReadJSON(&response)
		conn.Close()
		if e == nil && response["status"] == "success" {
			t.Fatalf("denied WS mutation succeeded: %v", response)
		}
		if n := count("SELECT COUNT(*) FROM threads WHERE company_id=$1 AND label=$2", env["ACCOUNT_ID"], label); n != 0 {
			t.Fatal("rejected websocket input mutated a thread")
		}
	}
	if status, body, _ := request("/graphql", graphql); status != 200 {
		t.Fatalf("baseline GraphQL: %d %s", status, body)
	}

	t.Log("profile cap and heartbeat upgrade via public WebSocket refs")
	setLimit("entity_profile_limit", 1)
	waitLimit("entity_profile_limit", 1)
	ws := connectWS()
	first := sendWS(ws, "startThread", map[string]any{"label": "live profile one", "role": "owner", "refs": map[string]string{"live_customer": "first"}})
	second := sendWS(ws, "startThread", map[string]any{"label": "live profile two", "role": "owner", "refs": map[string]string{"live_customer": "second"}})
	ids := []string{first["threadId"].(string), second["threadId"].(string)}
	wait("accepted profile threads/refs did not persist", 10*time.Second, func() bool {
		return count("SELECT COUNT(*) FROM threads WHERE id=ANY($1)", ids) == 2 && count("SELECT COUNT(*) FROM thread_refs WHERE thread_id=ANY($1) AND ref_key='live_customer'", ids) == 2
	})
	if n := count("SELECT COUNT(*) FROM entity_profile WHERE company_id=$1", env["ACCOUNT_ID"]); n != 1 {
		t.Fatalf("profile cap1 produced %d profiles", n)
	}
	setLimit("entity_profile_limit", 2)
	waitLimit("entity_profile_limit", 2)
	sendWS(ws, "addRefs", map[string]any{"threadId": ids[0], "refs": map[string]string{"live_customer": "first"}})
	sendWS(ws, "addRefs", map[string]any{"threadId": ids[1], "refs": map[string]string{"live_customer": "second"}})
	wait("upgraded profile allowance did not materialize second profile", 10*time.Second, func() bool {
		return count("SELECT COUNT(*) FROM entity_profile WHERE company_id=$1", env["ACCOUNT_ID"]) == 2
	})
	if count("SELECT COUNT(*) FROM threads WHERE id=ANY($1)", ids) != 2 || count("SELECT COUNT(*) FROM thread_refs WHERE thread_id=ANY($1) AND ref_key='live_customer'", ids) != 2 {
		t.Fatal("profile limit changed accepted threads or refs")
	}
	ws.Close()
	reset()
	waitHealthy()

	t.Log("input quota rejection before HTTP/WS mutation")
	ws = connectWS()
	setLimit("input_bandwidth_bytes", 0)
	waitLimit("input_bandwidth_bytes", 0)
	if status, _, _ := request("/graphql", graphql); status != 429 {
		t.Fatalf("input bandwidth cap: HTTP%d", status)
	}
	rejectWSMutation(ws, "must-not-persist-input")
	reset()
	waitHealthy()

	t.Log("incoming request rate and output bandwidth quotas")
	for _, metric := range []string{"input_requests_per_second", "output_bandwidth_bytes"} {
		setLimit(metric, 0)
		wait("quota did not deny "+metric, 8*time.Second, func() bool { status, _, _ := request("/graphql", graphql); return status == 429 })
		status, body, headers := request("/graphql", graphql)
		if status != 429 {
			t.Fatalf("%s: HTTP%d", metric, status)
		}
		if metric == "output_bandwidth_bytes" {
			if len(body) != 0 || headers.Get("Content-Length") != "" && headers.Get("Content-Length") != "0" {
				t.Fatalf("%s leaked output: headers%v body%s", metric, headers, body)
			}
		}
		checkHealth()
		reset()
		waitHealthy()
	}

	t.Log("real Registry suspension blocks HTTP/WS then restores")
	ws = connectWS()
	control(map[string]any{"suspended": true})
	wait("suspension did not block Engine", 8*time.Second, func() bool { status, _, _ := request("/graphql", graphql); return status == 503 })
	rejectWSMutation(ws, "must-not-persist-suspended")
	checkHealth()
	reset()
	waitHealthy()

	t.Log("Registry outage retains verified limits and HTTP/WS availability beyond old grace")
	ws = connectWS()
	control(map[string]any{"registry_unavailable": true})
	outageDeadline := time.Now().Add(4 * time.Second) // Fixture's former grace is three seconds.
	var registryOutageIDs []string
	for {
		if status, body, _ := request("/graphql", graphql); status != 200 {
			t.Fatalf("Registry outage interrupted HTTP availability: HTTP%d %s", status, body)
		}
		response := sendWS(ws, "startThread", map[string]any{"label": "live Registry outage", "role": "owner"})
		registryOutageIDs = append(registryOutageIDs, response["threadId"].(string))
		if time.Now().After(outageDeadline) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	ws.Close()
	wait("Registry outage lost accepted threads", 10*time.Second, func() bool {
		return count("SELECT COUNT(*) FROM threads WHERE id=ANY($1)", registryOutageIDs) == int64(len(registryOutageIDs))
	})
	// Recovery must actually apply new limits; an unchanged healthy response
	// alone would not prove heartbeat recovery while the cached policy works.
	setLimit("input_bandwidth_bytes", 0)
	control(map[string]any{"registry_unavailable": false})
	waitLimit("input_bandwidth_bytes", 0)
	if status, _, _ := request("/graphql", graphql); status != 429 {
		t.Fatalf("recovered Registry input allowance not enforced: HTTP%d", status)
	}
	reset()
	waitHealthy()

	t.Log("usage delivery outage survives compiled-process restart")
	control(map[string]any{"usage_unavailable": true})
	ws = connectWS()
	var outageIDs []string
	for i := 0; i < 2; i++ {
		response := sendWS(ws, "startThread", map[string]any{"label": "live usage outage", "role": "owner"})
		outageIDs = append(outageIDs, response["threadId"].(string))
	}
	ws.Close()
	wait("usage fault did not retain durable outbox", 5*time.Second, func() bool { return count("SELECT COUNT(*) FROM threadify_registry_outbox") > 0 })
	pendingBefore := count("SELECT COUNT(*) FROM threadify_registry_outbox")
	usageBefore := count("SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.input.bytes'")
	stop()
	stop = start()
	if count("SELECT COUNT(*) FROM threadify_registry_outbox") < pendingBefore {
		t.Fatal("restart lost unacknowledged usage")
	}
	if count("SELECT COALESCE(SUM(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.input.bytes'") < usageBefore {
		t.Fatal("restart reset durable bandwidth usage")
	}
	wait("restart lost accepted outage threads", 10*time.Second, func() bool { return count("SELECT COUNT(*) FROM threads WHERE id=ANY($1)", outageIDs) == 2 })
	control(map[string]any{"usage_unavailable": false})
	wait("restored Registry did not acknowledge outbox", 10*time.Second, func() bool { return count("SELECT COUNT(*) FROM threadify_registry_outbox") == 0 })
	if n := count("SELECT COUNT(*) FROM credit_accounts WHERE company_id=$1", env["ACCOUNT_ID"]); n != 0 {
		t.Fatal("live licensed runtime created legacy credit accounts")
	}
	// The intervening orderly restart drained persistence; this also detects
	// an incorrectly accepted rejection which reached the archiver late.
	if count("SELECT COUNT(*) FROM threads WHERE company_id=$1 AND label=ANY($2)", env["ACCOUNT_ID"], []string{"must-not-persist-input", "must-not-persist-suspended"}) != 0 {
		t.Fatal("a rejected mutation appeared after persistence flushed")
	}
	t.Log("all live failure/recovery scenarios passed against compiled Engine and Registry handlers")
}
