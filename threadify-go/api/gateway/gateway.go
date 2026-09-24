package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const maxRequestBytes = 2 << 20
const maxClassifierRequestBytes = 256 << 10

// Gateway holds only immutable configuration, connection pools and in-flight slots.
// It never stores conversations, credentials, prompts or usage records.
type Gateway struct {
	config             Config
	registry           *url.URL
	upstream           *url.URL
	classifierUpstream *url.URL
	verifier           *http.Client
	proxy              *httputil.ReverseProxy
	slots              chan struct{}
}

// New constructs a multi-tenant gateway without a database, NATS or installation binding.
func New(c Config) (*Gateway, error) {
	registry, err := endpoint(c.RegistryURL)
	if err != nil {
		return nil, errors.New("invalid registry URL: HTTPS or loopback HTTP required")
	}
	upstream, err := endpoint(c.UpstreamURL)
	if err != nil {
		return nil, errors.New("invalid upstream URL: HTTPS or loopback HTTP required")
	}
	if c.Model == "" || c.UpstreamModel == "" || strings.ContainsAny(c.Model+c.UpstreamModel, " \r\n\t") || c.MaxConcurrent < 1 || c.Timeout <= 0 {
		return nil, errors.New("model, upstream model, positive concurrency and timeout are required")
	}
	var classifierUpstream *url.URL
	if c.ClassifierUpstreamURL != "" {
		classifierUpstream, err = endpoint(c.ClassifierUpstreamURL)
		if err != nil || c.ClassifierModel == "" || c.ClassifierUpstreamModel == "" || c.ClassifierModel == c.Model || strings.ContainsAny(c.ClassifierModel+c.ClassifierUpstreamModel, " \r\n\t") {
			return nil, errors.New("valid classifier upstream and distinct public/upstream models are required")
		}
	}
	if upstream.Scheme != "https" && (c.CAFile != "" || c.CertFile != "" || c.KeyFile != "") {
		return nil, errors.New("upstream TLS files require HTTPS")
	}
	transport, err := upstreamTransport(c)
	if err != nil {
		return nil, err
	}
	registryTransport := http.DefaultTransport.(*http.Transport).Clone()
	registryTransport.Proxy = nil
	g := &Gateway{config: c, registry: registry, upstream: upstream, classifierUpstream: classifierUpstream, slots: make(chan struct{}, c.MaxConcurrent), verifier: &http.Client{Transport: registryTransport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	g.proxy = &httputil.ReverseProxy{
		ErrorLog:      log.New(io.Discard, "", 0),
		Transport:     transport,
		FlushInterval: -1,
		Rewrite:       g.rewrite,
		ModifyResponse: func(res *http.Response) error {
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				res.Body.Close()
				return errors.New("upstream rejected inference")
			}
			contentType := res.Header.Get("Content-Type")
			// Never forward upstream cookies, internal URLs or provider diagnostics.
			res.Header = make(http.Header)
			res.Header.Set("Content-Type", contentType)
			res.Header.Set("Cache-Control", "no-store")
			res.Header.Set("X-Accel-Buffering", "no")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
			failure(w, http.StatusBadGateway, "model_unavailable")
		},
	}
	return g, nil
}

// Close releases idle sockets; callers drain active requests before closing.
func (g *Gateway) Close() {
	g.verifier.CloseIdleConnections()
	g.proxy.Transport.(*http.Transport).CloseIdleConnections()
}

func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": code, "type": "gateway_error", "code": code}})
}

func (g *Gateway) rewrite(p *httputil.ProxyRequest) {
	upstream, suffix, key := g.upstream, "/chat/completions", g.config.UpstreamKey
	if p.In.URL.Path == "/v1/systemone" {
		upstream, suffix, key = g.classifierUpstream, "/systemone", g.config.ClassifierUpstreamKey
	}
	p.SetURL(upstream)
	p.Out.URL.Path = upstream.Path + suffix
	p.Out.URL.RawPath = ""
	p.Out.URL.RawQuery = ""
	p.Out.Host = upstream.Host
	// Explicit allowlist: customer license, cookies and forwarded identity never reach the model.
	p.Out.Header = make(http.Header)
	p.Out.Header.Set("Content-Type", "application/json")
	p.Out.Header.Set("Accept", "application/json, text/event-stream")
	if key != "" {
		p.Out.Header.Set("Authorization", "Bearer "+key)
	}
}

