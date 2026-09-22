package auth

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"threadify-go/shared/registry"
)

// Wrap uses cookies only for browsers; SDK/API keys keep their existing transport and permissions.
func (s *BrowserService) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := s.requestOrigin(r)
		if r.Header.Get("Origin") == origin && validBrowserURL(origin, true) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-API-Key,X-Threadify-CSRF")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/auth/") {
			s.handleAuth(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/auth/") || r.URL.Path == "/api/team" || strings.HasPrefix(r.URL.Path, "/api/team/") {
			browserError(w, http.StatusGone, "Use Engine /auth and /v1/users endpoints")
			return
		}
		cookie := oneBrowserCookie(r, s.cookieName(r, "session"))
		if cookie != "" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
				browserError(w, 401, "ambiguous_credentials")
				return
			}
			if !s.validCSRF(r, cookie) {
				browserError(w, 403, "csrf_denied")
				return
			}
			claims, err := s.Authenticate(r.Context(), cookie)
			if err != nil {
				s.clearSession(w, r)
				browserError(w, 401, "authentication_required")
				return
			}
			// Service principals can inspect their own session without impersonating a human profile.
			if claims.PrincipalType == "service_account" && (r.URL.Path == "/api/user/profile" || r.URL.Path == "/v1/user/profile") && r.Method == http.MethodGet {
				browserJSON(w, 200, map[string]any{"user": s.profile(r, claims)})
				return
			}
			r = r.Clone(r.Context())
			r.Header.Set("Authorization", "Bearer "+cookie)
		}
		next.ServeHTTP(w, r)
	})
}
func (s *BrowserService) validCSRF(r *http.Request, token string) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	expected := s.signed("csrf", token)
	return s.sameOrigin(r) && equalBrowser(r.Header.Get(BrowserCSRFHeader), expected) && equalBrowser(oneBrowserCookie(r, s.cookieName(r, "csrf")), expected)
}
func decodeBrowserBody(w http.ResponseWriter, r *http.Request, out any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return false
	}
	return errors.Is(d.Decode(&struct{}{}), io.EOF)
}

