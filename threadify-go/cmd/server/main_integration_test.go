package main

import (
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

// This test uses only explicitly supplied disposable services, never developer defaults.
// Build bin/threadify first and set all three THREADIFY_SMOKE_* variables to opt in.
func TestStandaloneBinaryPersistenceAndRestart(t *testing.T) {
	binary, pgURL, valkeyAddr := os.Getenv("THREADIFY_SMOKE_BINARY"), os.Getenv("THREADIFY_SMOKE_POSTGRES_URL"), os.Getenv("THREADIFY_SMOKE_VALKEY_ADDR")
	if binary == "" || pgURL == "" || valkeyAddr == "" {
		t.Skip("requires a built binary and explicitly supplied disposable PostgreSQL and Valkey")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
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
	host, port, err := net.SplitHostPort(valkeyAddr)
	if err != nil {
		t.Fatal(err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	for key, value := range map[string]any{
		"server.host": "127.0.0.1", "server.port": httpPort, "postgres.url": pgURL,
		"redis.host": host, "redis.port": portNumber, "redis.password": "",
		"nats.mode": "embedded", "nats.store_dir": filepath.Join(work, "jetstream"),
		"jwks.url": "http://127.0.0.1:1/unused-jwks", "supabase.url": "",
		"security.hash_chain_secrets.v1": "isolated-test-hash-chain-key-not-a-production-secret",
		"billing.provider":               "noop", "archiver.streams.block_timeout_ms": 100,
		"archiver.streams.step_state_flush_interval_ms": 100,
	} {
		v.Set(key, value)
	}
	configPath := filepath.Join(work, "config.yaml")
	if err := v.WriteConfigAs(configPath); err != nil {
		t.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	client := &http.Client{Timeout: time.Second}
	start := func() func() {
		t.Helper()
		logs, err := os.CreateTemp(work, "engine-*.log")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "--config", configPath)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GO_ENV=production"}
		cmd.Dir = work
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
				_ = cmd.Process.Kill()
				<-done
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
		if out, err := healthCmd.CombinedOutput(); err != nil {
			t.Fatalf("binary healthcheck: %v %s", err, out)
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
	stop := start()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	company, account, keyID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	apiKey := "tf_" + uuid.NewString()
	hash := sha256.Sum256([]byte(apiKey))
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO companies(id,name) VALUES($1,'Standalone test')", []any{company}},
		{"INSERT INTO service_accounts(id,company_id,name) VALUES($1,$2,'Test service')", []any{account, company}},
		{"INSERT INTO api_keys(id,service_account_id,company_id,key_hash,key_prefix) VALUES($1,$2,$3,$4,'tf_')", []any{keyID, account, company, hex.EncodeToString(hash[:])}},
		{"INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES($1,'service_account','owner',$1)", []any{account}},
		{"INSERT INTO credit_accounts(id,company_id,billing_cycle_start,credit_balance_millicents,rate_limit_tps,payload_limit_bytes) VALUES($1,$2,NOW(),10000000,60000,1048576)", []any{uuid.NewString(), company}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/threads", httpPort), nil)
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
	// Leave the authenticated websocket open: shutdown must close/join it itself.
	stop()
	stop = start()
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
	response, err := client.Get(baseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("metrics unavailable")
	}
	stop()
}
