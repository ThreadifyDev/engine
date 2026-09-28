package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"threadify-go/shared/registry"
)

type browserRegistryFixture struct {
	mu                 sync.Mutex
	pending, suspended bool
	verifier, local    string
	assertion          registry.IdentityAssertion
	logoutCalls        int
	exchangeErr        error
}

func (f *browserRegistryFixture) Snapshot() (registry.Snapshot, error) {
	if f.suspended {
		return registry.Snapshot{}, registry.ErrUnverified
	}
	return registry.Snapshot{}, nil
}
func (*browserRegistryFixture) CompanyID() string            { return "company" }
func (*browserRegistryFixture) AccountID() string            { return "account" }
func (*browserRegistryFixture) OwnerEmail() string           { return "owner@example.test" }
func (*browserRegistryFixture) InstallationID() string       { return "installation" }
func (*browserRegistryFixture) BrowserKey() []byte           { return []byte(strings.Repeat("k", 32)) }
func (*browserRegistryFixture) BrowserOrigin() string        { return "http://127.0.0.1:3002" }
func (*browserRegistryFixture) IsLicenseKey(key string) bool { return key == "test-license" }
func (f *browserRegistryFixture) StartIdentity(_ context.Context, verifier, local string) (registry.IdentityTransaction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verifier, f.local = verifier, local
	now := time.Now().UTC()
	f.assertion = registry.IdentityAssertion{SchemaVersion: 1, TransactionID: "remote", AccountID: "account", InstallationID: "installation", Purpose: "browser_login", Provider: "logto", Issuer: "https://identity.example.test", ExternalSubject: "subject", VerifiedEmail: "owner@example.test", AuthMethod: "email_code", EnrollmentRef: local, AuthenticatedAt: now, ExpiresAt: now.Add(5 * time.Minute), LogoutToken: "logout-capability", LogoutExpiresAt: now.Add(time.Hour)}
	return registry.IdentityTransaction{TransactionID: "remote", VerificationURL: "https://registry.example.test/sign-in", ExpiresAt: f.assertion.ExpiresAt}, nil
}
func (f *browserRegistryFixture) ExchangeIdentity(_ context.Context, id, verifier string) (registry.IdentityAssertion, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != "remote" || verifier != f.verifier {
		return registry.IdentityAssertion{}, ErrBrowserAuth
	}
	if f.pending {
		return registry.IdentityAssertion{}, registry.ErrIdentityPending
	}
	if f.exchangeErr != nil {
		return registry.IdentityAssertion{}, f.exchangeErr
	}
	return f.assertion, nil
}
func (f *browserRegistryFixture) LogoutIdentity(_ context.Context, token, target string) (string, error) {
	if token != "logout-capability" || target != f.BrowserOrigin()+"/login" {
		return "", errors.New("bad logout binding")
	}
	f.logoutCalls++
	return "https://identity.example.test/logout", nil
}

// browserFixture isolates database state while exercising the actual PostgreSQL queries and transactions.
func browserFixture(t *testing.T) (*BrowserService, *browserRegistryFixture) {
	t.Helper()
	dsn := os.Getenv("THREADIFY_AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_AUTH_TEST_DATABASE_URL to run PostgreSQL browser auth integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	schema := "browser_test_" + strings.ToLower(browserHash(randomBrowserToken())[:16])
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["TimeZone"] = "Europe/London"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(ctx, `CREATE TABLE users(id varchar(255) PRIMARY KEY,company_id varchar(255),email varchar(255) UNIQUE NOT NULL,full_name varchar(255),email_verified bool DEFAULT false,onboarding_completed bool DEFAULT false,first_instrumentation_done bool DEFAULT false,last_login_at timestamp);
 CREATE TABLE service_accounts(id varchar(255) PRIMARY KEY,company_id varchar(255),name text,is_active bool DEFAULT true);
 CREATE TABLE user_roles(principal_id varchar(255),principal_type varchar(50),role_name varchar(100),assigned_by varchar(255),PRIMARY KEY(principal_id,role_name));
 CREATE TABLE api_keys(id varchar(255) PRIMARY KEY,company_id varchar(255),user_id varchar(255),service_account_id varchar(255),key_hash varchar(255) UNIQUE,is_active bool DEFAULT true,revoked_at timestamp,expires_at timestamp);
 CREATE TABLE team_invitations(id varchar(255) PRIMARY KEY,company_id varchar(255),email varchar(255),role varchar(50),status varchar(50),expires_at timestamp,created_at timestamp DEFAULT NOW(),accepted_at timestamp,accepted_by_user_id varchar(255));`)
	require.NoError(t, err)
	f := &browserRegistryFixture{}
	s, err := NewBrowserService(ctx, pool, f)
	require.NoError(t, err)
	return s, f
}
func browserRequest(s *BrowserService, method, path, body string, cookies []*http.Cookie, csrf bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:8083"+path, strings.NewReader(body))
	r.Header.Set("Origin", s.origin)
	for _, c := range cookies {
		r.AddCookie(c)
		if csrf && strings.Contains(c.Name, "csrf") {
			r.Header.Set(BrowserCSRFHeader, c.Value)
		}
	}
	w := httptest.NewRecorder()
	s.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := s.Authenticate(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if e != nil {
			w.WriteHeader(401)
			return
		}
		browserJSON(w, 200, c)
	})).ServeHTTP(w, r)
	return w
}

