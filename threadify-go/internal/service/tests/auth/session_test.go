package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/service"
	sharedauth "threadify-go/shared/auth"
)

func TestEngineSupabaseSessionFallback(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		local     *sharedauth.TokenClaims
		lookupErr error
		wantErr   bool
	}{
		{"valid", 200, `{"id":"provider-user"}`, &sharedauth.TokenClaims{UserID: "local-user", CompanyID: "local-company", Roles: []string{"untrusted"}}, nil, false},
		{"invalid", 401, `{}`, nil, nil, true},
		{"expired", 403, `{}`, nil, nil, true},
		{"provider unavailable", 503, `{}`, nil, nil, true},
		{"missing provider subject", 200, `{}`, nil, nil, true},
		{"unknown local user", 200, `{"id":"provider-user"}`, nil, errors.New("not found"), true},
		{"missing tenant", 200, `{"id":"provider-user"}`, &sharedauth.TokenClaims{UserID: "local-user"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			providerCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/jwks" {
					_, _ = w.Write([]byte(`{"keys":[]}`))
					return
				}
				providerCalls++
				require.Equal(t, "/auth/v1/user", r.URL.Path)
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "Bearer session.payload.signature", r.Header.Get("Authorization"))
				require.Equal(t, "publishable-key", r.Header.Get("apikey"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			verifier, err := sharedauth.NewSupabaseAccessTokenVerifier(sharedauth.SupabaseAuthConfig{URL: server.URL, PublishableKey: "publishable-key"})
			require.NoError(t, err)
			svc := service.NewAuthService(nil, 60)
			defer svc.Stop()
			svc.SetJWKSVerifier(sharedauth.NewJWKSVerifier(server.URL+"/jwks", "authenticated", server.URL))
			svc.SetSessionVerifier(verifier, func(_ context.Context, sub string) (*sharedauth.TokenClaims, error) {
				require.Equal(t, "provider-user", sub)
				return tc.local, tc.lookupErr
			})
			claims, err := svc.VerifyToken(context.Background(), "session.payload.signature")
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, claims)
			} else {
				require.NoError(t, err)
				require.Equal(t, "local-user", claims.UserID)
				require.Equal(t, "local-company", claims.CompanyID)
				require.Equal(t, "provider-user", claims.AuthUserID)
				require.Empty(t, claims.Roles)
			}
			require.Equal(t, 1, providerCalls)
			_, err = svc.VerifyToken(context.Background(), "td_local-api-key")
			require.Error(t, err)
			require.Equal(t, 1, providerCalls, "API keys must not be sent to the auth provider")
		})
	}
}

func TestEngineSessionRequiresLocalResolver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"id":"provider-user"}`)) }))
	defer server.Close()
	v, err := sharedauth.NewSupabaseAccessTokenVerifier(sharedauth.SupabaseAuthConfig{URL: server.URL, PublishableKey: "public"})
	require.NoError(t, err)
	svc := service.NewAuthService(nil, 60)
	defer svc.Stop()
	svc.SetSessionVerifier(v, nil)
	_, err = svc.VerifyToken(context.Background(), "session.payload.signature")
	require.ErrorIs(t, err, service.ErrDatabaseNotConfigured)
}
