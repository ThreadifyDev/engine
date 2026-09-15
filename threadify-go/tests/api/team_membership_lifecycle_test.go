package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTeamInvitations_AcceptAndRemoveMember(t *testing.T) {
	owner := setupAuthenticatedUser(t)
	email := uniqueEmail()
	response := doJSONWithAuth(t, http.MethodPost, "/api/team/invitations", map[string]any{"email": email, "role": "member"}, owner.AccessToken)
	require.Equal(t, http.StatusCreated, response.StatusCode, string(response.Body))
	invitationID := decodeJSONBody(t, response)["invitationId"].(string)
	listed := doRawWithAuth(t, http.MethodGet, "/api/team/invitations", nil, "", owner.AccessToken)
	token := findInvitationByID(t, listed, invitationID)["token"].(string)
	require.Eventually(t, func() bool { return len(plunk.MessagesTo(email)) > 0 }, 10*time.Second, 100*time.Millisecond, "invitation should reach only the local email sink")
	const password = "InvitedUser123!@#"
	response = doJSON(t, http.MethodPost, "/api/auth/signup", map[string]any{"email": email, "password": password, "full_name": "Invited Test Member", "invitation_token": token})
	require.Equal(t, http.StatusCreated, response.StatusCode, string(response.Body))
	waitForAuthUser(t, email)
	session := loginUser(t, email, password)
	member := session["user"].(map[string]any)
	require.Equal(t, owner.CompanyID, member["company_id"])
	memberID := member["id"].(string)
	memberToken := session["token"].(string)
	listed = doRawWithAuth(t, http.MethodGet, "/api/team/members", nil, "", owner.AccessToken)
	require.Equal(t, http.StatusOK, listed.StatusCode, string(listed.Body))
	require.Contains(t, string(listed.Body), memberID)
	reused := doJSON(t, http.MethodPost, "/api/team/invitation/validate", map[string]any{"token": token})
	require.Equal(t, http.StatusBadRequest, reused.StatusCode, string(reused.Body))
	outsider := setupAuthenticatedUser(t)
	denied := doRawWithAuth(t, http.MethodDelete, "/api/team/members/"+memberID, nil, "", outsider.AccessToken)
	require.Equal(t, http.StatusForbidden, denied.StatusCode, string(denied.Body))
	removed := doRawWithAuth(t, http.MethodDelete, "/api/team/members/"+memberID, nil, "", owner.AccessToken)
	require.Equal(t, http.StatusOK, removed.StatusCode, string(removed.Body))
	listed = doRawWithAuth(t, http.MethodGet, "/api/team/members", nil, "", owner.AccessToken)
	require.NotContains(t, string(listed.Body), email)
	revoked := doRawWithAuth(t, http.MethodGet, "/api/user/profile?minimal=true", nil, "", memberToken)
	require.Equal(t, http.StatusUnauthorized, revoked.StatusCode, string(revoked.Body))
}
