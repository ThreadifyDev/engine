package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"threadify-go/shared/testutil/registryfixture"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/tests/internal/testenv"
)

// A live target is explicit. Otherwise launch the supplied release binary with
// fresh PostgreSQL/Valkey containers and embedded NATS; never use developer DBs.
func standaloneDirectory(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("THREADIFY_LIVE_DIR"); dir != "" {
		return dir
	}
	binary := os.Getenv("THREADIFY_E2E_BINARY")
	if binary == "" {
		t.Skip("run make test-e2e, or set THREADIFY_LIVE_DIR for an existing local instance")
	}
	// Disposable runs own their database, broker, ports, and output directory.
	// Keep live targets and explicitly shared evidence files sequential.
	if os.Getenv("THREADIFY_E2E_EVIDENCE_FILE") == "" {
		t.Parallel()
	}
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	_, err = os.Stat(binary)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pg, err := testenv.StartPostgresContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, pg.Terminate(ctx))
	})
	vk, err := testenv.StartValkeyContainer(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		require.NoError(t, vk.Terminate(ctx))
	})
	dir := t.TempDir()
	registryServer := registryfixture.New(t, "8bf9099d-2ff9-4d88-a2eb-acb114679909")
	v := viper.New()
	v.SetConfigFile("../../config/config.selfhost.yaml")
	require.NoError(t, v.ReadInConfig())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	httpPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	for key, value := range map[string]any{
		"registry.url": registryServer.URL, "registry.license_key": registryfixture.License, "registry.company_id": "8bf9099d-2ff9-4d88-a2eb-acb114679909",
		"server.host": "127.0.0.1", "server.port": httpPort, "postgres.url": pg.ConnectionString,
		"redis.url":    vk.URI,
		"runtime_mode": "combined", "nats.mode": "embedded", "nats.store_dir": filepath.Join(dir, "jetstream"),
		"jwks.url": "http://127.0.0.1:1/unused-jwks", "supabase.url": "",
		"security.hash_chain_secrets.v1":   "isolated-e2e-fixture-secret",
		"rate_limit.ip_rate_limit_enabled": false,
	} {
		v.Set(key, value)
	}
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, v.WriteConfigAs(configPath))
	logFile, err := os.OpenFile(filepath.Join(dir, "engine.log"), os.O_CREATE|os.O_WRONLY, 0600)
	require.NoError(t, err)
	cmd := exec.Command(binary, "--config", configPath)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GO_ENV=production"}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("binary did not shut down within 30 seconds")
		}
		logFile.Close()
		if t.Failed() {
			data, _ := os.ReadFile(filepath.Join(dir, "engine.log"))
			t.Logf("engine logs:\n%s", data)
		}
	})
	client := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(httpPort) + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		var health map[string]string
		if json.NewDecoder(resp.Body).Decode(&health) != nil {
			return false
		}
		return resp.StatusCode == 200 && health["nats"] == "ok" && health["persistence"] == "ok"
	}, 30*time.Second, 100*time.Millisecond, "standalone binary did not become ready")
	return dir
}