func TestBrowserKeySessionLifecycle(t *testing.T) {
	s, f := browserFixture(t)
	ctx := context.Background()
	denied := browserRequest(s, "POST", "/auth/api-key/exchange", `{"api_key":"invalid"}`, nil, false)
	require.Equal(t, 401, denied.Code)
	login := browserRequest(s, "POST", "/auth/api-key/exchange", `{"api_key":"test-license"}`, nil, false)
	require.Equal(t, 200, login.Code, login.Body.String())
	require.NotContains(t, login.Body.String(), sessionPrefix)
	cookies := login.Result().Cookies()
	require.Len(t, cookies, 2)
	require.True(t, cookies[0].HttpOnly)
	require.False(t, cookies[1].HttpOnly)
	require.Equal(t, http.SameSiteStrictMode, cookies[0].SameSite)
	token := cookies[0].Value
	claims, err := s.Authenticate(ctx, token)
	require.NoError(t, err)
	require.Equal(t, []string{"admin"}, claims.Roles)
	require.Equal(t, "user", claims.PrincipalType)
	var stored string
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT token_hash FROM threadify_browser_sessions`).Scan(&stored))
	require.NotEqual(t, token, stored)
	// An independently initialized Web API accepts the same durable session.
	replica, err := NewBrowserService(ctx, s.pool, f)
	require.NoError(t, err)
	require.Equal(t, 200, browserRequest(replica, "GET", "/api/user/profile", "", cookies, false).Code)
	require.Equal(t, 403, browserRequest(s, "POST", "/graphql", "{}", cookies, false).Code)
	require.Equal(t, 200, browserRequest(s, "POST", "/graphql", "{}", cookies, true).Code)
	require.Equal(t, 403, browserRequest(s, "POST", "/auth/logout", "{}", cookies, false).Code)
	require.Equal(t, 410, browserRequest(s, "POST", "/api/auth/login", "{}", nil, false).Code)
	// Role changes are effective immediately, including on the other replica.
	_, err = s.pool.Exec(ctx, `UPDATE user_roles SET role_name='viewer' WHERE principal_id=$1`, claims.UserID)
	require.NoError(t, err)
	updated, err := replica.Authenticate(ctx, token)
	require.NoError(t, err)
	require.Equal(t, []string{"viewer"}, updated.Roles)
	f.suspended = true
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	// Local logout remains available when Registry suspends access.
	require.Equal(t, 200, browserRequest(s, "POST", "/auth/logout", "{}", cookies, true).Code)
	f.suspended = false
	_, err = replica.Authenticate(ctx, token)
	require.Error(t, err)
	require.Equal(t, 0, f.logoutCalls)
}

func TestBrowserServiceKeyPreservesAuthorityAndRevocation(t *testing.T) {
	s, _ := browserFixture(t)
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `INSERT INTO service_accounts(id,company_id,name) VALUES('reader','company','Reader'); INSERT INTO user_roles VALUES('reader','service_account','reader','reader')`)
	require.NoError(t, err)
	_, err = s.pool.Exec(ctx, `INSERT INTO api_keys(id,company_id,service_account_id,key_hash) VALUES('key','company','reader',$1)`, browserHash("service-key"))
	require.NoError(t, err)
	token, _, err := s.ExchangeKey(ctx, "service-key")
	require.NoError(t, err)
	c, err := s.Authenticate(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "service_account", c.PrincipalType)
	require.Equal(t, []string{"reader"}, c.Roles)
	// A legacy creator user_id on a service key must never turn it into a human administrator.
	_, err = s.pool.Exec(ctx, `UPDATE api_keys SET user_id='creator' WHERE id='key'`)
	require.NoError(t, err)
	legacy, _, err := s.ExchangeKey(ctx, "service-key")
	require.NoError(t, err)
	legacyClaims, err := s.Authenticate(ctx, legacy)
	require.NoError(t, err)
	require.Equal(t, "service_account", legacyClaims.PrincipalType)
	_, err = s.pool.Exec(ctx, `UPDATE api_keys SET revoked_at=NOW() WHERE id='key'`)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	_, _, err = s.ExchangeKey(ctx, "service-key")
	require.Error(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE api_keys SET revoked_at=NULL,expires_at=NOW()-interval '1 minute'`)
	require.NoError(t, err)
	_, _, err = s.ExchangeKey(ctx, "service-key")
	require.Error(t, err)
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	_, err = s.AuthenticateAPIKey(ctx, "service-key")
	require.Error(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE api_keys SET expires_at=NULL; UPDATE service_accounts SET is_active=false`)
	require.NoError(t, err)
	_, _, err = s.ExchangeKey(ctx, "service-key")
	require.Error(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE service_accounts SET is_active=true; DELETE FROM user_roles`)
	require.NoError(t, err)
	_, _, err = s.ExchangeKey(ctx, "service-key")
	require.Error(t, err)
}

