package managedvalkey

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	endpoint := &url.URL{Scheme: "redis", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), User: url.UserPassword("default", "test-only \"quoted\" \\ password"), Path: "/0"}
	return Options{Mode: "managed", URL: endpoint.String(), Bind: "127.0.0.1", StoreDir: filepath.Join(t.TempDir(), "store with spaces"), BinaryPath: binary, StartupTimeout: 5 * time.Second}
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
	opts, err := redis.ParseURL(cfg.URL)
	require.NoError(t, err)
	opts.MaxRetries = -1
	opts.DialTimeout, opts.ReadTimeout, opts.WriteTimeout = time.Second, time.Second, time.Second
	c := redis.NewClient(opts)
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
	u, err := url.Parse(bad.URL)
	require.NoError(t, err)
	u.User = url.UserPassword("default", "wrong")
	bad.URL = u.String()
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
	u, err := url.Parse(bad.URL)
	require.NoError(t, err)
	u.User = nil
	bad.URL = u.String()
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
	cfg.URL = strings.Replace(cfg.URL, "127.0.0.1", "127.0.0.2", 1)
	cfg.StartupTimeout = 250 * time.Millisecond
	_, err := Start(context.Background(), cfg)
	require.ErrorContains(t, err, "not ready")
	// Startup created persistence before timing out; restoring its marker is an
	// explicit test recovery step, not something production startup does silently.
	require.NoError(t, writeMarker(filepath.Join(cfg.StoreDir, ".threadify-managed")))
	cfg.URL = strings.Replace(cfg.URL, "127.0.0.2", "127.0.0.1", 1)
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

func urlDatabaseClient(raw string) (*database.ValkeyService, error) {
	return database.NewValkeyService(raw, 1, 1, 1, -1, 1000, 1000, 1000, 1000, 1000)
}

func TestURLDatabaseAuthenticationAndTLS(t *testing.T) {
	cfg := options(t)
	start(t, cfg)
	ctx := context.Background()
	owner := client(t, cfg)
	endpoint, err := url.Parse(cfg.URL)
	require.NoError(t, err)
	endpoint.Path = "/2"
	connected, err := urlDatabaseClient(endpoint.String())
	require.NoError(t, err)
	defer connected.Close()
	require.NoError(t, connected.Client.Set(ctx, "url-db-check", "selected-db", 0).Err())
	require.ErrorIs(t, owner.Get(ctx, "url-db-check").Err(), redis.Nil)
	require.Equal(t, "selected-db", connected.Client.Get(ctx, "url-db-check").Val())
	// ACL credentials are independent from the default requirepass user.
	password := "acl@:/#% password"
	require.NoError(t, owner.Do(ctx, "ACL", "SETUSER", "url-user", "on", ">"+password, "~*", "+@all").Err())
	endpoint.User = url.UserPassword("url-user", password)
	acl, err := urlDatabaseClient(endpoint.String())
	require.NoError(t, err)
	defer acl.Close()
	require.Equal(t, "selected-db", acl.Client.Get(ctx, "url-db-check").Val())
	wrong := *endpoint
	wrong.User = url.UserPassword("url-user", "incorrect")
	_, err = urlDatabaseClient(wrong.String())
	require.Error(t, err)
	require.NotContains(t, err.Error(), password)

	// Put a verified TLS listener in front of the disposable plaintext Valkey.
	certificates := httptest.NewTLSServer(http.NotFoundHandler())
	tlsConfig := &tls.Config{Certificates: certificates.TLS.Certificates, MinVersion: tls.VersionTLS12}
	certificate := certificates.Certificate()
	certificates.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	require.NoError(t, err)
	defer listener.Close()
	backend := endpoint.Host
	go func() {
		for {
			incoming, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer incoming.Close()
				upstream, err := net.Dial("tcp", backend)
				if err != nil {
					return
				}
				defer upstream.Close()
				go io.Copy(upstream, incoming)
				io.Copy(incoming, upstream)
			}()
		}
	}()
	endpoint.Scheme = "rediss"
	endpoint.Host = listener.Addr().String()
	_, err = urlDatabaseClient(endpoint.String())
	var untrusted x509.UnknownAuthorityError
	require.ErrorAs(t, err, &untrusted, "rediss must verify the server certificate")
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0600))
	// Isolate test trust roots in a child process while exercising the public
	// database constructor with certificate verification enabled on all platforms.
	command := exec.Command(os.Args[0], "-test.run=^TestTLSURLClientHelper$")
	command.Env = append(os.Environ(), "THREADIFY_TLS_TEST_URL="+endpoint.String(), "THREADIFY_TLS_TEST_CA_FILE="+caFile, "GODEBUG=x509usefallbackroots=1")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestTLSURLClientHelper(t *testing.T) {
	raw := os.Getenv("THREADIFY_TLS_TEST_URL")
	if raw == "" {
		t.Skip("TLS subprocess helper")
	}
	pemBytes, err := os.ReadFile(os.Getenv("THREADIFY_TLS_TEST_CA_FILE"))
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(pemBytes))
	x509.SetFallbackRoots(roots)
	connected, err := urlDatabaseClient(raw)
	require.NoError(t, err)
	defer connected.Close()
	require.Equal(t, "selected-db", connected.Client.Get(context.Background(), "url-db-check").Val())
}
