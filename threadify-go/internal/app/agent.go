package app

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/threadify/engine/internal/config"
	sharedauth "threadify-go/shared/auth"
)

var agentResourceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)

// agentServiceURL prefers the product setting while preserving existing deployments.
// An explicitly empty new setting disables routing rather than using the old value.
func agentServiceURL() string {
	if value, configured := os.LookupEnv("THREADIFY_AGENT_URL"); configured {
		return value
	}
	return os.Getenv("THREADIFY_HARNEST_URL")
}

// configuredAgentProxy lets the Engine disable AI without changing legacy setups.
func configuredAgentProxy(rawURL string, ai *config.AIConfig) http.Handler {
	if ai != nil && ai.Enabled != nil && !*ai.Enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Threadify AI is disabled by configuration."}`))
		})
	}
	return agentProxy(rawURL)
}

// Only expose the conversation transport, never Harnest's administration/playground.
func allowedAgentRoute(method, path string) bool {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 1 {
		return (parts[0] == "sessions" && (method == "GET" || method == "POST")) ||
			(parts[0] == "responses" && method == "POST")
	}
	if len(parts) < 2 || !agentResourceID.MatchString(parts[1]) {
		return false
	}
	if len(parts) == 2 {
		return (parts[0] == "client-tools" && method == "POST") ||
			(parts[0] == "sessions" && (method == "GET" || method == "DELETE"))
	}
	return len(parts) == 3 && parts[0] == "sessions" && parts[2] == "messages" && method == "GET"
}

func agentProxy(rawURL string) http.Handler {
	return agentProxyWithTransport(rawURL, nil)
}

func validAgentURL(rawURL string) (*url.URL, error) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" || target.RawPath != "" || (target.Path != "" && target.Path != "/") {
		return nil, errors.New("agent URL must be an HTTP(S) origin without credentials, path, query, or fragment")
	}
	return target, nil
}

func agentProxyWithTransport(rawURL string, transport http.RoundTripper) http.Handler {
	target, err := validAgentURL(rawURL)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"Threadify agent is not configured. Set THREADIFY_AGENT_URL on the Engine."}`, http.StatusServiceUnavailable)
		})
	}
	proxy := &httputil.ReverseProxy{
		FlushInterval: -1,
		Transport:     transport,
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.URL.Path = strings.TrimPrefix(p.In.URL.Path, "/api/harnest")
			p.Out.URL.RawPath = ""
			p.Out.Header = make(http.Header)
			for _, name := range []string{"Authorization", "Content-Type", "Accept"} {
				p.Out.Header.Set(name, p.In.Header.Get(name))
			}
		},
		ModifyResponse: func(r *http.Response) error {
			if r.StatusCode >= 300 && r.StatusCode < 400 {
				return errors.New("agent redirects are not supported")
			}
			r.Header.Del("Set-Cookie")
			r.Header.Set("Cache-Control", "no-store")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			http.Error(w, `{"error":"Threadify agent is unavailable. Check the Threadify agent service."}`, http.StatusBadGateway)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedAgentRoute(r.Method, strings.TrimPrefix(r.URL.Path, "/api/harnest")) {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		// Model streams may outlive the Engine's normal 30-second write deadline.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		proxy.ServeHTTP(w, r)
	})
}

// Harnest introspects opaque, revocable Engine sessions through the same auth
// middleware as other requests. Only selected identity facts leave this handler.
func agentIdentity(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{
		"user_id":    c.GetString(sharedauth.CtxUserID),
		"company_id": c.GetString(sharedauth.CtxCompanyID),
	})
}
