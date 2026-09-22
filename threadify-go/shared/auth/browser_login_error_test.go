package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"threadify-go/shared/registry"
)

func TestManagedLoginFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		stage  string
		cause  error
		status int
		code   string
	}{
		{"transaction", ErrBrowserAuth, 403, "managed_login_denied"},
		{"membership", ErrBrowserAuth, 403, "managed_login_denied"},
		{"assertion", ErrBrowserAuth, 403, "managed_login_denied"},
		{"registry_exchange", registry.ErrIdentityUnavailable, 503, "managed_login_unavailable"},
		{"registry_exchange", registry.ErrUnverified, 503, "managed_login_unavailable"},
		{"membership", errors.New("SQL error containing private@example.test"), 500, "managed_login_internal_error"},
		{"session", errors.New("private session material"), 500, "managed_login_internal_error"},
	} {
		t.Run(tc.stage+"/"+tc.code, func(t *testing.T) {
			err := &managedLoginError{stage: tc.stage, cause: tc.cause}
			status, code, stage := managedLoginFailure(err)
			require.Equal(t, tc.status, status)
			require.Equal(t, tc.code, code)
			require.Equal(t, tc.stage, stage)
			require.ErrorIs(t, err, tc.cause)
			require.NotContains(t, err.Error(), "private")
		})
	}
}
