package oauthprovider

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Usefused/fused-open-core/oauthserver"
	"github.com/google/uuid"
	"threadify-go/shared/rbac"
)

type testRevisions struct{}

func (testRevisions) SetRevision(int64) bool { return true }

// The embedded interface makes any unexpected protocol call fail the test.
// The implemented methods model a single client and a single-use code.
type testStore struct {
	oauthserver.Store[string]
	client     oauthserver.OAuthClient
	secretHash string
	consent    oauthserver.OAuthConsent[string]
	hasConsent bool
	code       oauthserver.OAuthAuthorizationCodeIssue[string]
	used       bool
}

func (s *testStore) CreateOAuthClient(_ context.Context, input oauthserver.OAuthClientRegistration[string]) (oauthserver.OAuthClientMutationResult, error) {
	s.client = oauthserver.OAuthClient{ID: uuid.New(), ClientID: input.ClientID, Name: input.Name, ClientType: input.ClientType, RedirectURIs: input.RedirectURIs, AllowedScopes: input.AllowedScopes, HasSecret: input.ClientSecretHash != "", CreatedAt: time.Now()}
	s.secretHash = input.ClientSecretHash
	return oauthserver.OAuthClientMutationResult{Client: s.client, AuthorizationRevision: 1}, nil
}
func (s *testStore) GetOAuthClientByPublicID(_ context.Context, publicID string) (oauthserver.OAuthClient, string, error) {
	if publicID != s.client.ClientID {
		return oauthserver.OAuthClient{}, "", oauthserver.ErrOAuthClientNotFound
	}
	return s.client, s.secretHash, nil
}
func (s *testStore) GetOAuthUserConsent(_ context.Context, clientID uuid.UUID, userID string) (oauthserver.OAuthConsent[string], bool, error) {
	return s.consent, s.hasConsent && s.consent.ClientID == clientID && s.consent.SubjectID == userID, nil
}
func (s *testStore) RecordOAuthConsentAndIssueCode(_ context.Context, consent oauthserver.OAuthConsentGrant[string], code oauthserver.OAuthAuthorizationCodeIssue[string]) error {
	s.consent = oauthserver.OAuthConsent[string]{ClientID: consent.ClientID, SubjectID: consent.SubjectID, GrantedScope: consent.GrantedScope, GrantedAt: time.Now()}
	s.hasConsent = true
	s.code = code
	s.used = false
	return nil
}
func (s *testStore) ExchangeOAuthAuthorizationCode(_ context.Context, input oauthserver.OAuthCodeExchange, now time.Time) (oauthserver.OAuthTokenMetadata[string], error) {
	verifierHash := sha256.Sum256([]byte(input.CodeVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(verifierHash[:])
	if s.used || input.CodeHash != s.code.CodeHash || input.ClientID != s.code.ClientID || input.RedirectURI != s.code.RedirectURI || challenge != s.code.CodeChallenge || !now.Before(s.code.ExpiresAt) {
		return oauthserver.OAuthTokenMetadata[string]{}, oauthserver.ErrOAuthGrantDenied
	}
	s.used = true
	return oauthserver.OAuthTokenMetadata[string]{ID: uuid.New(), ClientID: s.code.ClientID, SubjectID: s.code.SubjectID, TokenFamilyID: uuid.New(), Scope: s.code.Scope, AccessExpiresAt: input.Issue.AccessExpiresAt, RefreshExpiresAt: input.Issue.RefreshExpiresAt, AuthorizationRevision: 2}, nil
}

func TestThreadifySharedOAuthFlowAndLiveRoles(t *testing.T) {
	loader, err := rbac.NewEmbeddedLoader()
	if err != nil {
		t.Fatal(err)
	}
	roles := []string{"viewer"}
	roleSource := func(_ context.Context, userID string) ([]string, error) {
		if userID != "threadify-user-1" || len(roles) == 0 {
			return nil, errors.New("inactive member")
		}
		return roles, nil
	}
	policy, err := NewPolicy(loader, roleSource, PilotScopes())
	if err != nil {
		t.Fatal(err)
	}
	store := &testStore{}
	service, err := NewService(store, testRevisions{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	actor := policy.Actor("threadify-user-1", "session-hash")
	registered, err := service.CreateClient(t.Context(), actor, oauthserver.CreateClientInput{Name: "Contract integration", ClientType: oauthserver.OAuthClientConfidential, RedirectURIs: []string{"https://client.example/callback"}, AllowedScopes: PilotScopes()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(registered.Client.ClientID, "toc_") || !strings.HasPrefix(registered.ClientSecret, "tos_") {
		t.Fatalf("Threadify credential namespace missing: %+v", registered)
	}
	verifier := strings.Repeat("v", 43)
	challengeBytes := sha256.Sum256([]byte(verifier))
	request := oauthserver.AuthorizeRequest{ClientID: registered.Client.ClientID, RedirectURI: "https://client.example/callback", ResponseType: "code", Scope: PilotScopes(), State: "state-123", CodeChallenge: base64.RawURLEncoding.EncodeToString(challengeBytes[:]), CodeChallengeMethod: "S256"}
	authorization, err := service.Authorize(t.Context(), actor, request)
	if err != nil || !authorization.RequiresConsent {
		t.Fatalf("authorize = %+v, %v", authorization, err)
	}
	redirect, err := service.Consent(t.Context(), actor, oauthserver.ConsentRequest{ClientID: request.ClientID, RedirectURI: request.RedirectURI, Scope: request.Scope, State: request.State, CodeChallenge: request.CodeChallenge, CodeChallengeMethod: request.CodeChallengeMethod})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(redirect)
	if err != nil || parsed.Query().Get("state") != request.State {
		t.Fatalf("consent redirect = %q, %v", redirect, err)
	}
	issuedCode := parsed.Query().Get("code")
	codeHash := sha256.Sum256([]byte(issuedCode))
	if issuedCode == "" || store.code.CodeHash != hex.EncodeToString(codeHash[:]) || store.code.CodeHash == issuedCode {
		t.Fatal("authorization code was not stored by hash")
	}
	secretHash := sha256.Sum256([]byte(registered.ClientSecret))
	if store.secretHash != hex.EncodeToString(secretHash[:]) || store.secretHash == registered.ClientSecret {
		t.Fatal("client secret was not stored by hash")
	}
	tokenRequest := oauthserver.TokenRequest{GrantType: "authorization_code", ClientID: request.ClientID, ClientSecret: registered.ClientSecret, Code: parsed.Query().Get("code"), RedirectURI: request.RedirectURI, CodeVerifier: verifier}
	tokens, err := service.Token(t.Context(), tokenRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tokens.AccessToken, "toat_") || !strings.HasPrefix(tokens.RefreshToken, "tort_") || len(tokens.Scope) != 2 || tokens.Scope[0] != ScopeContractReadAll || tokens.Scope[1] != ScopeMCPRead || store.code.SubjectID != "threadify-user-1" {
		t.Fatalf("unexpected Threadify token result: %+v", tokens)
	}
	if _, err := service.Token(t.Context(), tokenRequest); !errors.Is(err, oauthserver.ErrInvalidGrant) {
		t.Fatalf("authorization code replay = %v", err)
	}
	if !policy.CanUse(t.Context(), actor.SubjectID, tokens.Scope, "contract.read.contract-123") {
		t.Fatal("viewer should be able to use contract read grant")
	}
	if policy.CanUse(t.Context(), actor.SubjectID, tokens.Scope, "contract.delete.contract-123") {
		t.Fatal("contract write escaped read-only grant")
	}
	roles = nil
	if policy.CanUse(t.Context(), actor.SubjectID, tokens.Scope, "contract.read.contract-123") {
		t.Fatal("removed membership retained OAuth access")
	}
	if _, err := service.Authorize(t.Context(), actor, request); !errors.Is(err, oauthserver.ErrAccessDenied) {
		t.Fatalf("removed membership can authorize: %v", err)
	}
}

func TestThreadifyScopeAllowlist(t *testing.T) {
	loader, err := rbac.NewEmbeddedLoader()
	if err != nil {
		t.Fatal(err)
	}
	roles := func(context.Context, string) ([]string, error) { return []string{"admin"}, nil }
	if _, err := NewPolicy(loader, roles, []string{"not.a.permission"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
	policy, err := NewPolicy(loader, roles, PilotScopes())
	if err != nil {
		t.Fatal(err)
	}
	if policy.ValidateScope("apikey.create") == nil {
		t.Fatal("administrative permission became delegable")
	}
	if policy.CanUse(t.Context(), "user", []string{"apikey.create"}, "apikey.create") {
		t.Fatal("unallowlisted stored scope became usable")
	}
	if !policy.CanUse(t.Context(), "user", PilotScopes(), "contract.read.contract-123") {
		t.Fatal("admin role lost permitted read scope")
	}
}
