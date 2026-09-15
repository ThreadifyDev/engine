package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupabaseVerifyAccessToken(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"valid", 200, `{"id":"auth-user","email":"test@example.invalid","email_confirmed_at":"2026-09-04T12:00:00Z"}`, false},
		{"invalid", 401, `{"error_code":"bad_jwt"}`, true},
		{"expired", 403, `{"error_code":"bad_jwt"}`, true},
		{"missing_identity", 200, `{}`, true},
		{"provider_unavailable", 503, `{"error":"unavailable"}`, true},
		{"malformed_response", 200, `not json`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/auth/v1/user" ||
					r.Header.Get("Authorization") != "Bearer test-user-session" ||
					r.Header.Get("apikey") != "publishable-test-key" {
					t.Error("Session verification must use the user bearer token and publishable key")
					w.WriteHeader(400)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewSupabaseClient(SupabaseAuthConfig{URL: server.URL, PublishableKey: "publishable-test-key", SecretKey: "admin-test-key"})
			require.NoError(t, err)
			info, err := client.(AccessTokenVerifier).VerifyAccessToken(context.Background(), "test-user-session")
			if tc.wantError {
				require.Error(t, err)
				require.Nil(t, info)
			} else {
				require.NoError(t, err)
				require.Equal(t, "auth-user", info.Sub)
				require.True(t, info.EmailVerified)
			}
		})
	}
}
