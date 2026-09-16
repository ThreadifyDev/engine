package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDashboardRoutesAndAPIIsolation(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><html>dashboard</html>")}, "assets/app-abc123.js": {Data: []byte("console.log('dashboard')")}}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Path", r.URL.Path)
		w.WriteHeader(418)
	})
	handler := serve(files, api)
	for _, path := range []string{"/", "/login", "/cli-login", "/u/threads/order-123?tab=steps", "/u/profiles/customer/name", "/auth/forgot-password"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 || !strings.Contains(r.Body.String(), "dashboard") || r.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d %s", path, r.Code, r.Body.String())
		}
	}
	for _, path := range []string{"/auth/session", "/auth/managed/start", "/api/user/profile", "/graphql", "/v1/contracts", "/v1/missing", "/threads", "/sse", "/health", "/metrics", "/missing"} {
		r := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Upgrade", "websocket")
		handler.ServeHTTP(r, req)
		if r.Code != 418 || r.Header().Get("X-API-Path") != path {
			t.Fatalf("API route intercepted: %s", path)
		}
	}
	for _, path := range []string{"/assets/missing.js", "/assets/../index.html", "/assets/%2e%2e/index.html"} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 404 {
			t.Fatalf("missing/unsafe asset %s: %d", path, r.Code)
		}
	}
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("HEAD", "/assets/app-abc123.js", nil))
	if r.Code != 200 || r.Body.Len() != 0 || !strings.Contains(r.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("asset HEAD/cache handling failed")
	}
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("POST", "/login", strings.NewReader("secret")))
	if r.Code != 405 {
		t.Fatal("UI accepted a mutation")
	}
	r = httptest.NewRecorder()
	serve(fstest.MapFS{}, api).ServeHTTP(r, httptest.NewRequest("GET", "/login", nil))
	if r.Code != 503 {
		t.Fatal("development build silently served an empty UI")
	}
}
