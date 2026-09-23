package valkey

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt-in real Lua regression: THREADIFY_TEST_DOCKER_LUA=1 go test ./internal/repository/valkey -run TestTerminalLifecycleLua
// Uses an isolated disposable container with no host ports or external network.
func TestTerminalLifecycleLua(t *testing.T) {
	if os.Getenv("THREADIFY_TEST_DOCKER_LUA") != "1" {
		t.Skip("set THREADIFY_TEST_DOCKER_LUA=1 to run isolated Valkey Lua regressions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	docker := func(args ...string) []byte {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		require.NoError(t, err, string(out))
		return out
	}
	image := os.Getenv("THREADIFY_TEST_VALKEY_IMAGE")
	if image == "" {
		image = "valkey/valkey:9.0.6-alpine3.24"
	}
	id := strings.TrimSpace(string(docker("run", "--rm", "-d", "--network", "none", image, "valkey-server", "--port", "0", "--unixsocket", "/tmp/test.sock", "--save", "", "--appendonly", "no")))
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = exec.CommandContext(cleanup, "docker", "rm", "-f", id).Run()
	})
	cli := func(args ...string) string {
		t.Helper()
		return string(docker(append([]string{"exec", id, "valkey-cli", "-s", "/tmp/test.sock", "--json"}, args...)...))
	}
	require.Eventually(t, func() bool {
		out, err := exec.CommandContext(ctx, "docker", "exec", id, "valkey-cli", "-s", "/tmp/test.sock", "PING").CombinedOutput()
		return err == nil && strings.Contains(string(out), "PONG")
	}, 5*time.Second, 50*time.Millisecond)
	load := func(name string) string {
		t.Helper()
		b, err := os.ReadFile("lua/" + name + ".lua")
		require.NoError(t, err)
		return string(b)
	}
	hash, validation := load("update_thread_hash"), load("validate_and_update_step_state")
	key, meta, step, history, violations := "thread:test", "thread:test:meta", "thread:test:steps:charge:turn", "thread:test:current_steps", "thread:test:violations"
	validate := func() string {
		return cli("EVAL", validation, "4", meta, history, step, violations, "charge:turn", "step-id", "success", "2026-09-22T00:00:00Z", "[]", "false", "0", "", "", "false", "3600", "test", "turn", "owner", "", "{}")
	}
	snapshot := func() string {
		return cli("EVAL", "local keys=redis.call('KEYS','thread:test*');table.sort(keys);local data={};for _,key in ipairs(keys) do table.insert(data,key);table.insert(data,redis.call('DUMP',key));end;return data", "0")
	}
	for _, status := range []string{"completed", "cancelled", "closed", "failed"} {
		for _, storedIn := range []string{"meta", "json"} {
			t.Run(status+"/"+storedIn, func(t *testing.T) {
				cli("FLUSHDB")
				js, ms := "active", "active"
				if storedIn == "meta" {
					ms = status
				} else {
					js = status
				}
				data, err := json.Marshal(map[string]any{"status": js, "lastHash": "old", "refs": map[string]string{"threadify.thread_key": "session"}})
				require.NoError(t, err)
				cli("SET", key, string(data))
				cli("HSET", meta, "status", ms)
				cli("HSET", step, "status", "pending", "retryCount", "0")
				before := snapshot()
				require.Contains(t, cli("EVAL", hash, "1", key, "old", "new", "v1"), "Cannot add steps")
				require.Equal(t, before, snapshot(), "rejected admission changed hash")
				require.Contains(t, cli("EVAL", addThreadRefsScript, "2", meta, key, "3600", "customer", "new"), "Cannot add refs")
				require.Equal(t, before, snapshot(), "rejected refs changed state")
				require.Contains(t, validate(), "thread_already_terminal")
				require.Equal(t, before, snapshot(), "rejected validation changed state/history/grants")
				cli("EVAL", checkAndUpdateThreadStatusScript, "2", meta, key, "completed", "2026-09-22T00:00:00Z", "3600")
				require.Equal(t, before, snapshot(), "completion overwrote an existing terminal status")
			})
		}
	}
	t.Run("active writes succeed", func(t *testing.T) {
		cli("FLUSHDB")
		cli("SET", key, `{"status":"active","lastHash":"old","refs":{"threadify.thread_key":"session"}}`)
		cli("HSET", meta, "status", "active")
		require.Contains(t, cli("EVAL", hash, "1", key, "old", "new", "v1"), "1")
		require.Contains(t, cli("EVAL", addThreadRefsScript, "2", meta, key, "3600", "customer", "new"), "1")
		require.Contains(t, cli("HGET", meta, "refs:customer"), "new")
		require.NotContains(t, validate(), "thread_already_terminal")
		require.Contains(t, cli("HGET", step, "status"), "success")
	})
}
