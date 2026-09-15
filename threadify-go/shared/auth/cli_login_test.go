package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCLILoginApprovalAndRevocation(t *testing.T) {
	s, _ := browserFixture(t)
	call := func(path string, body any, key string) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8083"+path, strings.NewReader(string(data)))
		r.Header.Set("X-API-Key", key)
		w := httptest.NewRecorder()
		s.handleCLIAuth(w, r)
		return w
	}
	raw := CLICredentialPrefix + randomBrowserToken()
	start := call("/auth/cli/start", map[string]string{"credential_hash": browserHash(raw)}, "")
	require.Equal(t, 201, start.Code, start.Body.String())
	require.NotContains(t, start.Body.String(), raw)
	var enrollment struct {
		ID   string `json:"transaction_id"`
		Poll string `json:"poll_token"`
		URL  string `json:"verification_url"`
	}
	require.NoError(t, json.Unmarshal(start.Body.Bytes(), &enrollment))
	u, err := url.Parse(enrollment.URL)
	require.NoError(t, err)
	fragment, err := url.ParseQuery(u.Fragment)
	require.NoError(t, err)
	poll := map[string]string{"transaction_id": enrollment.ID, "poll_token": enrollment.Poll}
	require.Equal(t, 202, call("/auth/cli/poll", poll, "").Code)
	require.Equal(t, 403, call("/auth/cli/poll", map[string]string{"transaction_id": enrollment.ID, "poll_token": fragment.Get("browser_token")}, "").Code)
	_, err = s.authenticateCLI(context.Background(), raw)
	require.Error(t, err, "unapproved credentials cannot authenticate")
	exchanged := browserRequest(s, "POST", "/auth/api-key/exchange", `{"api_key":"test-license"}`, nil, false)
	require.Equal(t, 200, exchanged.Code)
	cookies := exchanged.Result().Cookies()
	session := browserRequest(s, "GET", "/auth/session", "", cookies, false)
	require.Equal(t, 200, session.Code)
	cookies = append(cookies, session.Result().Cookies()...)
	// Keep one cookie per name, as a browser cookie jar would.
	byName := map[string]*http.Cookie{}
	for _, cookie := range cookies {
		byName[cookie.Name] = cookie
	}
	cookies = nil
	for _, cookie := range byName {
		cookies = append(cookies, cookie)
	}
	approval, _ := json.Marshal(map[string]string{"transaction_id": enrollment.ID, "browser_token": fragment.Get("browser_token")})
	require.Equal(t, 403, browserRequest(s, "POST", "/auth/cli/approve", string(approval), cookies, false).Code)
	approved := browserRequest(s, "POST", "/auth/cli/approve", string(approval), cookies, true)
	require.Equal(t, 200, approved.Code, approved.Body.String())
	require.Equal(t, 403, browserRequest(s, "POST", "/auth/cli/approve", string(approval), cookies, true).Code)
	require.Equal(t, 200, call("/auth/cli/poll", poll, "").Code)
	claims, err := s.authenticateCLI(context.Background(), raw)
	require.NoError(t, err)
	require.Equal(t, "company", claims.CompanyID)
	require.Contains(t, claims.Roles, "admin")
	require.Equal(t, 200, call("/auth/cli/revoke", map[string]any{}, raw).Code)
	_, err = s.authenticateCLI(context.Background(), raw)
	require.Error(t, err)
	require.Equal(t, 200, call("/auth/cli/revoke", map[string]any{}, raw).Code, "logout retries are idempotent")
}

func TestCLICredentialFreshIdentityAndSource(t *testing.T) {
	s, _ := browserFixture(t)
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `INSERT INTO users(id,company_id,email,status) VALUES('person','company','person@example.test','active'); INSERT INTO user_roles(principal_id,principal_type,role_name,assigned_by) VALUES('person','user','member','person'); INSERT INTO api_keys(id,company_id,user_id,key_hash) VALUES('source','company','person','source-hash')`)
	require.NoError(t, err)
	raw := CLICredentialPrefix + randomBrowserToken()
	_, err = s.pool.Exec(ctx, `INSERT INTO threadify_cli_credentials(id,token_hash,company_id,installation_id,principal_id,principal_type,source_key_id,auth_generation,expires_at) VALUES('cli',$1,'company','installation','person','user','source',$2,$3)`, browserHash(raw), s.signed("generation", "v1"), s.now().Add(time.Hour))
	require.NoError(t, err)
	claims, err := s.authenticateCLI(ctx, raw)
	require.NoError(t, err)
	require.Equal(t, []string{"member"}, claims.Roles)
	_, err = s.pool.Exec(ctx, `UPDATE users SET status='suspended' WHERE id='person'`)
	require.NoError(t, err)
	_, err = s.authenticateCLI(ctx, raw)
	require.Error(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE users SET status='active' WHERE id='person'; UPDATE api_keys SET revoked_at=NOW() WHERE id='source'`)
	require.NoError(t, err)
	_, err = s.authenticateCLI(ctx, raw)
	require.Error(t, err)
}
