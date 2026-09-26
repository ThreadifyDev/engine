package oauthprovider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Usefused/fused-open-core/oauthserver"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"threadify-go/shared/rbac"
)

func postgresFixture(t *testing.T) (*PostgresStore, *pgxpool.Pool, *Policy, *oauthserver.Service[string]) {
	t.Helper()
	dsn := os.Getenv("THREADIFY_OAUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set THREADIFY_OAUTH_TEST_DATABASE_URL for PostgreSQL OAuth integration test")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "threadify_oauth_test_" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "_")
	if len(schema) > 50 {
		schema = schema[:50]
	}
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(ctx, `CREATE TABLE users(id text PRIMARY KEY,company_id text NOT NULL,status text NOT NULL);
 CREATE TABLE user_roles(principal_id text NOT NULL,principal_type text NOT NULL,role_name text NOT NULL,PRIMARY KEY(principal_id,role_name));
 INSERT INTO users(id,company_id,status) VALUES('user-1','company-1','active');
 INSERT INTO user_roles(principal_id,principal_type,role_name) VALUES('user-1','user','viewer')`)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewPostgresStore(ctx, pool, "company-1", "installation-1")
	if err != nil {
		t.Fatal(err)
	}
	source, err := DatabaseRoleSource(pool, "company-1")
	if err != nil {
		t.Fatal(err)
	}
	loader, err := rbac.NewEmbeddedLoader()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := NewPolicy(loader, source, PilotScopes())
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, testRevisions{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	return store, pool, policy, service
}

