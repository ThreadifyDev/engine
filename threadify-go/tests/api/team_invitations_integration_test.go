package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findInvitationByID(t *testing.T, listResp httpResponse, invitationID string) map[string]any {
	t.Helper()

	body := decodeJSONBody(t, listResp)
	raw := body["invitations"]
	if raw == nil {
		return nil
	}

	items, ok := raw.([]any)
	require.True(t, ok, "expected invitations to be an array: %s", string(listResp.Body))

	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if m["id"] == invitationID {
			return m
		}
	}
	return nil
}

func TestTeamInvitations_Lifecycle(t *testing.T) {
	user := setupAuthenticatedUser(t)

	t.Run("invalid_email_rejected", func(t *testing.T) {
		resp := doJSONWithAuth(t, http.MethodPost, "/api/team/invitations", map[string]any{
			"email": "not-an-email",
			"role":  "member",
		}, user.AccessToken)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		body := decodeJSONBody(t, resp)
		assert.NotEmpty(t, body["error"], "bad request must carry an error message")
	})

	// Send invitation
	inviteEmail := uniqueEmail()
	sendResp := doJSONWithAuth(t, http.MethodPost, "/api/team/invitations", map[string]any{
		"email": inviteEmail,
		"role":  "member",
	}, user.AccessToken)
	require.Equal(t, http.StatusCreated, sendResp.StatusCode, string(sendResp.Body))

	sendBody := decodeJSONBody(t, sendResp)
	assert.Equal(t, true, sendBody["success"])
	assert.NotEmpty(t, sendBody["expiresAt"], "send response must include expiresAt")
	assert.Equal(t, "Invitation sent successfully", sendBody["message"])

	invID, ok := sendBody["invitationId"].(string)
	require.True(t, ok, "invitationId must be a string")
	require.NotEmpty(t, invID)

	// List — verify the invitation is present with correct shape
	list1 := doRawWithAuth(t, http.MethodGet, "/api/team/invitations", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, list1.StatusCode, string(list1.Body))

	item := findInvitationByID(t, list1, invID)
	require.NotNil(t, item, "expected invitation %q in list response: %s", invID, string(list1.Body))

	assert.Equal(t, inviteEmail, item["email"], "listed invitation must carry the invited email")
	assert.Equal(t, "member", item["role"], "listed invitation must carry the requested role")
	assert.Equal(t, "pending", item["status"])
	assert.NotEmpty(t, item["invited_by"], "invitation must identify who sent it")
	assert.NotEmpty(t, item["expires_at"], "invitation must have an expires_at timestamp")
	assert.NotEmpty(t, item["created_at"], "invitation must have a created_at timestamp")

	token1, ok := item["token"].(string)
	require.True(t, ok, "token must be a string")
	require.NotEmpty(t, token1)

	expiresAt1, ok := item["expires_at"].(float64)
	require.True(t, ok, "expires_at must be a number")

	// Validate token publicly — response is scoped to company_name and email only
	validateResp := doJSON(t, http.MethodPost, "/api/team/invitation/validate", map[string]any{
		"token": token1,
	})
	require.Equal(t, http.StatusOK, validateResp.StatusCode, string(validateResp.Body))

	validateBody := decodeJSONBody(t, validateResp)
	assert.Equal(t, inviteEmail, validateBody["email"])
	assert.NotEmpty(t, validateBody["company_name"])

	time.Sleep(time.Second)

	resendResp := doRawWithAuth(t, http.MethodPost, "/api/team/invitations/"+invID+"/resend", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, resendResp.StatusCode, string(resendResp.Body))

	resendBody := decodeJSONBody(t, resendResp)
	assert.Equal(t, true, resendBody["success"])
	assert.Equal(t, "Invitation resent successfully", resendBody["message"])

	list2 := doRawWithAuth(t, http.MethodGet, "/api/team/invitations", nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, list2.StatusCode)

	item2 := findInvitationByID(t, list2, invID)
	require.NotNil(t, item2, "expected invitation %q after resend: %s", invID, string(list2.Body))

	token2, ok := item2["token"].(string)
	require.True(t, ok, "token must be a string after resend")
	require.NotEmpty(t, token2)
	assert.NotEqual(t, token1, token2, "resend must rotate the invitation token")
	assert.Equal(t, "pending", item2["status"], "status must remain pending after resend")

	expiresAt2, ok := item2["expires_at"].(float64)
	require.True(t, ok, "expires_at must be a number after resend")
	assert.Greater(t, expiresAt2, expiresAt1, "resend must push out the expiry")

	// Cancel
	cancelResp := doRawWithAuth(t, http.MethodDelete, "/api/team/invitations/"+invID, nil, "", user.AccessToken)
	require.Equal(t, http.StatusOK, cancelResp.StatusCode, string(cancelResp.Body))

	cancelBody := decodeJSONBody(t, cancelResp)
	assert.Equal(t, true, cancelBody["success"], "cancel response must confirm success")
	assert.NotEmpty(t, cancelBody["message"])

	// Verify list is now empty
	list3 := doRawWithAuth(t, http.MethodGet, "/api/team/invitations", nil, "", user.AccessToken)
	list3Body := decodeJSONBody(t, list3)
	assertEmptyOrNilList(t, list3Body, "invitations")

	cancelledItem := findInvitationByID(t, list3, invID)
	assert.Nil(t, cancelledItem, "cancelled invitation must not appear in list")

	// Old token must no longer validate
	validateAfterCancel := doJSON(t, http.MethodPost, "/api/team/invitation/validate", map[string]any{
		"token": token2,
	})
	require.Equal(t, http.StatusBadRequest, validateAfterCancel.StatusCode)

	cancelledValidateBody := decodeJSONBody(t, validateAfterCancel)
	assert.NotEmpty(t, cancelledValidateBody["error"], "cancelled token validation must return an error message")
}