// authorize verifies every request so revocation applies without a local cache.
// The existing Registry handshake is the license/product authority; it has no AI billing dimension yet.
func (g *Gateway) authorize(r *http.Request) (int, string) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == "" || len(auth) > 8192 {
		return http.StatusUnauthorized, "license_required"
	}
	u := *g.registry
	u.Path += "/api/threadify/handshake"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		return http.StatusServiceUnavailable, "verification_unavailable"
	}
	req.Header.Set("Authorization", auth)
	if installation := r.Header.Get("X-Threadify-Installation-ID"); installation != "" {
		req.Header.Set("X-Threadify-Installation-ID", installation)
	}
	response, err := g.verifier.Do(req)
	if err != nil {
		return http.StatusServiceUnavailable, "verification_unavailable"
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return response.StatusCode, "license_denied"
	}
	if response.StatusCode != http.StatusOK {
		return http.StatusServiceUnavailable, "verification_unavailable"
	}
	var snapshot struct {
		Status       string `json:"status"`
		AccountID    string `json:"account_id"`
		WorkspaceID  string `json:"workspace_id"`
		Suspended    *bool  `json:"suspended"`
		Entitlements struct {
			Revision string `json:"revision"`
		} `json:"entitlements"`
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || json.Unmarshal(body, &snapshot) != nil || snapshot.Status != "ok" || snapshot.AccountID == "" || snapshot.WorkspaceID == "" || snapshot.Entitlements.Revision == "" || snapshot.Suspended == nil {
		return http.StatusServiceUnavailable, "verification_unavailable"
	}
	if *snapshot.Suspended {
		return http.StatusForbidden, "license_denied"
	}
	return 0, ""
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok\n")
		return
	}
	models := r.URL.Path == "/v1/models" && r.Method == http.MethodGet
	chat := r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost
	classifier := g.classifierUpstream != nil && r.URL.Path == "/v1/systemone" && r.Method == http.MethodPost
	if (!models && !chat && !classifier) || r.URL.RawQuery != "" {
		failure(w, http.StatusNotFound, "not_found")
		return
	}
	select {
	case g.slots <- struct{}{}:
		defer func() { <-g.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		failure(w, http.StatusTooManyRequests, "gateway_busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), g.config.Timeout)
	defer cancel()
	// Also bound slow downstream writes; cancelling the upstream alone cannot unblock them.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(g.config.Timeout))
	r = r.WithContext(ctx)
	if status, code := g.authorize(r); status != 0 {
		failure(w, status, code)
		return
	}
	if models {
		w.Header().Set("Content-Type", "application/json")
		available := []any{map[string]any{"id": g.config.Model, "object": "model", "created": 0, "owned_by": "threadify"}}
		if g.classifierUpstream != nil {
			available = append(available, map[string]any{"id": g.config.ClassifierModel, "object": "model", "created": 0, "owned_by": "threadify"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": available})
		return
	}
	limit := int64(maxRequestBytes)
	if classifier {
		limit = maxClassifierRequestBytes
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		failure(w, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil || payload == nil {
		failure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var model string
	expectedModel, upstreamModel := g.config.Model, g.config.UpstreamModel
	if classifier {
		expectedModel, upstreamModel = g.config.ClassifierModel, g.config.ClassifierUpstreamModel
	}
	if json.Unmarshal(payload["model"], &model) != nil || model != expectedModel {
		failure(w, http.StatusBadRequest, "unsupported_model")
		return
	}
	// The gateway forwards tool definitions and results but never executes tools or accepts a caller-selected endpoint.
	if classifier && (len(payload["state"]) == 0 || len(payload["questions"]) == 0) {
		failure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	payload["model"], _ = json.Marshal(upstreamModel)
	body, _ = json.Marshal(payload)
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	g.proxy.ServeHTTP(w, r)
}
