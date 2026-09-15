package auth

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const CLICredentialPrefix = "tfcli_"

func VerifyCLICredential(ctx context.Context, key string) (*TokenClaims, error) {
	s := currentBrowser.Load()
	if s == nil {
		return nil, ErrBrowserAuth
	}
	return s.authenticateCLI(ctx, key)
}

func (s *BrowserService) authenticateCLI(ctx context.Context, key string) (*TokenClaims, error) {
	if !strings.HasPrefix(key, CLICredentialPrefix) || len(key) != len(CLICredentialPrefix)+43 {
		return nil, ErrBrowserAuth
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return nil, ErrBrowserAuth
	}
	var id, kind string
	var expiry time.Time
	err := s.pool.QueryRow(ctx, `SELECT c.principal_id,c.principal_type,c.expires_at FROM threadify_cli_credentials c WHERE c.token_hash=$1 AND c.company_id=$2 AND c.installation_id=$3 AND c.expires_at>$4 AND c.revoked_at IS NULL AND c.auth_generation=$5 AND (c.source_key_id IS NULL OR EXISTS(SELECT 1 FROM api_keys k WHERE k.id=c.source_key_id AND k.company_id=c.company_id AND k.is_active AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>$4)))`, browserHash(key), s.registry.CompanyID(), s.registry.InstallationID(), s.now().UTC(), s.signed("generation", "v1")).Scan(&id, &kind, &expiry)
	if err != nil {
		return nil, ErrBrowserAuth
	}
	return s.principalClaims(ctx, id, kind, expiry)
}

