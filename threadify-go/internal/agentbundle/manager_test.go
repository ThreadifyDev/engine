package agentbundle

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalTLSPinsChild(t *testing.T) {
	dir := t.TempDir()
	transport, err := localTLS(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "local.crt"), filepath.Join(dir, "local.key"))
	if err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	trusted := httptest.NewUnstartedServer(handler)
	trusted.TLS = &tls.Config{Certificates: []tls.Certificate{pair}}
	trusted.StartTLS()
	defer trusted.Close()
	client := &http.Client{Transport: transport}
	response, err := client.Get(trusted.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	wrong := httptest.NewTLSServer(handler)
	defer wrong.Close()
	if response, err = client.Get(wrong.URL); err == nil {
		response.Body.Close()
		t.Fatal("trusted another local listener")
	}
}
func TestManagerFailureDoesNotExposeEndpoint(t *testing.T) {
	manager, err := New(Options{EngineURL: "http://127.0.0.1:1234", CacheDir: t.TempDir(), ArchivePath: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatal(err)
	}
	manager.Start(context.Background())
	defer manager.Close()
	select {
	case <-manager.done:
	case <-time.After(3 * time.Second):
		t.Fatal("manager did not report failure")
	}
	if manager.Status().State != "unavailable" {
		t.Fatal(manager.Status())
	}
	if endpoint, _ := manager.Endpoint(); endpoint != "" {
		t.Fatal(endpoint)
	}
}
func TestChildEnvironmentUsesEngineAuthority(t *testing.T) {
	t.Setenv("THREADIFY_ENGINE_URL", "http://wrong")
	t.Setenv("HARNEST_DATABASE_URL", "shared")
	t.Setenv("THREADIFY_API_KEY", "service")
	env := strings.Join(childEnvironment(Options{EngineURL: "http://127.0.0.1:1234", ConfigPath: "/config.yaml"}), "\n")
	if strings.Contains(env, "http://wrong") || strings.Contains(env, "HARNEST_DATABASE_URL=") || strings.Contains(env, "THREADIFY_API_KEY=") {
		t.Fatal("inherited shared authority")
	}
	if !strings.Contains(env, "THREADIFY_GRAPHQL_URL=http://127.0.0.1:1234/graphql") {
		t.Fatal("missing local GraphQL URL")
	}
}

// Opt-in real portable runtime smoke test; does not call a model or require a service key.
func TestPortableRuntimeLifecycle(t *testing.T) {
	archive := os.Getenv("THREADIFY_TEST_AGENT_ARCHIVE")
	if archive == "" {
		t.Skip("portable runtime archive not supplied")
	}
	manager, err := New(Options{EngineURL: "http://127.0.0.1:1234", CacheDir: t.TempDir(), ArchivePath: archive})
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
			t.Fatal(state.Message)
		}
		select {
		case <-deadline:
			t.Fatal("agent did not become ready")
		case <-time.After(200 * time.Millisecond):
		}
	}
	endpoint, transport := manager.Endpoint()
	response, err := (&http.Client{Transport: transport}).Get(endpoint + "/agent")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.StatusCode)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if manager.Status().State == "ready" {
		t.Fatal("agent still ready after shutdown")
	}
}
