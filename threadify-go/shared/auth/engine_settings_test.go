package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnginePublicURLPersistenceAndScope(t *testing.T) {
	s, f := browserFixture(t)
	ctx := context.Background()
	owner, _ := userTestOwner(t, s)
	configured := "https://config.example.test"
	setting, err := s.engineSettings(ctx, owner, configured)
	require.NoError(t, err)
	require.Equal(t, "config", setting.Source)
	require.Equal(t, configured, setting.PublicURL)
	require.Equal(t, "wss://config.example.test/threads", setting.Endpoints["websocket"])
	require.NoError(t, s.changeEngineURL(ctx, owner, "https://public.example.test/threadify/", false))
	// Recreate the service against the same DB, as a restart or another replica would.
	restarted, err := NewBrowserService(ctx, s.pool, f)
	require.NoError(t, err)
	setting, err = restarted.engineSettings(ctx, owner, "https://changed-config.example.test")
	require.NoError(t, err)
	require.Equal(t, "ui", setting.Source)
	require.Equal(t, "https://public.example.test/threadify/v1/traces", setting.Endpoints["otel"])
	require.Equal(t, "https://public.example.test/threadify/mcp", setting.Endpoints["mcp"])
	other := *owner
	other.CompanyID = "other"
	_, err = s.engineSettings(ctx, &other, configured)
	require.ErrorIs(t, err, ErrUserDenied)
	require.ErrorIs(t, s.changeEngineURL(ctx, &other, "https://evil.test", false), ErrUserDenied)
	// Claims alone cannot grant mutation authority; roles are rechecked in PostgreSQL.
	member := *owner
	member.UserID = "unknown-member"
	require.ErrorIs(t, s.changeEngineURL(ctx, &member, "https://evil.test", false), ErrUserDenied)
	require.ErrorIs(t, s.changeEngineURL(ctx, owner, "https://user:password@host.test", false), ErrInvalidUser)
	require.NoError(t, restarted.changeEngineURL(ctx, owner, "", true))
	setting, err = s.engineSettings(ctx, owner, configured)
	require.NoError(t, err)
	require.Equal(t, "config", setting.Source)
	require.Equal(t, configured, setting.PublicURL)
	setting, err = s.engineSettings(ctx, owner, "")
	require.NoError(t, err)
	require.Equal(t, "unset", setting.Source)
	require.Empty(t, setting.Endpoints)
}

func TestEnginePublicURLHTTPRequiresAdminAndCSRF(t *testing.T) {
	s, _ := browserFixture(t)
	_, token := userTestOwner(t, s)
	handler := s.Wrap(s.EngineSettingsHandler("https://config.example.test", http.NotFoundHandler()))
	call := func(method, body string, withSession, withCSRF bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8083/v1/engine/settings", strings.NewReader(body))
		r.Header.Set("Origin", s.origin)
		r.Header.Set("Content-Type", "application/json")
		if withSession {
			r.AddCookie(&http.Cookie{Name: s.cookieName(r, "session"), Value: token})
		}
		if withCSRF {
			csrf := s.signed("csrf", token)
			r.AddCookie(&http.Cookie{Name: s.cookieName(r, "csrf"), Value: csrf})
			r.Header.Set(BrowserCSRFHeader, csrf)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, call("GET", "", false, false).Code)
	require.Equal(t, 403, call("PUT", `{"public_url":"https://public.example.test"}`, true, false).Code)
	w := call("PUT", `{"public_url":"https://public.example.test"}`, true, true)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"source":"ui"`)
	require.Equal(t, 400, call("PUT", `{"public_url":"https://host.test?token=key"}`, true, true).Code)
	require.Equal(t, 200, call("DELETE", "", true, true).Code)
	w = call("GET", "", true, false)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"source":"config"`)
}
