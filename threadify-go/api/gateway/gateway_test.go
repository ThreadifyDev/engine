package gateway

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

const activeSnapshot = `{"status":"ok","account_id":"account-a","workspace_id":"workspace-a","entitlements":{"revision":"1"},"suspended":false}`
const chatBody = `{"model":"threadify-agent","messages":[{"role":"user","content":"hello"}],"stream":true,"tools":[{"type":"function","function":{"name":"get_page_context"}}]}`

func configFor(registry, upstream string) Config {
	return Config{RegistryURL: registry, UpstreamURL: upstream, Model: "threadify-agent", UpstreamModel: "qwen3.5:cloud", MaxConcurrent: 4, Timeout: 5 * time.Second}
}
func testGateway(t *testing.T, c Config) *Gateway {
	t.Helper()
	g, err := New(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	return g
}
func request(g *Gateway, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	if key != "" {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	return w
}

func TestStreamingMultiTenantAndCredentialIsolation(t *testing.T) {
	var verified, inference atomic.Int32
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/threadify/handshake" || r.Header.Get("X-Threadify-Installation-ID") != "test-installation" {
			t.Error("wrong registry request")
		}
		key := r.Header.Get("Authorization")
		if key != "Bearer tenant-one" && key != "Bearer tenant-two" {
			t.Error("wrong registry credential")
		}
		verified.Add(1)
		_, _ = io.WriteString(w, strings.ReplaceAll(activeSnapshot, "account-a", key))
	}))
	defer registry.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inference.Add(1)
		if r.URL.Path != "/ollama/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer provider-secret" {
			t.Error("incorrect upstream routing/auth")
		}
		for _, name := range []string{"Cookie", "X-Threadify-Installation-ID", "X-Forwarded-For", "X-Customer-Secret"} {
			if r.Header.Get(name) != "" {
				t.Error("leaked header", name)
			}
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "qwen3.5:cloud" || body["tools"] == nil || body["messages"] == nil {
			t.Error("lost tool or message payload")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Set-Cookie", "provider=secret")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"name\":\"get_page_context\"}}]}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	c := configFor(registry.URL, upstream.URL+"/ollama/v1")
	c.UpstreamKey = "provider-secret"
	server := httptest.NewServer(testGateway(t, c))
	defer server.Close()
	for _, tenant := range []string{"tenant-one", "tenant-two"} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/chat/completions", strings.NewReader(chatBody))
		req.Header.Set("Authorization", "Bearer "+tenant)
		req.Header.Set("X-Threadify-Installation-ID", "test-installation")
		for _, name := range []string{"Cookie", "X-Customer-Secret", "X-Forwarded-For"} {
			req.Header.Set(name, "private")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		line, err := bufio.NewReader(response.Body).ReadString('\n')
		if err != nil || !strings.Contains(line, "get_page_context") {
			t.Error("stream buffered or tool lost", err)
		}
		if response.Header.Get("Set-Cookie") != "" || response.Header.Get("Cache-Control") != "no-store" {
			t.Error("unsafe response headers")
		}
		response.Body.Close()
		cancel()
	}
	if verified.Load() != 2 || inference.Load() != 2 {
		t.Fatal("requests not independently authorized")
	}
}
func TestLicenseFailuresNeverReachModel(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"revoked", 401, "secret diagnostic", 401}, {"forbidden", 403, "", 403}, {"outage", 500, "secret", 503}, {"redirect", 302, "", 503},
		{"suspended", 200, strings.Replace(activeSnapshot, "false", "true", 1), 403}, {"malformed", 200, `{}`, 503},
		{"missing suspension state", 200, strings.Replace(activeSnapshot, `,"suspended":false`, "", 1), 503},
		{"trailing", 200, activeSnapshot + `{}`, 503}, {"oversized", 200, activeSnapshot + strings.Repeat(" ", 1<<20), 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer registry.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unauthorized model request") }))
			defer upstream.Close()
			result := request(testGateway(t, configFor(registry.URL, upstream.URL)), chatBody, "license")
			if result.Code != test.want || strings.Contains(result.Body.String(), "secret") {
				t.Fatalf("unsafe denial: %d %s", result.Code, result.Body.String())
			}
		})
	}
}
func TestRoutesLimitsAndModelAllowlist(t *testing.T) {
	var called atomic.Int32
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, activeSnapshot) }))
	defer registry.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(500)
		_, _ = io.WriteString(w, "private provider error")
	}))
	defer upstream.Close()
	g := testGateway(t, configFor(registry.URL, upstream.URL))
	for _, test := range []struct {
		body, key string
		status    int
	}{
		{chatBody, "", 401}, {`{`, "key", 400}, {`null`, "key", 400}, {`{"model":"arbitrary"}`, "key", 400},
		{strings.Repeat("x", maxRequestBytes+1), "key", 413}, {chatBody, "key", 502},
	} {
		result := request(g, test.body, test.key)
		if result.Code != test.status || strings.Contains(result.Body.String(), "private") {
			t.Fatal(result.Code, result.Body.String())
		}
	}
	if called.Load() != 1 {
		t.Fatal("invalid requests reached upstream")
	}
	for _, path := range []string{"/api/contracts", "/api/chat/ask", "/v1/responses", "/v1/chat/completions?target=bad"} {
		rec := httptest.NewRecorder()
		g.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
		if rec.Code != 404 {
			t.Fatal(path, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer key")
	g.ServeHTTP(rec, req)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "threadify-agent") || strings.Contains(rec.Body.String(), "qwen") {
		t.Fatal("wrong public models")
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 {
		t.Fatal("health requires license")
	}
}
func TestRevocationIsCheckedOnNextRequest(t *testing.T) {
	var count atomic.Int32
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if count.Add(1) > 1 {
			w.WriteHeader(403)
			return
		}
		_, _ = io.WriteString(w, activeSnapshot)
	}))
	defer registry.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer upstream.Close()
	g := testGateway(t, configFor(registry.URL, upstream.URL))
	if request(g, chatBody, "same-key").Code != 200 || request(g, chatBody, "same-key").Code != 403 {
		t.Fatal("revoked license retained access")
	}
}
func TestUpstreamTLSAndClientCertificate(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, activeSnapshot) }))
	defer registry.Close()
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"choices":[]}`) }))
	// Sign a test-only certificate with both server and client authentication EKUs.
	temporary := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := temporary.TLS.Certificates[0]
	template := temporary.Certificate()
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	der, err := x509.CreateCertificate(rand.Reader, template, template, template.PublicKey, cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	cert.Certificate = [][]byte{der}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	ca := x509.NewCertPool()
	ca.AddCert(parsed)
	temporary.Close()
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: ca, ClientAuth: tls.RequireAndVerifyClientCert}
	upstream.StartTLS()
	defer upstream.Close()
	dir := t.TempDir()
	certFile := filepath.Join(dir, "client.pem")
	keyFile := filepath.Join(dir, "client-key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	c := configFor(registry.URL, upstream.URL)
	c.CAFile = certFile
	c.CertFile = certFile
	c.KeyFile = keyFile
	if got := request(testGateway(t, c), chatBody, "key").Code; got != 200 {
		t.Fatal("mTLS request failed", got)
	}
	c.CertFile = ""
	c.KeyFile = ""
	if got := request(testGateway(t, c), chatBody, "key").Code; got != 502 {
		t.Fatal("missing certificate accepted", got)
	}
	c.CertFile = certFile
	c.KeyFile = keyFile
	c.CAFile = ""
	if got := request(testGateway(t, c), chatBody, "key").Code; got != 502 {
		t.Fatal("untrusted CA accepted", got)
	}
}
func TestConfigurationRejectsUnsafeEndpoints(t *testing.T) {
	for _, raw := range []string{"", "http://remote.example/v1", "https://user:secret@host/v1", "https://host/v1?key=secret", "https://host/#fragment", "file:///tmp/model"} {
		if _, err := New(configFor(raw, "http://127.0.0.1:11434/v1")); err == nil {
			t.Fatal("accepted unsafe URL", raw)
		}
	}
	t.Setenv("THREADIFY_AI_PORT", "-1")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("accepted invalid port")
	}
}

func TestTimeoutCancelsUpstreamAndReleasesCapacity(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, activeSnapshot) }))
	defer registry.Close()
	started, cancelled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	c := configFor(registry.URL, upstream.URL)
	c.MaxConcurrent = 1
	c.Timeout = 500 * time.Millisecond
	g := testGateway(t, c)
	finished := make(chan int, 1)
	go func() { finished <- request(g, chatBody, "key").Code }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not start")
	}
	if result := request(g, chatBody, "key"); result.Code != 429 || result.Header().Get("Retry-After") == "" {
		t.Fatal("missing overload protection")
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout did not cancel inference")
	}
	select {
	case status := <-finished:
		if status != 502 {
			t.Fatal(status)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}
	if request(g, chatBody, "").Code != 401 {
		t.Fatal("timed-out request retained capacity")
	}
}
