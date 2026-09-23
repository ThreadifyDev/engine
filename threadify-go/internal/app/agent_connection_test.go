package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

func clearAgentEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"THREADIFY_AGENT_URL", "THREADIFY_HARNEST_URL"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}

func TestAgentActivationConfiguration(t *testing.T) {
	clearAgentEnvironment(t)
	for _, tc := range []struct {
		name, external, mode, wantState string
		local, disabled, wantError      bool
	}{
		{name: "absent", mode: "none", wantState: "disabled"},
		{name: "local", local: true, mode: "local", wantState: "starting"},
		{name: "external", external: "https://agent.example.test", mode: "external", wantState: "starting"},
		{name: "explicit disable", external: "https://agent.example.test", disabled: true, mode: "none", wantState: "disabled"},
		{name: "conflicting activation", local: true, external: "https://agent.example.test", wantError: true},
		{name: "conflicting disable", local: true, disabled: true, wantError: true},
		{name: "malformed external URL", external: "file:///tmp/agent", wantError: true},
		{name: "credentials in URL", external: "https://user:secret@agent.example.test", wantError: true},
		{name: "path in URL", external: "https://agent.example.test/private", wantError: true},
		{name: "Engine sharing endpoint rejected", external: "https://engine.example.test/v1/agent/shared", wantError: true},
		{name: "Engine sharing endpoint trailing slash rejected", external: "https://engine.example.test/v1/agent/shared/", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{WithAgent: tc.local, AI: &config.AIConfig{}}
			cfg.AI.Agent.URL = tc.external
			if tc.disabled {
				enabled := false
				cfg.AI.Enabled = &enabled
			}
			err := ValidateAgentConfiguration(cfg)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			connection := newAgentConnection(cfg, zap.NewNop())
			t.Cleanup(func() { require.NoError(t, connection.close()) })
			status := connection.snapshot()
			require.Equal(t, tc.mode, status.Mode)
			require.Equal(t, tc.wantState, status.Status)
			require.Equal(t, tc.mode != "none", status.Enabled)
		})
	}
}

func TestAgentExternalSettingPrecedence(t *testing.T) {
	clearAgentEnvironment(t)
	cfg := &config.Config{AI: &config.AIConfig{}}
	cfg.AI.Agent.URL = "https://yaml.test"
	t.Setenv("THREADIFY_HARNEST_URL", "https://legacy.test")
	require.Equal(t, "https://yaml.test", externalAgentURL(cfg))
	t.Setenv("THREADIFY_AGENT_URL", "https://env.test")
	require.Equal(t, "https://env.test", externalAgentURL(cfg))
	t.Setenv("THREADIFY_AGENT_URL", "")
	require.Empty(t, externalAgentURL(cfg))
}

func TestAgentGatewaySettingsAloneDoNotActivateAgent(t *testing.T) {
	clearAgentEnvironment(t)
	enabled := true
	cfg := &config.Config{AI: &config.AIConfig{Enabled: &enabled}}
	cfg.AI.Gateway.BaseURL = "http://localhost:11434/v1"
	cfg.AI.Gateway.Model = "some-model"
	connection := newAgentConnection(cfg, zap.NewNop())
	connection.start(context.Background(), "http://engine.invalid")
	require.Nil(t, connection.local)
	w := httptest.NewRecorder()
	connection.statusHandler(w, httptest.NewRequest(http.MethodGet, "/v1/agent/status", nil))
	require.JSONEq(t, `{"enabled":false,"status":"disabled","mode":"none"}`, w.Body.String())
	w = httptest.NewRecorder()
	connection.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/harnest/responses", nil))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	require.Contains(t, w.Body.String(), "disabled")
	require.NoError(t, connection.close())
}

func TestAgentExternalAvailability(t *testing.T) {
	clearAgentEnvironment(t)
	for _, code := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/healthz", r.URL.Path)
				require.Empty(t, r.Header.Get("Authorization"))
				w.Header().Set("Location", "https://must-not-follow.invalid")
				w.WriteHeader(code)
			}))
			defer upstream.Close()
			cfg := &config.Config{AI: &config.AIConfig{}}
			cfg.AI.Agent.URL = upstream.URL
			connection := newAgentConnection(cfg, zap.NewNop())
			defer connection.close()
			connection.start(context.Background(), "http://engine.invalid")
			want := "unavailable"
			if code == http.StatusOK {
				want = "ready"
			}
			require.Eventually(t, func() bool { return connection.snapshot().Status == want }, time.Second, time.Millisecond*5)
			require.True(t, connection.snapshot().Enabled)
		})
	}
}

func TestAgentWriterModeRejectsLocalActivation(t *testing.T) {
	clearAgentEnvironment(t)
	require.ErrorContains(t, ValidateAgentConfiguration(&config.Config{RuntimeMode: "writer", WithAgent: true}), "writer")
}