func TestBrowserManagedLoginBindingConsumptionAndLogout(t *testing.T) {
	s, f := browserFixture(t)
	start := browserRequest(s, "POST", "/auth/managed/start", "{}", nil, false)
	require.Equal(t, 201, start.Code, start.Body.String())
	var result map[string]any
	require.NoError(t, json.Unmarshal(start.Body.Bytes(), &result))
	payload, _ := json.Marshal(map[string]any{"transaction_id": result["transaction_id"], "poll_token": result["poll_token"]})
	require.NotContains(t, start.Body.String(), f.verifier)
	require.Equal(t, 403, browserRequest(s, "POST", "/auth/managed/poll", string(payload), nil, false).Code)
	f.pending = true
	require.Equal(t, 202, browserRequest(s, "POST", "/auth/managed/poll", string(payload), start.Result().Cookies(), false).Code)
	f.pending = false
	// Two simultaneous poll attempts must produce exactly one session.
	results := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- browserRequest(s, "POST", "/auth/managed/poll", string(payload), start.Result().Cookies(), false)
		}()
	}
	a, b := <-results, <-results
	if a.Code != 200 {
		a, b = b, a
	}
	require.Equal(t, 200, a.Code, a.Body.String())
	require.Equal(t, 403, b.Code, b.Body.String())
	var token string
	for _, c := range a.Result().Cookies() {
		if strings.Contains(c.Name, "session") {
			token = c.Value
		}
	}
	var ciphertext string
	require.NoError(t, s.pool.QueryRow(context.Background(), `SELECT logout_ciphertext FROM threadify_browser_sessions WHERE token_hash=$1`, browserHash(token)).Scan(&ciphertext))
	require.NotContains(t, ciphertext, "logout-capability")
	logout := browserRequest(s, "POST", "/auth/logout", "{}", a.Result().Cookies(), true)
	require.Equal(t, 200, logout.Code, logout.Body.String())
	require.Equal(t, 1, f.logoutCalls)
	_, err := s.Authenticate(context.Background(), token)
	require.Error(t, err)
}

