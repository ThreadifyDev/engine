package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuth_ResetPassword_SuccessAndSingleUse(t *testing.T) {
	user := setupAuthenticatedUser(t)
	response := doJSON(t, http.MethodPost, "/api/auth/forgot-password", map[string]any{"email": user.Email})
	require.Equal(t, http.StatusOK, response.StatusCode, string(response.Body))
	token := supabase.GetResetToken(user.Email)
	require.NotEmpty(t, token)
	delivered := false
	for _, message := range plunk.MessagesTo(user.Email) {
		if strings.Contains(message.Body, token) {
			delivered = true
		}
	}
	require.True(t, delivered, "reset link must reach the local email sink")
	const replacement = "Replacement123!@#"
	response = doJSON(t, http.MethodPost, "/api/auth/reset-password", map[string]any{"token": token, "password": replacement})
	require.Equal(t, http.StatusOK, response.StatusCode, string(response.Body))
	old := doJSON(t, http.MethodPost, "/api/auth/login", map[string]any{"email": user.Email, "password": user.Password})
	require.Equal(t, http.StatusUnauthorized, old.StatusCode, string(old.Body))
	session := loginUser(t, user.Email, replacement)
	require.NotEmpty(t, session["token"])
	replay := doJSON(t, http.MethodPost, "/api/auth/reset-password", map[string]any{"token": token, "password": "Different123!@#"})
	require.Equal(t, http.StatusBadRequest, replay.StatusCode, string(replay.Body))
	// Failed replay must not replace the successfully set password.
	require.NotEmpty(t, loginUser(t, user.Email, replacement)["token"])
}
