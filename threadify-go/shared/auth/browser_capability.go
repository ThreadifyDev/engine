package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

const browserCapabilityPrefix = "tfb_"

var ErrInvalidBrowserGrant = errors.New("invalid browser grant")

// BrowserCapability is an opaque, short-lived grant issued to an application backend.
// The Engine stores its hash and checks the source key on every use.
type BrowserCapability struct {
	OwnerID    string
	CompanyID  string
	Actions    []string
	ThreadIDs  []string
	ThreadKeys []string
	ExpiresAt  time.Time
}

var browserCapabilityActions = []string{
	"startThread", "recordThreadEvent", "recordBrowserAction", "waitFor", "addRefs", "closeThread", "threadEnd",
}

func (c *BrowserCapability) Allows(action, threadID, threadKey string, created []string) bool {
	if c == nil {
		return false
	}
	if action == "closeConnection" || action == "heartbeat" || action == "cancelWait" {
		return true
	}
	if action == "thread" {
		action = "startThread"
	}
	if !slices.Contains(c.Actions, action) {
		return false
	}
	if action == "startThread" {
		return threadKey != "" && slices.Contains(c.ThreadKeys, threadKey)
	}
	return threadID != "" && (slices.Contains(c.ThreadIDs, threadID) || slices.Contains(created, threadID))
}

func (s *BrowserService) allowedBrowserOrigin(r *http.Request, origin string) bool {
	if !validBrowserURL(origin, true) {
		return false
	}
	if origin == s.requestOrigin(r) {
		return true
	}
	for _, configured := range strings.Split(os.Getenv("THREADIFY_BROWSER_ORIGINS"), ",") {
		if origin == strings.TrimSpace(configured) {
			return true
		}
	}
	return false
}

// BrowserOriginAllowed leaves non-browser SDK connections untouched while
// requiring browser Origins to match the Engine or an explicit allowlist.
func BrowserOriginAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	s := currentBrowser.Load()
	return s != nil && s.allowedBrowserOrigin(r, origin)
}