// Enrollment separates the browser approval capability from the CLI polling
// capability. Only the locally generated credential hash reaches the Engine.
func (s *BrowserService) handleCLIAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/auth/cli/whoami" && r.Method == http.MethodGet {
		claims, err := s.AuthenticateAPIKey(r.Context(), r.Header.Get("X-API-Key"))
		if err != nil {
			browserError(w, 401, "authentication_required")
			return
		}
		browserJSON(w, 200, map[string]any{"user": s.profile(r, claims), "expires_at": func() any {
			if strings.HasPrefix(r.Header.Get("X-API-Key"), CLICredentialPrefix) {
				return claims.ExpiresAt
			}
			return nil
		}()})
		return
	}
	if r.Method != http.MethodPost {
		browserError(w, 405, "method_not_allowed")
		return
	}
	if !s.allowAuthRequest(r) {
		browserError(w, 429, "rate_limited")
		return
	}
	if _, err := s.registry.Snapshot(); err != nil {
		browserError(w, 503, "license_unavailable")
		return
	}
	switch r.URL.Path {
	case "/auth/cli/start":
		// Browser-origin enrollment is forbidden: the local CLI owns the credential.
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
			browserError(w, 403, "CLI_request_required")
			return
		}
		var body struct {
			Hash string `json:"credential_hash"`
		}
		if !decodeBrowserBody(w, r, &body) {
			browserError(w, 400, "invalid_request")
			return
		}
		decoded, err := hex.DecodeString(body.Hash)
		if err != nil || len(decoded) != 32 {
			browserError(w, 400, "invalid_credential_hash")
			return
		}
		origin := s.requestOrigin(r)
		if !validBrowserURL(origin, true) {
			browserError(w, 503, "browser_origin_required")
			return
		}
		id, poll, browser := randomBrowserToken(), randomBrowserToken(), randomBrowserToken()
		expiry := s.now().UTC().Add(5 * time.Minute)
		_, err = s.pool.Exec(r.Context(), `INSERT INTO threadify_cli_logins(id,poll_hash,browser_hash,credential_hash,company_id,installation_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, browserHash(poll), browserHash(browser), strings.ToLower(body.Hash), s.registry.CompanyID(), s.registry.InstallationID(), expiry)
		if err != nil {
			browserError(w, 500, "could_not_start_login")
			return
		}
		link := origin + "/login?next=cli-login#" + url.Values{"transaction_id": {id}, "browser_token": {browser}}.Encode()
		browserJSON(w, 201, map[string]any{"transaction_id": id, "poll_token": poll, "verification_url": link, "expires_at": expiry})
	case "/auth/cli/poll":
		if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
			browserError(w, 403, "CLI_request_required")
			return
		}
		var body struct {
			ID    string `json:"transaction_id"`
			Token string `json:"poll_token"`
		}
		if !decodeBrowserBody(w, r, &body) {
			browserError(w, 400, "invalid_request")
			return
		}
		var credentialID *string
		var expiry *time.Time
		err := s.pool.QueryRow(r.Context(), `SELECT l.credential_id,c.expires_at FROM threadify_cli_logins l LEFT JOIN threadify_cli_credentials c ON c.id=l.credential_id AND c.revoked_at IS NULL WHERE l.id=$1 AND l.poll_hash=$2 AND l.company_id=$3 AND l.installation_id=$4 AND l.expires_at>$5`, body.ID, browserHash(body.Token), s.registry.CompanyID(), s.registry.InstallationID(), s.now().UTC()).Scan(&credentialID, &expiry)
		if err != nil {
			browserError(w, 403, "login_expired_or_denied")
			return
		}
		if credentialID == nil {
			browserJSON(w, 202, map[string]string{"status": "pending"})
			return
		}
		if expiry == nil {
			browserError(w, 403, "credential_revoked")
			return
		}
		browserJSON(w, 200, map[string]any{"status": "authenticated", "credential_id": *credentialID, "expires_at": *expiry})
	case "/auth/cli/approve":
		token := oneBrowserCookie(r, s.cookieName(r, "session"))
		if !s.validCSRF(r, token) {
			browserError(w, 403, "csrf_denied")
			return
		}
		var body struct {
			ID    string `json:"transaction_id"`
			Token string `json:"browser_token"`
		}
		if !decodeBrowserBody(w, r, &body) {
			browserError(w, 400, "invalid_request")
			return
		}
		tx, err := s.pool.Begin(r.Context())
		if err != nil {
			browserError(w, 500, "approval_failed")
			return
		}
		defer tx.Rollback(r.Context())
		if err = s.lockUsers(r.Context(), tx); err != nil {
			browserError(w, 500, "approval_failed")
			return
		}
		claims, err := s.Authenticate(r.Context(), token)
		if err != nil {
			browserError(w, 401, "authentication_required")
			return
		}
		var hash string
		err = tx.QueryRow(r.Context(), `SELECT credential_hash FROM threadify_cli_logins WHERE id=$1 AND browser_hash=$2 AND company_id=$3 AND installation_id=$4 AND expires_at>$5 AND credential_id IS NULL FOR UPDATE`, body.ID, browserHash(body.Token), claims.CompanyID, s.registry.InstallationID(), s.now().UTC()).Scan(&hash)
		if err != nil {
			browserError(w, 403, "login_expired_or_denied")
			return
		}
		var source *string
		err = tx.QueryRow(r.Context(), `SELECT source_key_id FROM threadify_browser_sessions WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>$2 FOR SHARE`, browserHash(token), s.now().UTC()).Scan(&source)
		if err != nil {
			browserError(w, 401, "authentication_required")
			return
		}
		id := randomBrowserToken()
		expiry := s.now().UTC().Add(30 * 24 * time.Hour)
		_, err = tx.Exec(r.Context(), `INSERT INTO threadify_cli_credentials(id,token_hash,company_id,installation_id,principal_id,principal_type,source_key_id,auth_generation,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, hash, claims.CompanyID, s.registry.InstallationID(), claims.UserID, claims.PrincipalType, source, s.signed("generation", "v1"), expiry)
		if err == nil {
			_, err = tx.Exec(r.Context(), `UPDATE threadify_cli_logins SET credential_id=$2 WHERE id=$1`, body.ID, id)
		}
		if err == nil {
			err = tx.Commit(r.Context())
		}
		if err != nil {
			browserError(w, 500, "approval_failed")
			return
		}
		browserJSON(w, 200, map[string]string{"status": "approved"})
	case "/auth/cli/revoke":
		key := r.Header.Get("X-API-Key")
		if r.Header.Get("Origin") != "" || !strings.HasPrefix(key, CLICredentialPrefix) {
			browserError(w, 403, "CLI_credential_required")
			return
		}
		// Idempotent revoke also permits cleaning up an expired or suspended login.
		result, err := s.pool.Exec(r.Context(), `UPDATE threadify_cli_credentials SET revoked_at=COALESCE(revoked_at,$4) WHERE token_hash=$1 AND company_id=$2 AND installation_id=$3`, browserHash(key), s.registry.CompanyID(), s.registry.InstallationID(), s.now().UTC())
		if err != nil {
			browserError(w, 500, "revocation_failed")
			return
		}
		if result.RowsAffected() == 0 {
			browserError(w, 401, "unknown_credential")
			return
		}
		browserJSON(w, 200, map[string]bool{"revoked": true})
	default:
		browserError(w, 404, "not_found")
	}
}
