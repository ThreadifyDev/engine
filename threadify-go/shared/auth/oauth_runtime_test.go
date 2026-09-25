package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"threadify-go/shared/rbac"
)

func TestBrowserMountedOAuthFlow(t *testing.T) {
	s, _ := browserFixture(t)
	SetBrowserService(s)
	t.Cleanup(func() { SetBrowserService(nil); currentOAuth.Store(nil) })
	loader, err := rbac.NewEmbeddedLoader()
	require.NoError(t, err)
	handler, err := s.OAuthServer(t.Context(), loader, s.origin, http.NotFoundHandler())
	require.NoError(t, err)
	login := browserRequest(s, "POST", "/auth/api-key/exchange", `{"api_key":"test-license"}`, nil, false)
	require.Equal(t, 200, login.Code)
	cookies := login.Result().Cookies()
	var session, csrf *http.Cookie
	for _, cookie := range cookies {
		if strings.Contains(cookie.Name, "session") {
			session = cookie
		}
		if strings.Contains(cookie.Name, "csrf") {
			csrf = cookie
		}
	}
	require.NotNil(t, session)
	require.NotNil(t, csrf)
	call := func(method, path, body, contentType string, withCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
		r.AddCookie(session)
		r.AddCookie(csrf)
		r.Header.Set("Origin", s.origin)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		if withCSRF {
			r.Header.Set(BrowserCSRFHeader, csrf.Value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	registration := `{"name":"Contract app","client_type":"confidential","redirect_uris":["https://client.example/callback"],"allowed_scopes":["contract.read.*"]}`
	w := call("POST", "/v1/oauth/clients", registration, "application/json", false)
	require.Equal(t, 403, w.Code)
	w = call("POST", "/v1/oauth/clients", registration, "application/json", true)
	require.Equal(t, 201, w.Code, w.Body.String())
	var created struct {
		Client struct {
			ID string `json:"ClientID"`
		} `json:"client"`
		Secret string `json:"client_secret"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Client.ID)
	require.NotEmpty(t, created.Secret)
	verifier := strings.Repeat("v", 43)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	query := url.Values{"client_id": {created.Client.ID}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "scope": {"contract.read.*"}, "state": {"state-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	w = call("GET", "/oauth/authorize?"+query.Encode(), "", "", false)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "Contract app")
	form := url.Values{"client_id": {created.Client.ID}, "redirect_uri": {"https://client.example/callback"}, "scope": {"contract.read.*"}, "state": {"state-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "decision": {"allow"}}
	w = call("POST", "/oauth/authorize/consent", form.Encode(), "application/x-www-form-urlencoded", false)
	require.Equal(t, 302, w.Code, w.Body.String())
	redirect, err := url.Parse(w.Result().Header.Get("Location"))
	require.NoError(t, err)
	code := redirect.Query().Get("code")
	require.NotEmpty(t, code)
	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://client.example/callback"}, "code_verifier": {verifier}}
	r := httptest.NewRequest("POST", s.origin+"/oauth/token", strings.NewReader(tokenForm.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetBasicAuth(created.Client.ID, created.Secret)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	require.Equal(t, 200, w.Code, w.Body.String())
	var tokens struct {
		Access string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tokens))
	require.NotEmpty(t, tokens.Access)
	claims, err := VerifyOAuthAccess(t.Context(), tokens.Access)
	require.NoError(t, err)
	require.True(t, claims.OAuthAccess)
	require.True(t, OAuthCanUse(t.Context(), claims, "contract.read.contract-1"))
	require.False(t, OAuthCanUse(t.Context(), claims, "contract.delete.contract-1"))
	_, err = s.pool.Exec(t.Context(), `DELETE FROM user_roles WHERE principal_id=$1`, claims.UserID)
	require.NoError(t, err)
	_, err = VerifyOAuthAccess(t.Context(), tokens.Access)
	require.Error(t, err)
}
