package agentbundle

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Status struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

// Manager owns one isolated agent process. Constructing it never installs or starts anything.
type Manager struct {
	options     Options
	mu          sync.RWMutex
	status      Status
	endpoint    string
	transport   *http.Transport
	cancel      context.CancelFunc
	done        chan struct{}
	startOnce   sync.Once
	diagnostics string
}

func New(options Options) (*Manager, error) {
	if options.EngineURL == "" {
		return nil, errors.New("local agent requires an Engine URL")
	}
	return &Manager{options: options, status: Status{State: "starting", Message: "Starting Threadify Agent"}, done: make(chan struct{})}, nil
}
func (m *Manager) DiagnosticsPath() string { m.mu.RLock(); defer m.mu.RUnlock(); return m.diagnostics }

func (m *Manager) Status() Status { m.mu.RLock(); defer m.mu.RUnlock(); return m.status }
func (m *Manager) Endpoint() (string, http.RoundTripper) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.status.State != "ready" {
		return "", nil
	}
	return m.endpoint, m.transport
}
func (m *Manager) set(state, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = Status{state, message}
	if state != "ready" {
		m.endpoint = ""
	}
}

// Start runs installation and startup in the background, independently of Engine readiness.
func (m *Manager) Start(parent context.Context) {
	m.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(parent)
		m.mu.Lock()
		m.cancel = cancel
		m.mu.Unlock()
		go func() { defer close(m.done); m.run(ctx) }()
	})
}
func (m *Manager) Close() error {
	m.mu.RLock()
	cancel := m.cancel
	m.mu.RUnlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-m.done
	return nil
}
func (m *Manager) run(ctx context.Context) {
	root, err := InstallRuntime(ctx, m.options)
	if err != nil {
		m.set("unavailable", err.Error())
		return
	}
	entry, err := runtimeSpec()
	if err != nil {
		m.set("unavailable", err.Error())
		return
	}
	work, err := os.MkdirTemp("", "threadify-agent-")
	if err != nil {
		m.set("unavailable", "Unable to prepare Threadify Agent")
		return
	}
	defer os.RemoveAll(work)
	if err = extractAgent(ctx, work); err != nil {
		m.set("unavailable", "Unable to unpack bundled Threadify Agent")
		return
	}
	transport, err := localTLS(work)
	if err != nil {
		m.set("unavailable", "Unable to secure local agent connection")
		return
	}
	defer transport.CloseIdleConnections()
	for attempt := 0; attempt < 3; attempt++ {
		if ctx.Err() != nil {
			m.set("unavailable", "Threadify Agent stopped")
			return
		}
		m.set("starting", "Starting Threadify Agent")
		err = m.serve(ctx, root, entry, work, transport)
		if ctx.Err() != nil {
			m.set("unavailable", "Threadify Agent stopped")
			return
		}
		m.set("unavailable", fmt.Sprintf("Threadify Agent stopped: %v", err))
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}
}
func (m *Manager) serve(ctx context.Context, root string, entry runtimeEntry, work string, transport *http.Transport) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	endpoint := "https://127.0.0.1:" + strconv.Itoa(port)
	command := exec.Command(filepath.Join(root, filepath.FromSlash(entry.Python)), "-I", filepath.Join(work, "launch.py"), filepath.Join(root, "packages"), work, strconv.Itoa(port), filepath.Join(work, "local.crt"), filepath.Join(work, "local.key"))
	command.Dir = work
	command.Env = childEnvironment(m.options)
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	// Keep bounded diagnostics private to this installation, separate from shared runtime files.
	cache, err := cacheRoot(m.options)
	if err != nil {
		stdin.Close()
		return err
	}
	logs := filepath.Join(cache, "logs")
	if err = os.MkdirAll(logs, 0700); err != nil {
		stdin.Close()
		return err
	}
	log, err := os.CreateTemp(logs, "agent-*.log")
	if err != nil {
		stdin.Close()
		return err
	}
	defer log.Close()
	m.mu.Lock()
	m.diagnostics = log.Name()
	m.mu.Unlock()
	output := &boundedLog{writer: log, remaining: 1 << 20}
	command.Stdout = output
	command.Stderr = output
	if err = command.Start(); err != nil {
		stdin.Close()
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	defer func() { stdin.Close(); command.Process.Kill() }()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ready := false
	for {
		select {
		case <-ctx.Done():
			stdin.Close()
			select {
			case <-exited:
			case <-time.After(5 * time.Second):
				command.Process.Kill()
				<-exited
			}
			return ctx.Err()
		case err := <-exited:
			return fmt.Errorf("agent process exited: %v", err)
		case <-deadline.C:
			command.Process.Kill()
			<-exited
			return errors.New("agent startup timed out")
		case <-tick.C:
			if ready {
				continue
			}
			request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/healthz", nil)
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				ready = true
				deadline.Stop()
				m.mu.Lock()
				m.endpoint = endpoint
				m.transport = transport
				m.status = Status{State: "ready"}
				m.mu.Unlock()
			}
		}
	}
}
func childEnvironment(options Options) []string {
	overrides := map[string]string{
		"THREADIFY_AUTH_MODE":                                "engine",
		"THREADIFY_ENGINE_URL":                               strings.TrimRight(options.EngineURL, "/"),
		"THREADIFY_GRAPHQL_URL":                              strings.TrimRight(options.EngineURL, "/") + "/graphql",
		"PYTHONDONTWRITEBYTECODE":                            "1",
		"LITELLM_LOCAL_MODEL_COST_MAP":                       "True",
		"OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT": "NO_CONTENT",
		"ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS":               "false",
		"NO_PROXY":                                           "127.0.0.1,localhost",
		"no_proxy":                                           "127.0.0.1,localhost",
	}
	if options.ConfigPath != "" {
		overrides["THREADIFY_CONFIG_PATH"] = options.ConfigPath
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if _, ok := overrides[key]; !ok && key != "HARNEST_DATABASE_URL" && key != "THREADIFY_API_KEY" && key != "PYTHONPATH" && key != "PYTHONHOME" {
			env = append(env, value)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}

// A per-process certificate pins the child identity before any user credential is sent.
func localTLS(directory string) (*http.Transport, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Threadify local agent"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().AddDate(10, 0, 0), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(directory, "local.crt"), certPEM, 0600); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(directory, "local.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)
	return &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 120 * time.Second, IdleConnTimeout: 90 * time.Second}, nil
}

// boundedLog bounds diagnostic storage without blocking or exposing output to Engine clients.
type boundedLog struct {
	mu        sync.Mutex
	writer    io.Writer
	remaining int
}

func (w *boundedLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	length := len(p)
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	if len(p) > 0 {
		n, err := w.writer.Write(p)
		w.remaining -= n
		if err != nil {
			return n, err
		}
	}
	return length, nil
}
