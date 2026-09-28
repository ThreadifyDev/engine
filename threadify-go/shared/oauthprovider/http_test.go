package oauthprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Usefused/fused-open-core/oauthserver"
)

func TestPostgresOAuthHTTP(t *testing.T) {
	store, _, policy, service := postgresFixture(t)
	handler := &HTTPServer{Service: service, Store: store, Policy: policy, PublicURL: "http://example.test",
		Browser: func(r *http.Request) (BrowserActor, error) {
			switch r.Header.Get("X-Test-Session") {
			case "admin":
				return BrowserActor{UserID: "user-1", CredentialHash: "admin-session", Admin: true}, nil
			case "user":
				return BrowserActor{UserID: "user-1", CredentialHash: "user-session"}, nil
			default:
				return BrowserActor{}, oauthserver.ErrAuthenticationRequired
			}
		},
		SameOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == "http://example.test" },
		ValidCSRF:  func(r *http.Request) bool { return r.Header.Get("X-Test-CSRF") == "valid" },
	}
	server := httptest.NewServer(handler.Wrap(http.NotFoundHandler()))
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, path, body, session, origin string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if session != "" {
			req.Header.Set("X-Test-Session", session)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	registration, _ := json.Marshal(map[string]any{"name": "Contract app", "client_type": "confidential", "redirect_uris": []string{"https://client.example/callback"}, "allowed_scopes": PilotScopes()})
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/oauth/clients", bytes.NewReader(registration))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test-Session", "admin")
	req.Header.Set("X-Test-CSRF", "valid")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("register status %d: %s", resp.StatusCode, data)
	}
	var created struct {
		Client oauthserver.OAuthClient `json:"client"`
		Secret string                  `json:"client_secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	query := url.Values{"client_id": {created.Client.ClientID}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "scope": {ScopeContractReadAll}, "state": {"state-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	resp = request(http.MethodGet, "/oauth/authorize?"+query.Encode(), "", "user", "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(body, []byte("Contract app")) || !bytes.Contains(body, []byte("contract.read.*")) {
		t.Fatalf("consent page status=%d", resp.StatusCode)
	}
	form := url.Values{"client_id": {created.Client.ClientID}, "redirect_uri": {"https://client.example/callback"}, "scope": {ScopeContractReadAll}, "state": {"state-1"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "decision": {"allow"}}
	resp = request(http.MethodPost, "/oauth/authorize/consent", form.Encode(), "user", "http://evil.example")
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cross-origin consent=%d", resp.StatusCode)
	}
	deniedForm := url.Values{}
	for key, values := range form {
		deniedForm[key] = append([]string(nil), values...)
	}
	deniedForm.Set("decision", "deny")
	resp = request(http.MethodPost, "/oauth/authorize/consent", deniedForm.Encode(), "user", "http://example.test")
	deniedLocation := resp.Header.Get("Location")
	resp.Body.Close()
	deniedURL, err := url.Parse(deniedLocation)
	if err != nil || resp.StatusCode != 302 || deniedURL.Query().Get("error") != "access_denied" || deniedURL.Query().Get("code") != "" {
		t.Fatalf("denied consent response: %d %q", resp.StatusCode, deniedLocation)
	}
	resp = request(http.MethodPost, "/oauth/authorize/consent", form.Encode(), "user", "http://example.test")
	location := resp.Header.Get("Location")
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("consent=%d", resp.StatusCode)
	}
	redirect, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	code := redirect.Query().Get("code")
	if code == "" || redirect.Query().Get("state") != "state-1" {
		t.Fatal("missing code or state")
	}
	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://client.example/callback"}, "code_verifier": {verifier}}
	postToken := func(form url.Values) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/oauth/token", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(created.Client.ClientID, created.Secret)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	resp = postToken(tokenForm)
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("token=%d: %s", resp.StatusCode, data)
	}
	var issued struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Scope   string `json:"scope"`
		Type    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&issued); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if issued.Type != "Bearer" || issued.Scope != ScopeContractReadAll || issued.Access == "" || issued.Refresh == "" {
		t.Fatal("bad token response")
	}
	if _, _, err := store.AccessGrant(t.Context(), issued.Access); err != nil {
		t.Fatal(err)
	}
	resp = postToken(tokenForm)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("code replay=%d", resp.StatusCode)
	}
	revokeForm := url.Values{"token": {issued.Refresh}}
	req, err = http.NewRequest(http.MethodPost, server.URL+"/oauth/revoke", strings.NewReader(revokeForm.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(created.Client.ClientID, created.Secret)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("revoke=%d", resp.StatusCode)
	}
	if _, _, err := store.AccessGrant(t.Context(), issued.Access); err == nil {
		t.Fatal("revoked HTTP token still active")
	}
	req, err = http.NewRequest(http.MethodDelete, server.URL+"/v1/oauth/clients/"+created.Client.ID.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test-Session", "admin")
	req.Header.Set("X-Test-CSRF", "valid")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("admin revoke=%d", resp.StatusCode)
	}
	resp = request(http.MethodGet, "/oauth/authorize?"+query.Encode(), "", "user", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("revoked client authorize=%d", resp.StatusCode)
	}
	resp = request(http.MethodGet, "/.well-known/oauth-authorization-server", "", "", "")
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("metadata=%d", resp.StatusCode)
	}
}