func TestPostgresOAuthRealGrantLifecycle(t *testing.T) {
	store, pool, policy, service := postgresFixture(t)
	ctx := t.Context()
	actor := policy.Actor("user-1", "browser-session-hash")
	if _, err := service.CreateClient(ctx, actor, oauthserver.CreateClientInput{Name: "Untrusted redirect", ClientType: oauthserver.OAuthClientPublic, RedirectURIs: []string{"http://outside.example/callback"}, AllowedScopes: PilotScopes()}); !errors.Is(err, oauthserver.ErrInvalidOAuthClient) {
		t.Fatalf("non-HTTPS external redirect accepted: %v", err)
	}
	client, err := service.CreateClient(ctx, actor, oauthserver.CreateClientInput{Name: "Contract app", ClientType: oauthserver.OAuthClientConfidential, RedirectURIs: []string{"https://client.example/callback"}, AllowedScopes: PilotScopes()})
	if err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	authReq := oauthserver.AuthorizeRequest{ClientID: client.Client.ClientID, RedirectURI: "https://client.example/callback", ResponseType: "code", Scope: PilotScopes(), State: "opaque-state", CodeChallenge: challenge, CodeChallengeMethod: "S256"}
	result, err := service.Authorize(ctx, actor, authReq)
	if err != nil || !result.RequiresConsent {
		t.Fatalf("authorization = %+v, %v", result, err)
	}
	redirect, err := service.Consent(ctx, actor, oauthserver.ConsentRequest{ClientID: authReq.ClientID, RedirectURI: authReq.RedirectURI, Scope: authReq.Scope, State: authReq.State, CodeChallenge: authReq.CodeChallenge, CodeChallengeMethod: authReq.CodeChallengeMethod})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(redirect)
	if err != nil {
		t.Fatal(err)
	}
	code := parsed.Query().Get("code")
	if code == "" || parsed.Query().Get("state") != "opaque-state" {
		t.Fatal("bad authorization redirect")
	}
	tokenReq := oauthserver.TokenRequest{GrantType: "authorization_code", Code: code, RedirectURI: authReq.RedirectURI, CodeVerifier: verifier, ClientID: authReq.ClientID, ClientSecret: client.ClientSecret}
	wrong := tokenReq
	wrong.CodeVerifier = strings.Repeat("x", 43)
	if _, err := service.Token(ctx, wrong); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("wrong verifier: %v", err)
	}
	wrong = tokenReq
	wrong.RedirectURI = "https://evil.example/callback"
	if _, err := service.Token(ctx, wrong); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("wrong redirect: %v", err)
	}
	tokens, err := service.Token(ctx, tokenReq)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tokens.AccessToken, "toat_") || !strings.HasPrefix(tokens.RefreshToken, "tort_") {
		t.Fatal("wrong token namespace")
	}
	if _, err := service.Token(ctx, tokenReq); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("code replay: %v", err)
	}
	userID, grants, err := store.AccessGrant(ctx, tokens.AccessToken)
	if err != nil || userID != "user-1" || !policy.CanUse(ctx, userID, grants, "contract.read.contract-1") {
		t.Fatalf("access grant: %s %v", userID, err)
	}
	if policy.CanUse(ctx, userID, grants, "contract.delete.contract-1") {
		t.Fatal("write permission escaped read scope")
	}
	apps, err := service.ListConnectedApps(ctx, actor)
	if err != nil || len(apps) != 1 {
		t.Fatalf("connected apps: %+v, %v", apps, err)
	}
	second, err := service.Authorize(ctx, actor, authReq)
	if err != nil || second.RequiresConsent || second.RedirectURL == "" {
		t.Fatalf("repeat authorize: %+v, %v", second, err)
	}
	refreshReq := oauthserver.TokenRequest{GrantType: "refresh_token", RefreshToken: tokens.RefreshToken, ClientID: authReq.ClientID, ClientSecret: client.ClientSecret}
	rotated, err := service.Token(ctx, refreshReq)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RefreshToken == tokens.RefreshToken || rotated.AccessToken == tokens.AccessToken {
		t.Fatal("refresh did not rotate")
	}
	if _, err := service.Token(ctx, refreshReq); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("refresh replay: %v", err)
	}
	if _, _, err := store.AccessGrant(ctx, rotated.AccessToken); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("replayed family still valid: %v", err)
	}
	if _, err := service.Token(ctx, oauthserver.TokenRequest{GrantType: "refresh_token", RefreshToken: rotated.RefreshToken, ClientID: authReq.ClientID, ClientSecret: client.ClientSecret}); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("family refresh still valid: %v", err)
	}
	if err := service.RevokeConnectedApp(ctx, actor, client.Client.ID); err != nil {
		t.Fatal(err)
	}
	apps, err = service.ListConnectedApps(ctx, actor)
	if err != nil || len(apps) != 0 {
		t.Fatalf("disconnect: %+v %v", apps, err)
	}
	afterDisconnect, err := service.Authorize(ctx, actor, authReq)
	if err != nil || !afterDisconnect.RequiresConsent {
		t.Fatalf("consent persisted after disconnect: %+v %v", afterDisconnect, err)
	}
	// Role loss and suspension apply without changing or reissuing an OAuth grant.
	_, err = pool.Exec(ctx, `DELETE FROM user_roles WHERE principal_id='user-1'`)
	if err != nil {
		t.Fatal(err)
	}
	if policy.CanUse(ctx, userID, grants, "contract.read.contract-1") {
		t.Fatal("removed role still grants access")
	}
	if _, err := service.Authorize(ctx, actor, authReq); !errors.Is(err, oauthserver.ErrAccessDenied) {
		t.Fatalf("removed role authorize: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO user_roles(principal_id,principal_type,role_name) VALUES('user-1','user','viewer')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE users SET status='suspended' WHERE id='user-1'`)
	if err != nil {
		t.Fatal(err)
	}
	if policy.CanUse(ctx, userID, grants, "contract.read.contract-1") {
		t.Fatal("suspended user still grants access")
	}
	_, err = pool.Exec(ctx, `UPDATE users SET status='active' WHERE id='user-1'`)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeClient(ctx, actor, client.Client.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authorize(ctx, actor, authReq); !errors.Is(err, oauthserver.ErrUnauthorizedClient) {
		t.Fatalf("revoked client authorize: %v", err)
	}
	if _, _, err := store.AccessGrant(ctx, tokens.AccessToken); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("revoked client access: %v", err)
	}
	// A second installation must not discover this client's registration.
	other, err := NewPostgresStore(ctx, pool, "company-1", "installation-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.GetOAuthClientByPublicID(ctx, client.Client.ClientID); !errors.Is(err, oauthserver.ErrOAuthClientNotFound) {
		t.Fatalf("cross-installation client lookup: %v", err)
	}
	if _, _, err := other.AccessGrant(ctx, tokens.AccessToken); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("cross-installation access: %v", err)
	}
	_, err = store.ExpireOAuthArtifacts(ctx, time.Now().Add(32*24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
}
