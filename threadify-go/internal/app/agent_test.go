package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	sharedauth "threadify-go/shared/auth"
)

func TestAgentServiceURLMigration(t *testing.T) {
	t.Setenv("THREADIFY_HARNEST_URL", "http://legacy-agent:8090")
	t.Setenv("THREADIFY_AGENT_URL", "http://threadify-agent:8090")
	require.Equal(t, "http://threadify-agent:8090", agentServiceURL())
	t.Setenv("THREADIFY_AGENT_URL", "")
	require.Empty(t, agentServiceURL())
	w := httptest.NewRecorder()
	configuredAgentProxy(agentServiceURL(), nil).ServeHTTP(w, httptest.NewRequest("POST", "/api/harnest/responses", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "THREADIFY_AGENT_URL")
	require.NotContains(t, w.Body.String(), "HARNEST")
	require.NoError(t, os.Unsetenv("THREADIFY_AGENT_URL"))
	require.Equal(t, "http://legacy-agent:8090", agentServiceURL())
}

func TestAgentProxyScopesTransportAndCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/responses", r.URL.Path)
		require.Equal(t, "Bearer synthetic-session", r.Header.Get("Authorization"))
		for _, header := range []string{"Cookie", "Origin", "X-API-Key", "X-Threadify-CSRF", "X-Forwarded-Host"} {
			require.Empty(t, r.Header.Get(header))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Set-Cookie", "should-not-escape=1")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\"}\n\n")
	}))
	defer upstream.Close()
	r := httptest.NewRequest("POST", "/api/harnest/responses", strings.NewReader(`{"input":"Hello"}`))
	r.Header.Set("Authorization", "Bearer synthetic-session")
	r.Header.Set("Cookie", "private-cookie=1")
	r.Header.Set("Origin", "https://threadify.test")
	r.Header.Set("X-Forwarded-Host", "spoofed.test")
	w := httptest.NewRecorder()
	agentProxy(upstream.URL).ServeHTTP(w, r)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "response.completed")
	require.Empty(t, w.Header().Get("Set-Cookie"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestAgentProxyRejectsUnrelatedRoutesAndRedirects(t *testing.T) {
	for _, path := range []string{"/docs", "/admin", "/sessions/../docs", "/client-tools/x/extra", "/sessions/a/b/c"} {
		require.False(t, allowedAgentRoute("POST", path))
	}
	require.True(t, allowedAgentRoute("POST", "/client-tools/client_tool_123"))
	require.True(t, allowedAgentRoute("GET", "/sessions/session-123/messages"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "https://unexpected.test", 307) }))
	defer server.Close()
	w := httptest.NewRecorder()
	agentProxy(server.URL).ServeHTTP(w, httptest.NewRequest("POST", "/api/harnest/responses", nil))
	require.Equal(t, 502, w.Code)
}

func TestAgentProxyUsesPrivateLocalTLSTransport(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer signed-in-user", r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, "private agent reached")
	}))
	defer upstream.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/harnest/responses", nil)
	request.Header.Set("Authorization", "Bearer signed-in-user")
	w := httptest.NewRecorder()
	agentProxyWithTransport(upstream.URL, upstream.Client().Transport).ServeHTTP(w, request)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "private agent reached")
}

func TestAgentIdentityReturnsOnlyScopedFacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/identity", func(c *gin.Context) {
		c.Set(sharedauth.CtxUserID, "user-1")
		c.Set(sharedauth.CtxCompanyID, "company-1")
		c.Set(sharedauth.CtxEmail, "not-in-response@example.test")
		agentIdentity(c)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/identity", nil))
	require.JSONEq(t, `{"user_id":"user-1","company_id":"company-1"}`, w.Body.String())
}

func TestAgentDisabledByYAMLDoesNotCallHarnest(t *testing.T) {
	enabled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("disabled agent contacted upstream") }))
	defer upstream.Close()
	w := httptest.NewRecorder()
	configuredAgentProxy(upstream.URL, &config.AIConfig{Enabled: &enabled}).ServeHTTP(w, httptest.NewRequest("POST", "/api/harnest/responses", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "disabled by configuration")
}
