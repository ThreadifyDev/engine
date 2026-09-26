package auth

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Usefused/fused-open-core/oauthserver"
	"threadify-go/shared/oauthprovider"
	"threadify-go/shared/rbac"
)

type oauthRuntime struct {
	store  *oauthprovider.PostgresStore
	policy *oauthprovider.Policy
}

var currentOAuth atomic.Pointer[oauthRuntime]

type oauthRevisionSink struct{}

func (oauthRevisionSink) SetRevision(int64) bool { return true }

// OAuthServer mounts Threadify's shared-protocol OAuth endpoints outside the
// browser cookie wrapper, so a native consent form can use same-origin checks.
func (s *BrowserService) OAuthServer(ctx context.Context, loader *rbac.Loader, publicURL string, next http.Handler) (http.Handler, error) {
	store, err := oauthprovider.NewPostgresStore(ctx, s.pool, s.registry.CompanyID(), s.registry.InstallationID())
	if err != nil {
		return nil, err
	}
	roles, err := oauthprovider.DatabaseRoleSource(s.pool, s.registry.CompanyID())
	if err != nil {
		return nil, err
	}
	policy, err := oauthprovider.NewPolicy(loader, roles, oauthprovider.PilotScopes())
	if err != nil {
		return nil, err
	}
	service, err := oauthprovider.NewService(store, oauthRevisionSink{}, policy)
	if err != nil {
		return nil, err
	}
	server := &oauthprovider.HTTPServer{Service: service, Store: store, Policy: policy, PublicURL: publicURL, AllowRequest: s.allowAuthRequest,
		Browser: func(r *http.Request) (oauthprovider.BrowserActor, error) {
			cookie := oneBrowserCookie(r, s.cookieName(r, "session"))
			if cookie == "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
				return oauthprovider.BrowserActor{}, ErrBrowserAuth
			}
			claims, err := s.Authenticate(r.Context(), cookie)
			if err != nil || claims.PrincipalType != "user" {
				return oauthprovider.BrowserActor{}, ErrBrowserAuth
			}
			return oauthprovider.BrowserActor{UserID: claims.UserID, CredentialHash: browserHash(cookie), Admin: userHasRole(claims, "admin")}, nil
		},
		SameOrigin: s.sameOrigin,
		ValidCSRF:  func(r *http.Request) bool { return s.validCSRF(r, oneBrowserCookie(r, s.cookieName(r, "session"))) },
	}
	currentOAuth.Store(&oauthRuntime{store: store, policy: policy})
	return server.Wrap(next), nil
}

// VerifyOAuthAccess resolves a live token row and current user membership.
// Route permissions must still be checked with OAuthCanUse.
func VerifyOAuthAccess(ctx context.Context, token string) (*TokenClaims, error) {
	runtime := currentOAuth.Load()
	browser := currentBrowser.Load()
	if runtime == nil || browser == nil || !strings.HasPrefix(token, oauthprovider.Prefixes.AccessToken) {
		return nil, oauthserver.ErrInvalidGrant
	}
	userID, scopes, err := runtime.store.AccessGrant(ctx, token)
	if err != nil {
		return nil, oauthserver.ErrInvalidGrant
	}
	claims, err := browser.principalClaims(ctx, userID, "user", time.Now().Add(time.Hour))
	if err != nil {
		return nil, oauthserver.ErrInvalidGrant
	}
	claims.OAuthAccess = true
	claims.OAuthScopes = scopes
	return claims, nil
}

func OAuthCanUse(ctx context.Context, claims *TokenClaims, permission string) bool {
	runtime := currentOAuth.Load()
	return runtime != nil && claims != nil && len(claims.OAuthScopes) > 0 && runtime.policy.CanUse(ctx, claims.UserID, claims.OAuthScopes, permission)
}
