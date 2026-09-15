package managedvalkey

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/domain"
	valkeyrepo "github.com/threadify/engine/internal/repository/valkey"
	"go.uber.org/zap"
)

func options(t *testing.T) Options {
	t.Helper()
	binary := os.Getenv("THREADIFY_TEST_VALKEY_BINARY")
	if binary == "" {
		t.Skip("set THREADIFY_TEST_VALKEY_BINARY to a real disposable valkey-server")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return Options{Mode: "managed", Host: "127.0.0.1", Bind: "127.0.0.1", Port: port, Password: "test-only \"quoted\" \\ password", StoreDir: filepath.Join(t.TempDir(), "store with spaces"), BinaryPath: binary, StartupTimeout: 5 * time.Second}
}
func start(t *testing.T, cfg Options) *Runtime {
	t.Helper()
	r, err := Start(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.Close(ctx)
	})
	return r
}
func client(t *testing.T, cfg Options) *redis.Client {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)), Password: cfg.Password, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	t.Cleanup(func() { c.Close() })
	return c
}

func TestSharedContractClaimsAndCrashRecovery(t *testing.T) {
	cfg := options(t)
	owner := start(t, cfg)
	ctx := context.Background()
	a, b := client(t, cfg), client(t, cfg)
	joinCfg := cfg
	joinCfg.Mode = "external"
	joinCfg.BinaryPath = "/missing"
	joiner := start(t, joinCfg)
	require.NoError(t, joiner.Close(ctx))
	require.NoError(t, b.Ping(ctx).Err())
	thread := uuid.NewString()
	require.NoError(t, a.HSet(ctx, "thread:"+thread+":meta", "status", "active").Err())
	state := valkeyrepo.NewStepStateRepository(&database.ValkeyService{Client: a}, 604800, zap.NewNop())
	require.NoError(t, state.LoadScripts(ctx))
	r, err := state.ValidateAndUpdateStepState(ctx, domain.ValidateStepParams{ThreadID: thread, StepID: uuid.NewString(), StepName: "approval", IdempotencyKey: uuid.NewString(), Status: "success", Timestamp: time.Now().Format(time.RFC3339Nano), Actor: "owner", RawContext: `{}`})
	require.NoError(t, err)
	require.False(t, r.HasCriticalViolation)
	graph := &domain.ContractGraph{Graph: domain.Graph{Nodes: map[string]domain.GraphNode{"approval": {ID: "approval"}, "charge": {ID: "charge", DependsOn: []string{"approval"}, FreshDependsOn: []string{"approval"}}}, EntryPoints: []string{"approval"}}}
	// Two independent Engine repositories contend for the same fresh prerequisite.
	repos := []*valkeyrepo.WaitRepository{valkeyrepo.NewWaitRepository(a), valkeyrepo.NewWaitRepository(b)}
	var wg sync.WaitGroup
	allowed := make(chan string, 32)
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := uuid.NewString()
			got, err := repos[i%2].Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "owner", graph)
			if err != nil {
				failures <- err
				return
			}
			if got.Decision == "allowed" {
				allowed <- id
			}
		}(i)
	}
	wg.Wait()
	close(allowed)
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Len(t, allowed, 1)
	id := <-allowed
	require.NoError(t, owner.cmd.Process.Kill())
	<-owner.done
	require.False(t, owner.IsHealthy())
	owner = start(t, cfg)
	got, err := repos[1].Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "owner", graph)
	require.NoError(t, err)
	require.Equal(t, "allowed", got.Decision)
	got, err = repos[0].Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: uuid.NewString()}, "owner", graph)
	require.NoError(t, err)
	require.Equal(t, "pending", got.Decision)
	require.NoError(t, owner.Close(ctx))
	// Graceful restart also retains the acknowledged grant.
	start(t, cfg)
	got, err = repos[1].Claim(ctx, domain.WaitRequest{ThreadID: thread, StepName: "charge", InvocationID: id}, "owner", graph)
	require.NoError(t, err)
	require.Equal(t, "allowed", got.Decision)
}