// IssueBrowserCapability accepts service keys only. Human keys and browser
// sessions cannot mint grants for service-owned threads.
func (s *BrowserService) IssueBrowserCapability(ctx context.Context, request *http.Request, key, origin string, actions, threadIDs, threadKeys []string, ttlSeconds int) (string, time.Time, error) {
	if key == "" || len(key) > 8192 {
		return "", time.Time{}, ErrBrowserAuth
	}
	if !s.allowedBrowserOrigin(request, origin) {
		return "", time.Time{}, ErrInvalidBrowserGrant
	}
	if ttlSeconds < 1 || ttlSeconds > 300 || len(actions) == 0 || len(actions) > len(browserCapabilityActions) || len(threadIDs) > 32 || len(threadKeys) > 32 {
		return "", time.Time{}, ErrInvalidBrowserGrant
	}
	for i, action := range actions {
		if !slices.Contains(browserCapabilityActions, action) || slices.Contains(actions[:i], action) {
			return "", time.Time{}, ErrInvalidBrowserGrant
		}
	}
	for i, id := range threadIDs {
		if strings.TrimSpace(id) != id || id == "" || len(id) > 256 || slices.Contains(threadIDs[:i], id) {
			return "", time.Time{}, ErrInvalidBrowserGrant
		}
	}
	for i, key := range threadKeys {
		if strings.TrimSpace(key) != key || key == "" || len(key) > 1024 || slices.Contains(threadKeys[:i], key) {
			return "", time.Time{}, ErrInvalidBrowserGrant
		}
	}
	if slices.Contains(actions, "startThread") && len(threadKeys) == 0 {
		return "", time.Time{}, ErrInvalidBrowserGrant
	}
	if threadIDs == nil {
		threadIDs = []string{}
	}
	if threadKeys == nil {
		threadKeys = []string{}
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return "", time.Time{}, ErrBrowserAuth
	}
	var keyID, ownerID string
	err := s.pool.QueryRow(ctx, `SELECT k.id,k.service_account_id FROM api_keys k JOIN service_accounts a ON a.id=k.service_account_id AND a.company_id=k.company_id
 WHERE k.key_hash=$1 AND k.company_id=$2 AND k.is_active AND k.revoked_at IS NULL AND a.is_active
 AND (k.expires_at IS NULL OR k.expires_at>$3)
 AND EXISTS(SELECT 1 FROM user_roles r WHERE r.principal_id=a.id AND r.principal_type='service_account')`, browserHash(key), s.registry.CompanyID(), s.now().UTC()).Scan(&keyID, &ownerID)
	if err != nil {
		return "", time.Time{}, ErrBrowserAuth
	}
	token := browserCapabilityPrefix + randomBrowserToken()
	expiry := s.now().UTC().Add(time.Duration(ttlSeconds) * time.Second)
	_, _ = s.pool.Exec(ctx, `DELETE FROM threadify_browser_capabilities WHERE expires_at<$1`, s.now().UTC().Add(-24*time.Hour))
	_, err = s.pool.Exec(ctx, `INSERT INTO threadify_browser_capabilities(token_hash,company_id,installation_id,principal_id,source_key_id,origin,actions,thread_ids,thread_keys,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, browserHash(token), s.registry.CompanyID(), s.registry.InstallationID(), ownerID, keyID, origin, actions, threadIDs, threadKeys, expiry)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expiry, nil
}

func VerifyBrowserCapability(ctx context.Context, token, origin string) (*BrowserCapability, error) {
	s := currentBrowser.Load()
	if s == nil || !strings.HasPrefix(token, browserCapabilityPrefix) || len(token) != len(browserCapabilityPrefix)+43 || origin == "" {
		return nil, ErrBrowserAuth
	}
	if _, err := s.registry.Snapshot(); err != nil {
		return nil, ErrBrowserAuth
	}
	c := &BrowserCapability{CompanyID: s.registry.CompanyID()}
	err := s.pool.QueryRow(ctx, `SELECT b.principal_id,b.actions,b.thread_ids,b.thread_keys,b.expires_at FROM threadify_browser_capabilities b
 JOIN api_keys k ON k.id=b.source_key_id AND k.company_id=b.company_id
 JOIN service_accounts a ON a.id=b.principal_id AND a.company_id=b.company_id
 WHERE b.token_hash=$1 AND b.company_id=$2 AND b.installation_id=$3 AND b.origin=$4 AND b.expires_at>$5
 AND k.is_active AND k.revoked_at IS NULL AND (k.expires_at IS NULL OR k.expires_at>$5) AND a.is_active
 AND EXISTS(SELECT 1 FROM user_roles r WHERE r.principal_id=a.id AND r.principal_type='service_account')`,
		browserHash(token), s.registry.CompanyID(), s.registry.InstallationID(), origin, s.now().UTC()).Scan(&c.OwnerID, &c.Actions, &c.ThreadIDs, &c.ThreadKeys, &c.ExpiresAt)
	if err != nil {
		return nil, ErrBrowserAuth
	}
	return c, nil
}

func (s *BrowserService) handleBrowserCapability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		browserError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	// A browser cannot mint its own grant, even if it has obtained a service key.
	if r.Header.Get("Origin") != "" {
		browserError(w, http.StatusForbidden, "browser_token_denied")
		return
	}
	var body struct {
		Origin     string   `json:"origin"`
		Actions    []string `json:"actions"`
		ThreadIDs  []string `json:"thread_ids"`
		ThreadKeys []string `json:"thread_keys"`
		TTLSeconds int      `json:"ttl_seconds"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	d.DisallowUnknownFields()
	if d.Decode(&body) != nil || !errors.Is(d.Decode(&struct{}{}), io.EOF) || !s.allowedBrowserOrigin(r, body.Origin) {
		browserError(w, http.StatusBadRequest, "invalid_browser_grant")
		return
	}
	token, expiry, err := s.IssueBrowserCapability(r.Context(), r, r.Header.Get("X-API-Key"), body.Origin, body.Actions, body.ThreadIDs, body.ThreadKeys, body.TTLSeconds)
	if err != nil {
		if errors.Is(err, ErrInvalidBrowserGrant) {
			browserError(w, http.StatusBadRequest, "invalid_browser_grant")
			return
		}
		browserError(w, http.StatusUnauthorized, "browser_token_denied")
		return
	}
	browserJSON(w, http.StatusCreated, map[string]any{"token": token, "expires_at": expiry})
}