// allowAuthRequest bounds abuse in PostgreSQL, including requests spread across replicas.
func (s *BrowserService) allowAuthRequest(r *http.Request) bool {
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	limit := 20
	if r.URL.Path == "/auth/managed/poll" || r.URL.Path == "/auth/cli/poll" {
		limit = 120
	}
	now := s.now().UTC().Truncate(time.Minute)
	var count int
	err := s.pool.QueryRow(r.Context(), `INSERT INTO threadify_browser_rate_limits(bucket,window_start,count) VALUES($1,$2,1) ON CONFLICT(bucket) DO UPDATE SET count=CASE WHEN threadify_browser_rate_limits.window_start=$2 THEN threadify_browser_rate_limits.count+1 ELSE 1 END,window_start=$2 RETURNING count`, browserHash(ip+":"+r.URL.Path), now).Scan(&count)
	return err == nil && count <= limit
}
func (s *BrowserService) profile(r *http.Request, c *TokenClaims) map[string]any {
	name := c.Email
	onboard, first := true, true
	if c.PrincipalType == "user" {
		_ = s.pool.QueryRow(r.Context(), `SELECT COALESCE(full_name,''),COALESCE(onboarding_completed,false),COALESCE(first_instrumentation_done,false) FROM users WHERE id=$1`, c.UserID).Scan(&name, &onboard, &first)
	} else {
		_ = s.pool.QueryRow(r.Context(), `SELECT name FROM service_accounts WHERE id=$1`, c.UserID).Scan(&name)
	}
	return map[string]any{"id": c.UserID, "company_id": c.CompanyID, "email": c.Email, "full_name": name, "email_verified": c.EmailVerified, "onboarding_completed": onboard, "first_instrumentation_done": first, "roles": c.Roles, "principal_type": c.PrincipalType}
}
func (s *BrowserService) handleAuth(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/auth/cli/") {
		s.handleCLIAuth(w, r)
		return
	}
	if r.URL.Path == "/auth/session" && r.Method == http.MethodGet {
		token := oneBrowserCookie(r, s.cookieName(r, "session"))
		c, err := s.Authenticate(r.Context(), token)
		if err != nil {
			s.clearSession(w, r)
			browserJSON(w, 200, map[string]bool{"authenticated": false})
			return
		}
		s.cookie(w, r, "csrf", s.signed("csrf", token), c.ExpiresAt, false)
		browserJSON(w, 200, map[string]any{"authenticated": true, "subject_kind": c.PrincipalType, "user": s.profile(r, c)})
		return
	}
	if r.Method != http.MethodPost {
		browserError(w, 405, "method_not_allowed")
		return
	}
	if !s.sameOrigin(r) {
		browserError(w, 403, "origin_denied")
		return
	}
	if !s.allowAuthRequest(r) {
		w.Header().Set("Retry-After", "60")
		browserError(w, 429, "rate_limited")
		return
	}
	// Prune expired capabilities during auth traffic; indexes bound steady-state session storage.
	_, _ = s.pool.Exec(r.Context(), `DELETE FROM threadify_browser_logins WHERE expires_at<NOW()-interval '1 day'; DELETE FROM threadify_browser_sessions WHERE expires_at<NOW()-interval '1 day'; DELETE FROM threadify_browser_rate_limits WHERE window_start<NOW()-interval '1 day'`)
	switch r.URL.Path {
	case "/auth/session/verify":
		token := oneBrowserCookie(r, s.cookieName(r, "session"))
		if !s.validCSRF(r, token) {
			browserError(w, 403, "csrf_denied")
			return
		}
		claims, err := s.Authenticate(r.Context(), token)
		if err != nil {
			browserError(w, 401, "authentication_required")
			return
		}
		browserJSON(w, 200, map[string]any{"authenticated": true, "user": s.profile(r, claims)})
	case "/auth/api-key/exchange", "/auth/license/exchange":
		var body struct {
			APIKey     string `json:"api_key"`
			LicenseKey string `json:"license_key"`
		}
		if !decodeBrowserBody(w, r, &body) || (body.APIKey != "" && body.LicenseKey != "") {
			browserError(w, 400, "invalid_request")
			return
		}
		key := body.APIKey
		if key == "" {
			key = body.LicenseKey
		}
		token, expiry, err := s.ExchangeKey(r.Context(), strings.TrimSpace(key))
		if err != nil {
			browserError(w, 401, "api_key_denied")
			return
		}
		s.setSession(w, r, token, expiry)
		browserJSON(w, 200, map[string]string{"status": "authenticated"})
	case "/auth/managed/start":
		var body struct{}
		if !decodeBrowserBody(w, r, &body) {
			browserError(w, 400, "invalid_request")
			return
		}
		result, err := s.StartLogin(r.Context())
		if err != nil {
			browserError(w, 503, "managed_login_unavailable")
			return
		}
		id, poll := result["transaction_id"].(string), result["poll_token"].(string)
		s.cookie(w, r, "login", s.signed("login", id+":"+poll), result["expires_at"].(time.Time), true)
		browserJSON(w, 201, result)
	case "/auth/managed/poll":
		var body struct {
			ID   string `json:"transaction_id"`
			Poll string `json:"poll_token"`
		}
		if !decodeBrowserBody(w, r, &body) {
			browserError(w, 400, "invalid_request")
			return
		}
		if !equalBrowser(oneBrowserCookie(r, s.cookieName(r, "login")), s.signed("login", body.ID+":"+body.Poll)) {
			browserError(w, 403, "managed_login_binding_denied")
			return
		}
		token, expiry, err := s.PollLogin(r.Context(), body.ID, body.Poll)
		if errors.Is(err, registry.ErrIdentityPending) {
			browserJSON(w, 202, map[string]string{"status": "pending"})
			return
		}
		if err != nil {
			status, code, stage := managedLoginFailure(err)
			slog.WarnContext(r.Context(), "Managed sign-in failed", "code", code, "stage", stage)
			browserError(w, status, code)
			return
		}
		s.cookie(w, r, "login", "", time.Unix(1, 0), true)
		s.setSession(w, r, token, expiry)
		browserJSON(w, 200, map[string]string{"status": "authenticated"})
	case "/auth/logout":
		token := oneBrowserCookie(r, s.cookieName(r, "session"))
		if !s.validCSRF(r, token) {
			browserError(w, 403, "csrf_denied")
			return
		}

		var encrypted string
		var logoutExpiry *time.Time
		err := s.pool.QueryRow(r.Context(), `UPDATE threadify_browser_sessions SET revoked_at=$2 WHERE token_hash=$1 AND revoked_at IS NULL AND company_id=$3 AND installation_id=$4 RETURNING logout_ciphertext,logout_expires_at`, browserHash(token), s.now().UTC(), s.registry.CompanyID(), s.registry.InstallationID()).Scan(&encrypted, &logoutExpiry)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			browserError(w, 503, "logout_unavailable")
			return
		}
		s.clearSession(w, r)
		logoutURL := ""
		if encrypted != "" && logoutExpiry != nil && logoutExpiry.After(s.now().UTC()) {
			raw, e := s.decrypt(encrypted)
			if e == nil {
				u, e := s.registry.LogoutIdentity(r.Context(), raw, s.requestOrigin(r)+"/login")
				if e == nil && validBrowserURL(u, false) {
					logoutURL = u
				}
			}
		}
		browserJSON(w, 200, map[string]string{"status": "logged_out", "logout_url": logoutURL})
	default:
		browserError(w, 404, "not_found")
	}
}
