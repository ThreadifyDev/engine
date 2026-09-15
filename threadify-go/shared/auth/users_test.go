package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func userTestOwner(t *testing.T, s *BrowserService) (*TokenClaims, string) {
	t.Helper()
	token, _, err := s.ExchangeKey(context.Background(), "test-license")
	require.NoError(t, err)
	owner, err := s.Authenticate(context.Background(), token)
	require.NoError(t, err)
	return owner, token
}
func TestEngineInvitationsAreLocalUsers(t *testing.T) {
	s, f := browserFixture(t)
	ctx := context.Background()
	owner, _ := userTestOwner(t, s)
	u, created, err := s.CreateInvitedUser(ctx, owner, "Invited@Example.test", "Person", "member")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "invited", u.Status)
	again, created, err := s.CreateInvitedUser(ctx, owner, "invited@example.test", "Person", "member")
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, u.ID, again.ID)
	_, _, err = s.CreateInvitedUser(ctx, owner, u.Email, "Changed", "admin")
	require.ErrorIs(t, err, ErrUserConflict)
	users, err := s.ListUsers(ctx, owner)
	require.NoError(t, err)
	require.Len(t, users, 2)
	active := "active"
	_, err = s.ChangeUser(ctx, owner, u.ID, UserChange{Status: &active})
	require.ErrorIs(t, err, ErrUserConflict)
	flow, err := s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = "someoneelse@example.test"
	_, _, err = s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.Error(t, err)
	flow, err = s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = u.Email
	token, _, err := s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.NoError(t, err)
	member, err := s.Authenticate(ctx, token)
	require.NoError(t, err)
	require.Equal(t, u.ID, member.UserID)
	require.Equal(t, []string{"member"}, member.Roles)
	_, _, err = s.CreateInvitedUser(ctx, member, "forbidden@example.test", "", "admin")
	require.ErrorIs(t, err, ErrUserDenied)
	_, _, err = s.CreateInvitedUser(ctx, owner, u.Email, "Person", "member")
	require.ErrorIs(t, err, ErrUserConflict)
	var legacyRows int
	require.NoError(t, s.pool.QueryRow(ctx, `SELECT count(*) FROM team_invitations`).Scan(&legacyRows))
	require.Zero(t, legacyRows)
}
func TestEngineUserSuspensionArchiveAndKeyAccess(t *testing.T) {
	s, f := browserFixture(t)
	ctx := context.Background()
	owner, _ := userTestOwner(t, s)
	u, _, err := s.CreateInvitedUser(ctx, owner, "member@example.test", "Member", "viewer")
	require.NoError(t, err)
	// An invited principal cannot use a key to skip the verified sign-in transition.
	_, err = s.pool.Exec(ctx, `INSERT INTO api_keys(id,company_id,user_id,key_hash) VALUES('human-key','company',$1,$2)`, u.ID, browserHash("human-key"))
	require.NoError(t, err)
	_, _, err = s.ExchangeKey(ctx, "human-key")
	require.Error(t, err)
	_, err = s.AuthenticateAPIKey(ctx, "human-key")
	require.Error(t, err)
	flow, err := s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = u.Email
	token, _, err := s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.NoError(t, err)
	_, err = s.AuthenticateAPIKey(ctx, "human-key")
	require.NoError(t, err)
	state := "suspended"
	u, err = s.ChangeUser(ctx, owner, u.ID, UserChange{Status: &state})
	require.NoError(t, err)
	require.Equal(t, state, u.Status)
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err)
	_, err = s.AuthenticateAPIKey(ctx, "human-key")
	require.Error(t, err)
	_, _, err = s.ExchangeKey(ctx, "human-key")
	require.Error(t, err)
	flow, err = s.StartLogin(ctx)
	require.NoError(t, err)
	f.assertion.VerifiedEmail = u.Email
	_, _, err = s.PollLogin(ctx, flow["transaction_id"].(string), flow["poll_token"].(string))
	require.Error(t, err)
	state = "active"
	_, err = s.ChangeUser(ctx, owner, u.ID, UserChange{Status: &state})
	require.NoError(t, err)
	_, err = s.Authenticate(ctx, token)
	require.Error(t, err, "reactivation must not revive sessions")
	_, err = s.AuthenticateAPIKey(ctx, "human-key")
	require.NoError(t, err)
	state = "archived"
	_, err = s.ChangeUser(ctx, owner, u.ID, UserChange{Status: &state})
	require.NoError(t, err)
	state = "active"
	_, err = s.ChangeUser(ctx, owner, u.ID, UserChange{Status: &state})
	require.ErrorIs(t, err, ErrUserConflict)
	_, _, err = s.CreateInvitedUser(ctx, owner, u.Email, "Member", "viewer")
	require.ErrorIs(t, err, ErrUserConflict)
	_, err = s.AuthenticateAPIKey(ctx, "human-key")
	require.Error(t, err)
}
func TestEngineUserLastAdminAndCompanyIsolation(t *testing.T) {
	s, _ := browserFixture(t)
	ctx := context.Background()
	owner, _ := userTestOwner(t, s)
	archived := "archived"
	viewer := "viewer"
	_, err := s.ChangeUser(ctx, owner, owner.UserID, UserChange{Status: &archived})
	require.ErrorIs(t, err, ErrLastAdmin)
	_, err = s.ChangeUser(ctx, owner, owner.UserID, UserChange{Role: &viewer})
	require.ErrorIs(t, err, ErrLastAdmin)
	other := *owner
	other.CompanyID = "other"
	_, _, err = s.CreateInvitedUser(ctx, &other, "alien@example.test", "", "member")
	require.ErrorIs(t, err, ErrUserDenied)
	_, err = s.pool.Exec(ctx, `INSERT INTO users(id,email,company_id,status) VALUES('alien','alien@example.test','other','active')`)
	require.NoError(t, err)
	_, err = s.ChangeUser(ctx, owner, "alien", UserChange{Status: &archived})
	require.ErrorIs(t, err, ErrUserMissing)
	// Concurrent demotions cannot each observe the other administrator as active.
	_, err = s.pool.Exec(ctx, `INSERT INTO users(id,email,company_id,status) VALUES('admin2','admin2@example.test','company','active'); INSERT INTO user_roles VALUES('admin2','user','admin','admin2')`)
	require.NoError(t, err)
	second := &TokenClaims{UserID: "admin2", CompanyID: "company", PrincipalType: "user", Roles: []string{"admin"}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, actor := range []*TokenClaims{owner, second} {
		wg.Add(1)
		go func(a *TokenClaims) {
			defer wg.Done()
			_, e := s.ChangeUser(ctx, a, a.UserID, UserChange{Role: &viewer})
			results <- e
		}(actor)
	}
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			require.ErrorIs(t, e, ErrLastAdmin)
			denied++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, denied)
}
func TestEngineUserHTTPAuthorityAndRetiredAPI(t *testing.T) {
	s, _ := browserFixture(t)
	_, token := userTestOwner(t, s)
	handler := s.Wrap(s.UserManagement(http.NotFoundHandler()))
	send := func(path, body string, csrf bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8083"+path, strings.NewReader(body))
		req.Header.Set("Origin", s.origin)
		req.AddCookie(&http.Cookie{Name: "threadify_session_dev", Value: token})
		if csrf {
			proof := s.signed("csrf", token)
			req.AddCookie(&http.Cookie{Name: "threadify_csrf_dev", Value: proof})
			req.Header.Set(BrowserCSRFHeader, proof)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	body := `{"email":"http@example.test","full_name":"HTTP","role":"member"}`
	require.Equal(t, 403, send("/v1/users", body, false).Code)
	created := send("/v1/users", body, true)
	require.Equal(t, 201, created.Code, created.Body.String())
	require.Contains(t, created.Body.String(), `"status":"invited"`)
	require.NotContains(t, created.Body.String(), "invitation_token")
	require.Equal(t, 200, send("/v1/users", body, true).Code)
	require.Equal(t, 410, send("/api/team/invitations", body, true).Code)
}
