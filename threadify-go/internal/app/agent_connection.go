package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/threadify/engine/internal/agentbundle"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

// agentStatus is the public capability contract; it contains no connection secrets.
type agentStatus struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Mode    string `json:"mode"`
}

func externalAgentURL(cfg *config.Config) string {
	if value, exists := os.LookupEnv("THREADIFY_AGENT_URL"); exists {
		return strings.TrimSpace(value)
	}
	if cfg.AI != nil && strings.TrimSpace(cfg.AI.Agent.URL) != "" {
		return strings.TrimSpace(cfg.AI.Agent.URL)
	}
	return strings.TrimSpace(os.Getenv("THREADIFY_HARNEST_URL"))
}

// ValidateAgentConfiguration fails before opening databases or downloading files.
func ValidateAgentConfiguration(cfg *config.Config) error {
	external := externalAgentURL(cfg)
	if cfg.WithAgent && external != "" {
		return errors.New("choose --with-agent or an external ai.agent.url/THREADIFY_AGENT_URL, not both")
	}
	if cfg.WithAgent && cfg.RuntimeMode == "writer" {
		return errors.New("--with-agent is unavailable in writer mode")
	}
	if cfg.AI != nil && cfg.AI.Enabled != nil && !*cfg.AI.Enabled {
		if cfg.WithAgent {
			return errors.New("--with-agent conflicts with ai.enabled: false")
		}
		return nil
	}
	if external != "" {
		if _, err := validAgentURL(external); err != nil {
			return fmt.Errorf("invalid external agent configuration: %w", err)
		}
	}
	return nil
}

type agentConnection struct {
	mu        sync.RWMutex
	startOnce sync.Once
	cfg       *config.Config
	logger    *zap.Logger
	state     agentStatus
	url       string
	local     *agentbundle.Manager
	proxy     http.Handler
	cancel    context.CancelFunc
	client    *http.Client
}

func newAgentConnection(cfg *config.Config, logger *zap.Logger) *agentConnection {
	a := &agentConnection{cfg: cfg, logger: logger, state: agentStatus{Status: "disabled", Mode: "none"}}
	if cfg.RuntimeMode == "writer" || (cfg.AI != nil && cfg.AI.Enabled != nil && !*cfg.AI.Enabled) {
		return a
	}
	if cfg.WithAgent {
		a.state = agentStatus{Enabled: true, Status: "starting", Mode: "local"}
	} else if endpoint := externalAgentURL(cfg); endpoint != "" {
		a.url = endpoint
		a.state = agentStatus{Enabled: true, Status: "starting", Mode: "external"}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		a.client = &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		a.proxy = agentProxyWithTransport(endpoint, transport)
	}
	return a
}

func (a *agentConnection) start(ctx context.Context, engineURL string) {
	a.startOnce.Do(func() {
		if !a.state.Enabled {
			return
		}
		ctx, cancel := context.WithCancel(ctx)
		a.mu.Lock()
		a.cancel = cancel
		a.mu.Unlock()
		if a.state.Mode == "external" {
			go a.watchExternal(ctx)
			return
		}
		manager, err := agentbundle.New(agentbundle.Options{
			ConfigPath: a.cfg.ConfigPath, EngineURL: engineURL, Version: a.cfg.AgentVersion,
			CacheDir: a.cfg.AgentCacheDir, ArchivePath: a.cfg.AgentRuntimeArchive,
		})
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			a.state.Status = "unavailable"
			a.logger.Warn("local Threadify agent could not initialize", zap.Error(err))
			return
		}
		a.local = manager
		manager.Start(ctx)
		go a.watchLocal(ctx, manager)
	})
}

func (a *agentConnection) watchLocal(ctx context.Context, manager *agentbundle.Manager) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	previous := ""
	for {
		state := manager.Status()
		if state.State != previous {
			previous = state.State
			fields := []zap.Field{zap.String("status", state.State)}
			if state.State == "unavailable" {
				fields = append(fields, zap.String("detail", state.Message), zap.String("diagnostics", manager.DiagnosticsPath()))
				a.logger.Warn("Threadify agent unavailable; fix the cause and restart the Engine", fields...)
			} else {
				a.logger.Info("Threadify agent status changed", fields...)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *agentConnection) watchExternal(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		status := "unavailable"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.url, "/")+"/healthz", nil)
		if err == nil {
			resp, requestErr := a.client.Do(req)
			if requestErr == nil {
				if resp.StatusCode == http.StatusOK {
					status = "ready"
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				_ = resp.Body.Close()
			}
		}
		a.mu.Lock()
		a.state.Status = status
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *agentConnection) snapshot() agentStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	state := a.state
	if a.local != nil {
		state.Status = a.local.Status().State
	}
	return state
}

func (a *agentConnection) statusHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(a.snapshot())
}

func (a *agentConnection) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	local, proxy := a.local, a.proxy
	a.mu.RUnlock()
	if local != nil {
		if endpoint, transport := local.Endpoint(); endpoint != "" {
			agentProxyWithTransport(endpoint, transport).ServeHTTP(w, r)
			return
		}
	} else if proxy != nil {
		proxy.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	message := "Threadify agent is starting or unavailable. Try again shortly."
	if !a.snapshot().Enabled {
		message = "Threadify agent is disabled."
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func (a *agentConnection) close() error {
	a.mu.RLock()
	cancel, local := a.cancel, a.local
	a.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
	if a.client != nil {
		a.client.CloseIdleConnections()
	}
	if local != nil {
		return local.Close()
	}
	return nil
}

// StartAgent is called only after binding the Engine listener, so the child can
// safely call back into this Engine using its actual address (including port 0).
func (a *App) StartAgent(ctx context.Context, engineURL string) {
	if a.agent != nil {
		a.agent.start(ctx, engineURL)
	}
}
