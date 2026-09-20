package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"threadify-go/shared/testutil/registryfixture"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
)

// This test uses only explicitly supplied disposable services, never developer defaults.
// Build bin/threadify first and set all three THREADIFY_SMOKE_* variables to opt in.
func TestStandaloneBinaryPersistenceAndRestart(t *testing.T) { runStandalone(t, false) }
func TestTwoEnginesShareManagedValkey(t *testing.T) {
	if os.Getenv("THREADIFY_SMOKE_MANAGED_VALKEY_BINARY") == "" {
		t.Skip("requires managed Valkey binary")
	}
	runStandalone(t, true)
}
func runStandalone(t *testing.T, shared bool) {
	binary, pgURL, valkeyAddr := os.Getenv("THREADIFY_SMOKE_BINARY"), os.Getenv("THREADIFY_SMOKE_POSTGRES_URL"), os.Getenv("THREADIFY_SMOKE_VALKEY_ADDR")
	managedBinary := os.Getenv("THREADIFY_SMOKE_MANAGED_VALKEY_BINARY")
	if managedBinary != "" && valkeyAddr == "" {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		valkeyAddr = l.Addr().String()
		l.Close()
	}
	if binary == "" || pgURL == "" || valkeyAddr == "" {
		t.Skip("requires a built binary and explicitly supplied disposable PostgreSQL and Valkey")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	company := "8bf9099d-2ff9-4d88-a2eb-acb114679909"
	registryURL := os.Getenv("THREADIFY_SMOKE_REGISTRY_URL")
	if registryURL == "" {
		registryURL = registryfixture.New(t, company).URL
	}
	work := t.TempDir()
	image, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(work, "threadify")
	if err := os.WriteFile(executable, image, 0700); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	v.SetConfigFile("../../config/config.selfhost.yaml")
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	sub, err := os.Open("../../config/subscription.selfhost.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.MergeConfig(sub); err != nil {
		t.Fatal(err)
	}
	sub.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	for key, value := range map[string]any{
		"registry.license_key": registryfixture.License, "registry.company_id": company,
		"server.host": "127.0.0.1", "server.port": httpPort, "postgres.url": pgURL,
		"redis.url": "redis://" + valkeyAddr + "/0",
		"jwks.url":  "http://127.0.0.1:1/unused-jwks", "supabase.url": "",
		"security.hash_chain_secrets.v1": "isolated-test-hash-chain-key-not-a-production-secret",
		"billing.provider":               "noop", "archiver.streams.block_timeout_ms": 100,
		"archiver.streams.step_state_flush_interval_ms": 100,
	} {
		v.Set(key, value)
	}
	if managedBinary != "" {
		image, err := os.ReadFile(managedBinary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(work, "libexec"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, "libexec", "valkey-server"), image, 0700); err != nil {
			t.Fatal(err)
		}
		v.Set("redis.mode", "managed")
	}
	if shared {
		broker, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: filepath.Join(work, "shared-jetstream"), NoLog: true, NoSigs: true})
		if err != nil {
			t.Fatal(err)
		}
		go broker.Start()
		t.Cleanup(func() { broker.Shutdown(); broker.WaitForShutdown() })
		if !broker.ReadyForConnections(5 * time.Second) {
			t.Fatal("shared NATS not ready")
		}
		v.Set("nats.mode", "external")
		v.Set("nats.url", broker.ClientURL())
	}
	configPath := filepath.Join(work, "config.yaml")
	if err := v.WriteConfigAs(configPath); err != nil {
		t.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	client := &http.Client{Timeout: time.Second}
	launchDir := t.TempDir() // Launching elsewhere must not move persistent storage.
	start := func(configPath, baseURL string) func() {
		t.Helper()
		logs, err := os.CreateTemp(work, "engine-*.log")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "--config", configPath)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GO_ENV=production", "THREADIFY_REGISTRY_URL=" + registryURL}
		cmd.Dir = launchDir
		cmd.Stdout = logs
		cmd.Stderr = logs
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait(); close(done) }()
		stopped := false
		t.Cleanup(func() {
			if !stopped {
				// Let the Engine reap its managed child even when an assertion
				// fails. Killing only the owner can intentionally leave Valkey up.
				_ = cmd.Process.Signal(syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(35 * time.Second):
					_ = cmd.Process.Kill()
					<-done
				}
			}
			logs.Close()
		})
		deadline := time.Now().Add(25 * time.Second)
		for {
			select {
			case err := <-done:
				data, _ := os.ReadFile(logs.Name())
				t.Fatalf("engine exited: %v\n%s", err, data)
			default:
			}
			response, err := client.Get(baseURL + "/health")
			if err == nil {
				var health map[string]any
				_ = json.NewDecoder(response.Body).Decode(&health)
				response.Body.Close()
				if response.StatusCode == 200 && health["persistence"] == "ok" && health["nats"] == "ok" {
					break
				}
			}
			if time.Now().After(deadline) {
				data, _ := os.ReadFile(logs.Name())
				t.Fatalf("engine not ready\n%s", data)
			}
			time.Sleep(50 * time.Millisecond)
		}
		healthCmd := exec.Command(executable, "--config", configPath, "--healthcheck")
		healthCmd.Env = cmd.Env
		healthCmd.Dir = work
		// Background persistence readiness can briefly transition during recovery;
		// require the CLI probe to become healthy within the same startup budget.
		probeDeadline := time.Now().Add(10 * time.Second)
		for {
			probe := exec.Command(executable, "--config", configPath, "--healthcheck")
			probe.Env = healthCmd.Env
			probe.Dir = healthCmd.Dir
			out, err := probe.CombinedOutput()
			if err == nil {
				break
			}
			if time.Now().After(probeDeadline) {
				data, _ := os.ReadFile(logs.Name())
				t.Fatalf("binary healthcheck: %v %s\n%s", err, out, data)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return func() {
			t.Helper()
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case err := <-done:
				stopped = true
				if err != nil {
					data, _ := os.ReadFile(logs.Name())
					t.Fatalf("shutdown: %v\n%s", err, data)
				}
			case <-time.After(35 * time.Second):
				t.Fatal("shutdown did not complete")
			}
		}
	}
	stop := start(configPath, baseURL)
	// Omitted broker configuration stores data beside the installed executable.
	if _, err := os.Stat(filepath.Join(work, "data", "jetstream")); err != nil && !shared {
		t.Fatalf("binary-relative broker store missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(launchDir, "data")); !os.IsNotExist(err) {
		t.Fatalf("storage leaked into working directory: %v", err)
	}
	apiURL := baseURL
	var stopJoiner func()
	var joinPath, joinURL string
	if shared {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		joinPort := l.Addr().(*net.TCPAddr).Port
		l.Close()
		v.Set("server.port", joinPort)
		v.Set("redis.mode", "external")
		joinPath = filepath.Join(work, "join.yaml")
		joinURL = fmt.Sprintf("http://127.0.0.1:%d", joinPort)
		if err := v.WriteConfigAs(joinPath); err != nil {
			t.Fatal(err)
		}
		stopJoiner = start(joinPath, joinURL)
		targets := []*httputil.ReverseProxy{}
		for _, address := range []string{baseURL, joinURL} {
			u, _ := url.Parse(address)
			targets = append(targets, httputil.NewSingleHostReverseProxy(u))
		}
		var requests atomic.Uint64
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targets[requests.Add(1)%2].ServeHTTP(w, r) }))
		t.Cleanup(proxy.Close)
		apiURL = proxy.URL
	}
	if managedBinary != "" {
		if _, err := os.Stat(filepath.Join(work, "data", "valkey", ".threadify-managed")); err != nil {
			t.Fatalf("managed Valkey store missing: %v", err)
		}
	}
	fixtureTimeout := 150 * time.Second
	if os.Getenv("THREADIFY_HARNEST_TEST_PYTHON") != "" {
		fixtureTimeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), fixtureTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	account, keyID := uuid.NewString(), uuid.NewString()
	apiKey := "tf_" + uuid.NewString()
	hash := sha256.Sum256([]byte(apiKey))
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO companies(id,name) VALUES($1,'Standalone test') ON CONFLICT(id) DO NOTHING", []any{company}},
		{"INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,'Test service')", []any{account, company}},
		{"INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix) VALUES($1,$2,$3,$4,'tf_')", []any{keyID, account, company, hex.EncodeToString(hash[:])}},
		{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','owner',$1)", []any{account}},
		{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','admin',$1)", []any{account}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	// Opt-in workloads reuse the isolated binary/Registry fixture.
	output := os.Getenv("THREADIFY_PERF_OUTPUT")
	profileOutput := os.Getenv("THREADIFY_PROFILE_WORKLOAD_OUTPUT")
	if profileOutput != "" {
		output = profileOutput
	}
	if output != "" {
		script, err := sdkSmokeScript("performance-wait.mjs")
		if err != nil {
			t.Fatal(err)
		}
		sdkDir := filepath.Dir(filepath.Dir(script))
		if profileOutput != "" {
			script, err = filepath.Abs("../../scripts/entity-profile-workload.mjs")
			if err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("node", script, apiURL, apiKey, output)
		cmd.Env = append(os.Environ(), "THREADIFY_PROFILE_SDK="+sdkDir)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("SDK performance run: %v", err)
		}
		if stopJoiner != nil {
			stopJoiner()
		}
		stop()
		return
	}
	verifyGherkinAfterRestart := prepareGherkinSmoke(t, apiURL, apiKey, pool)
	verifyOTelAfterRestart := prepareOTelCorrelationSmoke(t, apiURL, apiKey, valkeyAddr, pool)
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(apiURL, "http")+"/threads", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	send := func(action string, payload map[string]any) map[string]any {
		t.Helper()
		payload["action"] = action
		if err := ws.WriteJSON(payload); err != nil {
			t.Fatal(err)
		}
		ws.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			var response map[string]any
			if err := ws.ReadJSON(&response); err != nil {
				t.Fatal(err)
			}
			if response["action"] != action {
				continue
			}
			if response["status"] != "success" {
				t.Fatalf("%s failed: %v", action, response)
			}
			return response
		}
	}
	send("connect", map[string]any{"apiKey": apiKey, "serviceName": "standalone-test"})
	var ids []string
	for i := 0; i < 5; i++ {
		response := send("startThread", map[string]any{"label": "standalone smoke", "role": "owner"})
		ids = append(ids, response["threadId"].(string))
	}
	// No credit account is needed, and bandwidth counters must survive restart.
	var creditRows, usageBefore int64
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM credit_accounts WHERE company_id=$1", company).Scan(&creditRows); err != nil {
		t.Fatal(err)
	}
	if creditRows != 0 {
		t.Fatal("licensed runtime created a credit account")
	}
	if err := pool.QueryRow(ctx, "SELECT COALESCE(sum(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.input.bytes'").Scan(&usageBefore); err != nil {
		t.Fatal(err)
	}
	if usageBefore <= 0 {
		t.Fatal("input bandwidth was not metered")
	}
	// The real-handler run must exercise the periodic signed heartbeat and usage
	// acknowledgement, not just the startup handshake. Fixture-only runs stay quick.
	if os.Getenv("THREADIFY_SMOKE_REGISTRY_URL") != "" {
		flushDeadline := time.Now().Add(80 * time.Second)
		for {
			var pending int64
			if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM threadify_registry_outbox").Scan(&pending); err != nil {
				t.Fatal(err)
			}
			if pending == 0 {
				break
			}
			if time.Now().After(flushDeadline) {
				t.Fatal("Registry did not acknowledge the signed usage outbox")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// Leave the authenticated websocket open: shutdown must close/join it itself.
	if stopJoiner != nil {
		stopJoiner()
	}
	stop()
	stop = start(configPath, baseURL)
	if shared {
		stopJoiner = start(joinPath, joinURL)
	}
	verifyGherkinAfterRestart()
	verifyOTelAfterRestart()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM threads WHERE id = ANY($1)", ids).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == len(ids) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("persisted %d/%d accepted threads after restart", count, len(ids))
		}
		time.Sleep(100 * time.Millisecond)
	}
	var usageAfter int64
	if err := pool.QueryRow(ctx, "SELECT COALESCE(sum(count),0) FROM threadify_registry_usage WHERE bucket_seconds>1 AND metric='threadify.input.bytes'").Scan(&usageAfter); err != nil {
		t.Fatal(err)
	}
	if usageAfter < usageBefore {
		t.Fatal("restart reset bandwidth accounting")
	}
	response, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("metrics unavailable")
	}
	if stopJoiner != nil {
		stopJoiner()
	}
	stop()
}