func TestStartupFailuresDoNotAttachOrEraseState(t *testing.T) {
	cfg := options(t)
	owner := start(t, cfg)
	ctx := context.Background()
	c := client(t, cfg)
	require.NoError(t, c.Set(ctx, "keep", "value", 0).Err())
	_, err := Start(ctx, cfg)
	require.ErrorContains(t, err, "already owned")
	other := cfg
	other.StoreDir = filepath.Join(t.TempDir(), "collision")
	_, err = Start(ctx, other)
	require.ErrorContains(t, err, "exited during startup")
	require.True(t, owner.IsHealthy())
	require.Equal(t, "value", c.Get(ctx, "keep").Val())
	bad := cfg
	bad.Password = "wrong"
	require.Error(t, client(t, bad).Ping(ctx).Err())
	require.NoError(t, owner.Close(ctx))
	require.NoError(t, os.Remove(filepath.Join(cfg.StoreDir, "appendonlydir", "appendonly.aof.manifest")))
	_, err = Start(ctx, cfg)
	require.ErrorContains(t, err, "persistence is missing")
}

func TestInvalidOptionsAndCorruptPersistence(t *testing.T) {
	cfg := options(t)
	ctx := context.Background()
	bad := cfg
	bad.BinaryPath = "/not/a/binary"
	_, err := Start(ctx, bad)
	require.ErrorContains(t, err, "missing")
	bad = cfg
	bad.Bind = "0.0.0.0"
	bad.Password = ""
	_, err = Start(ctx, bad)
	require.ErrorContains(t, err, "password")
	bad = cfg
	bad.Bind = "127.0.0.1\nport 1"
	_, err = Start(ctx, bad)
	require.ErrorContains(t, err, "single IP")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = Start(cancelled, cfg)
	require.ErrorIs(t, err, context.Canceled)
	r := start(t, cfg)
	c := client(t, cfg)
	require.NoError(t, c.Set(ctx, "persist", "yes", 0).Err())
	require.NoError(t, r.Close(ctx))
	files, err := filepath.Glob(filepath.Join(cfg.StoreDir, "appendonlydir", "*.incr.aof"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	f, err := os.OpenFile(files[0], os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.WriteString("*3\r\n$3\r\nSET\r\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	_, err = Start(ctx, cfg)
	require.ErrorContains(t, err, "exited during startup")
}

func TestReadinessTimeoutCleansUp(t *testing.T) {
	cfg := options(t)
	// A real server starts, but the owner's deliberately wrong connection port
	// (via a wrong loopback IP) can never pass readiness. It must be reaped.
	cfg.Host = "127.0.0.2"
	cfg.StartupTimeout = 250 * time.Millisecond
	_, err := Start(context.Background(), cfg)
	require.ErrorContains(t, err, "not ready")
	// Startup created persistence before timing out; restoring its marker is an
	// explicit test recovery step, not something production startup does silently.
	require.NoError(t, writeMarker(filepath.Join(cfg.StoreDir, ".threadify-managed")))
	cfg.Host = "127.0.0.1"
	cfg.StartupTimeout = 5 * time.Second
	start(t, cfg)
}

// The helper is a real owner process so killing it exercises the inherited
// store lock, rather than just a goroutine in the same process as Valkey.
func TestOwnerProcessHelper(t *testing.T) {
	encoded := os.Getenv("THREADIFY_OWNER_HELPER")
	if encoded == "" {
		t.Skip("subprocess helper")
	}
	var cfg Options
	require.NoError(t, json.Unmarshal([]byte(encoded), &cfg))
	r, err := Start(context.Background(), cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.StoreDir, "test-child-pid"), []byte(strconv.Itoa(r.cmd.Process.Pid)), 0600))
	select {}
}

func TestOwnerCrashKeepsOrphanStoreLocked(t *testing.T) {
	cfg := options(t)
	encoded, err := json.Marshal(cfg)
	require.NoError(t, err)
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerProcessHelper$")
	cmd.Env = append(os.Environ(), "THREADIFY_OWNER_HELPER="+string(encoded))
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var pid int
	require.Eventually(t, func() bool {
		data, err := os.ReadFile(filepath.Join(cfg.StoreDir, "test-child-pid"))
		if err != nil {
			return false
		}
		pid, err = strconv.Atoi(string(data))
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)
	child, err := os.FindProcess(pid)
	require.NoError(t, err)
	t.Cleanup(func() { _ = child.Kill() })
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait()
	_, err = Start(context.Background(), cfg)
	require.ErrorContains(t, err, "already owned")
	require.NoError(t, child.Signal(syscall.SIGTERM))
	require.Eventually(t, func() bool {
		f, err := os.OpenFile(filepath.Join(cfg.StoreDir, ".threadify.lock"), os.O_RDWR, 0600)
		if err != nil {
			return false
		}
		defer f.Close()
		return lockFile(f) == nil
	}, 5*time.Second, 20*time.Millisecond)
	start(t, cfg)
}