func TestBrowserManagedMembershipAndExpiry(t *testing.T) {
	s, f := browserFixture(t)
	ctx := context.Background()
	flow, err := s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = "stranger@example.test"
	_, _, err = s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.Error(t, err)
	ownerToken, _, err := s.ExchangeKey(ctx, "test-license")
	require.NoError(t, err)
	owner, err := s.Authenticate(ctx, ownerToken)
	require.NoError(t, err)
	invited, created, err := s.CreateInvitedUser(ctx, owner, "stranger@example.test", "Stranger", "viewer")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "invited", invited.Status)
	flow, err = s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = "stranger@example.test"
	token, _, err := s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.NoError(t, err)
	c, err := s.Authenticate(ctx, token)
	require.NoError(t, err)
	require.Equal(t, []string{"viewer"}, c.Roles)
	var status string
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT status FROM users WHERE id=$1`, invited.ID).Scan(&status))
	require.Equal(t, "active", status)
	s.now = func() time.Time { return time.Now().Add(9 * time.Hour) }
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	s.now = time.Now
	_, err = s.pool.Exec(ctx, `DELETE FROM user_roles WHERE principal_id=$1`, c.UserID)
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	flow, err = s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = "stranger@example.test"
	_, _, err = s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.Error(t, err)
}

func TestBrowserManagedLoginReportsFailureBoundary(t *testing.T) {
	s, f := browserFixture(t)
	for _, tc := range []struct {
		name, code string
		status     int
		setup      func()
	}{
		{"membership", "managed_login_denied", 403, func() { f.assertion.VerifiedEmail = "uninvited@example.test" }},
		{"assertion", "managed_login_denied", 403, func() { f.assertion.AccountID = "other-account" }},
		{"registry", "managed_login_unavailable", 503, func() { f.exchangeErr = registry.ErrIdentityUnavailable }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.exchangeErr = nil
			start := browserRequest(s, "POST", "/auth/managed/start", "{}", nil, false)
			require.Equal(t, 201, start.Code)
			var flow map[string]any
			require.NoError(t, json.Unmarshal(start.Body.Bytes(), &flow))
			tc.setup()
			payload, err := json.Marshal(map[string]any{"transaction_id": flow["transaction_id"], "poll_token": flow["poll_token"]})
			require.NoError(t, err)
			result := browserRequest(s, "POST", "/auth/managed/poll", string(payload), start.Result().Cookies(), false)
			require.Equal(t, tc.status, result.Code, result.Body.String())
			require.JSONEq(t, `{"error":"`+tc.code+`"}`, result.Body.String())
			require.Empty(t, result.Result().Cookies())
		})
	}
}

func TestBrowserAssertionAndCookieBoundaries(t *testing.T) {
	f := &browserRegistryFixture{}
	s := &BrowserService{registry: f, key: f.BrowserKey(), now: time.Now}
	tx, err := f.StartIdentity(context.Background(), "verifier", "local")
	require.NoError(t, err)
	good := f.assertion
	require.True(t, s.validAssertion(good, "remote", "local", tx.ExpiresAt))
	cases := map[string]func(*registry.IdentityAssertion){
		"account": func(a *registry.IdentityAssertion) { a.AccountID = "other" }, "installation": func(a *registry.IdentityAssertion) { a.InstallationID = "other" },
		"purpose": func(a *registry.IdentityAssertion) { a.Purpose = "enroll" }, "transaction": func(a *registry.IdentityAssertion) { a.TransactionID = "other" },
		"enrollment": func(a *registry.IdentityAssertion) { a.EnrollmentRef = "other" }, "provider": func(a *registry.IdentityAssertion) { a.Provider = "other" },
		"expiry": func(a *registry.IdentityAssertion) { a.ExpiresAt = time.Now().Add(-time.Minute) }, "issuer": func(a *registry.IdentityAssertion) { a.Issuer = "http://public.example.test" },
		"logout": func(a *registry.IdentityAssertion) { a.LogoutToken = "" }, "method": func(a *registry.IdentityAssertion) { a.AuthMethod = "password" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			a := good
			mutate(&a)
			require.False(t, s.validAssertion(a, "remote", "local", tx.ExpiresAt))
		})
	}
	s.origin = "https://threadify.example.test"
	r := httptest.NewRequest("POST", s.origin+"/graphql", nil)
	r.Header.Set("Origin", s.origin)
	w := httptest.NewRecorder()
	s.setSession(w, r, "opaque", time.Now().Add(time.Hour))
	cookies := w.Result().Cookies()
	require.True(t, cookies[0].Secure)
	require.True(t, cookies[0].HttpOnly)
	require.Equal(t, "__Host-threadify_session", cookies[0].Name)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	r.Header.Set(BrowserCSRFHeader, cookies[1].Value)
	require.True(t, s.validCSRF(r, "opaque"))
	r.Header.Set("Origin", "https://evil.example.test")
	require.False(t, s.validCSRF(r, "opaque"))
	r.Header.Set("Origin", s.origin)
	r.AddCookie(cookies[1])
	require.False(t, s.validCSRF(r, "opaque"))
	encrypted, err := s.encrypt("capability")
	require.NoError(t, err)
	s.key = []byte(strings.Repeat("x", 32))
	_, err = s.decrypt(encrypted)
	require.Error(t, err)
}
