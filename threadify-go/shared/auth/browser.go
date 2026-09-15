package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"threadify-go/shared/registry"
)

const BrowserCSRFHeader = "X-Threadify-CSRF"
const sessionPrefix = "tfs_"

var ErrBrowserAuth = errors.New("browser authentication required")
var currentBrowser atomic.Pointer[BrowserService]

// BrowserRegistry keeps sign-in tied to a verified product installation.
type BrowserRegistry interface {
	Snapshot() (registry.Snapshot, error)
	CompanyID() string
	AccountID() string
	OwnerEmail() string
	InstallationID() string
	BrowserKey() []byte
	BrowserOrigin() string
	IsLicenseKey(string) bool
	StartIdentity(context.Context, string, string) (registry.IdentityTransaction, error)
	ExchangeIdentity(context.Context, string, string) (registry.IdentityAssertion, error)
	LogoutIdentity(context.Context, string, string) (string, error)
}

type BrowserService struct {
	pool     *pgxpool.Pool
	registry BrowserRegistry
	key      []byte
	origin   string
	now      func() time.Time
}

// NewBrowserService initializes durable sessions shared by the Engine and external API.
func NewBrowserService(ctx context.Context, pool *pgxpool.Pool, r BrowserRegistry) (*BrowserService, error) {
	if pool == nil || r == nil || len(r.BrowserKey()) != 32 {
		return nil, errors.New("invalid browser auth configuration")
	}
	origin := strings.TrimRight(r.BrowserOrigin(), "/")
	if origin != "" && !validBrowserURL(origin, true) {
		return nil, errors.New("registry.browser_origin must be an HTTPS origin (HTTP is allowed on loopback)")
	}
	s := &BrowserService{pool: pool, registry: r, key: r.BrowserKey(), origin: origin, now: time.Now}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Serialize schema initialization when the Engine and Web API start together.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('threadify:browser:schema',0))`); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `ALTER TABLE users ADD COLUMN IF NOT EXISTS status varchar(16) NOT NULL DEFAULT 'active' CHECK(status IN ('invited','active','suspended','archived'));
 CREATE TABLE IF NOT EXISTS threadify_user_audit(id bigserial PRIMARY KEY,company_id text NOT NULL,actor_id text NOT NULL,user_id text NOT NULL,action text NOT NULL,created_at timestamptz NOT NULL);
 CREATE TABLE IF NOT EXISTS threadify_engine_settings(company_id text NOT NULL,installation_id text NOT NULL,public_url text NOT NULL,updated_by text NOT NULL,updated_at timestamptz NOT NULL,PRIMARY KEY(company_id,installation_id));
 CREATE TABLE IF NOT EXISTS threadify_browser_sessions(
 token_hash text PRIMARY KEY, company_id text NOT NULL, installation_id text NOT NULL,
 principal_id text NOT NULL, principal_type text NOT NULL CHECK(principal_type IN ('user','service_account')),
 source_key_id text, source text NOT NULL, auth_generation text NOT NULL DEFAULT '', expires_at timestamptz NOT NULL, revoked_at timestamptz,
 logout_ciphertext text NOT NULL DEFAULT '', logout_expires_at timestamptz, created_at timestamptz NOT NULL DEFAULT NOW());
 ALTER TABLE threadify_browser_sessions ADD COLUMN IF NOT EXISTS auth_generation text NOT NULL DEFAULT '';
 CREATE INDEX IF NOT EXISTS threadify_browser_sessions_expiry ON threadify_browser_sessions(expires_at);
 CREATE TABLE IF NOT EXISTS threadify_cli_credentials(
 id text PRIMARY KEY,token_hash text UNIQUE NOT NULL,company_id text NOT NULL,installation_id text NOT NULL,
 principal_id text NOT NULL,principal_type text NOT NULL CHECK(principal_type IN ('user','service_account')),
 source_key_id text,auth_generation text NOT NULL,expires_at timestamptz NOT NULL,revoked_at timestamptz);
 CREATE TABLE IF NOT EXISTS threadify_cli_logins(
 id text PRIMARY KEY,poll_hash text NOT NULL,browser_hash text NOT NULL,credential_hash text NOT NULL,
 company_id text NOT NULL,installation_id text NOT NULL,expires_at timestamptz NOT NULL,credential_id text REFERENCES threadify_cli_credentials(id));
 CREATE TABLE IF NOT EXISTS threadify_browser_logins(
 id text PRIMARY KEY, registry_id text NOT NULL, poll_hash text NOT NULL, verifier_ciphertext text NOT NULL,
 company_id text NOT NULL, installation_id text NOT NULL, expires_at timestamptz NOT NULL, consumed_at timestamptz);
 CREATE TABLE IF NOT EXISTS threadify_managed_identities(
 company_id text NOT NULL, issuer text NOT NULL, subject text NOT NULL, user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 PRIMARY KEY(company_id,issuer,subject), UNIQUE(user_id));
 CREATE TABLE IF NOT EXISTS threadify_browser_rate_limits(bucket text PRIMARY KEY, window_start timestamptz NOT NULL, count int NOT NULL);`)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s, nil
}
func SetBrowserService(s *BrowserService) { currentBrowser.Store(s) }
func BrowserSessionsEnabled() bool        { return currentBrowser.Load() != nil }
func VerifyBrowserSession(ctx context.Context, token string) (*TokenClaims, error) {
	s := currentBrowser.Load()
	if s == nil {
		return nil, ErrBrowserAuth
	}
	return s.Authenticate(ctx, token)
}
func randomBrowserToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func browserHash(v string) string   { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func equalBrowser(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *BrowserService) signed(purpose, value string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(purpose + "\x00" + value))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (s *BrowserService) encrypt(value string) (string, error) {
	b, e := aes.NewCipher(s.key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(g.Seal(n, n, []byte(value), []byte(s.registry.InstallationID()))), nil
}
func (s *BrowserService) decrypt(value string) (string, error) {
	v, e := base64.RawURLEncoding.DecodeString(value)
	if e != nil {
		return "", e
	}
	b, e := aes.NewCipher(s.key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil || len(v) < g.NonceSize() {
		return "", ErrBrowserAuth
	}
	out, e := g.Open(nil, v[:g.NonceSize()], v[g.NonceSize():], []byte(s.registry.InstallationID()))
	return string(out), e
}

// validBrowserURL permits cleartext only for loopback development endpoints.
func validBrowserURL(raw string, originOnly bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	if originOnly && (u.Path != "" || u.RawQuery != "") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())))
}
func (s *BrowserService) requestOrigin(r *http.Request) string {
	if s.origin != "" {
		return s.origin
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
func (s *BrowserService) sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == s.requestOrigin(r) && validBrowserURL(s.requestOrigin(r), true)
}
func (s *BrowserService) secure(r *http.Request) bool {
	return strings.HasPrefix(s.requestOrigin(r), "https://")
}
func (s *BrowserService) cookieName(r *http.Request, name string) string {
	if s.secure(r) {
		return "__Host-threadify_" + name
	}
	return "threadify_" + name + "_dev"
}
func oneBrowserCookie(r *http.Request, name string) string {
	var value string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == name {
			value = c.Value
			count++
		}
	}
	if count != 1 {
		return ""
	}
	return value
}
func (s *BrowserService) cookie(w http.ResponseWriter, r *http.Request, name, value string, expiry time.Time, httpOnly bool) {
	c := &http.Cookie{Name: s.cookieName(r, name), Value: value, Path: "/", Secure: s.secure(r), HttpOnly: httpOnly, SameSite: http.SameSiteStrictMode, Expires: expiry}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}
func (s *BrowserService) setSession(w http.ResponseWriter, r *http.Request, token string, expiry time.Time) {
	s.cookie(w, r, "session", token, expiry, true)
	s.cookie(w, r, "csrf", s.signed("csrf", token), expiry, false)
}
func (s *BrowserService) clearSession(w http.ResponseWriter, r *http.Request) {
	s.cookie(w, r, "session", "", time.Unix(1, 0), true)
	s.cookie(w, r, "csrf", "", time.Unix(1, 0), false)
}
func browserJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
func browserError(w http.ResponseWriter, status int, code string) {
	browserJSON(w, status, map[string]string{"error": code})
}
