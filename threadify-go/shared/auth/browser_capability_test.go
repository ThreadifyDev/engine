package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserCapabilityAllows(t *testing.T) {
	grant := &BrowserCapability{Actions: []string{"startThread", "recordThreadEvent", "recordBrowserAction", "waitFor"}, ThreadIDs: []string{"existing"}, ThreadKeys: []string{"order:1"}}
	tests := []struct {
		action, thread, key string
		created             []string
		allowed             bool
	}{
		{"startThread", "", "order:1", nil, true},
		{"startThread", "", "order:2", nil, false},
		{"startThread", "", "", nil, false},
		{"recordThreadEvent", "existing", "", nil, true},
		{"recordBrowserAction", "existing", "", nil, true},
		{"recordBrowserAction", "other", "", nil, false},
		{"waitFor", "created", "", []string{"created"}, true},
		{"recordThreadEvent", "other", "", nil, false},
		{"addRefs", "existing", "", nil, false},
		{"waitFor", "", "", nil, false},
		{"closeConnection", "", "", nil, true},
	}
	for _, test := range tests {
		if got := grant.Allows(test.action, test.thread, test.key, test.created); got != test.allowed {
			t.Errorf("Allows(%q,%q,%q)=%v, want %v", test.action, test.thread, test.key, got, test.allowed)
		}
	}
	var absent *BrowserCapability
	if absent.Allows("startThread", "", "order:1", nil) {
		t.Fatal("missing grant allowed an action")
	}
}

func TestBrowserCapabilityLifecycle(t *testing.T) {
	s, _ := browserFixture(t)
	SetBrowserService(s)
	t.Cleanup(func() { SetBrowserService(nil) })
	t.Setenv("THREADIFY_BROWSER_ORIGINS", "http://localhost:5173")
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `INSERT INTO service_accounts(id,company_id,name) VALUES('processor','company','Processor');
 INSERT INTO user_roles VALUES('processor','service_account','processor','processor')`)
	require.NoError(t, err)
	_, err = s.pool.Exec(ctx, `INSERT INTO api_keys(id,company_id,service_account_id,key_hash) VALUES('key','company','processor',$1)`, browserHash("service-key"))
	require.NoError(t, err)
	body := `{"origin":"http://localhost:5173","actions":["startThread","recordThreadEvent"],"thread_keys":["order:1"],"ttl_seconds":60}`
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8083/v1/browser-tokens", strings.NewReader(body))
	r.Header.Set("X-API-Key", "service-key")
	w := httptest.NewRecorder()
	s.Wrap(http.NotFoundHandler()).ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	bad := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8083/v1/browser-tokens", strings.NewReader(`{"origin":"http://localhost:5173","actions":["inviteParty"],"ttl_seconds":60}`))
	bad.Header.Set("X-API-Key", "service-key")
	badResponse := httptest.NewRecorder()
	s.Wrap(http.NotFoundHandler()).ServeHTTP(badResponse, bad)
	require.Equal(t, http.StatusBadRequest, badResponse.Code)
	var issued struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &issued))
	grant, err := VerifyBrowserCapability(ctx, issued.Token, "http://localhost:5173")
	require.NoError(t, err)
	require.True(t, grant.Allows("startThread", "", "order:1", nil))
	require.False(t, grant.Allows("startThread", "", "order:2", nil))
	_, err = VerifyBrowserCapability(ctx, issued.Token, "http://localhost:5174")
	require.Error(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE api_keys SET revoked_at=NOW() WHERE id='key'`)
	require.NoError(t, err)
	_, err = VerifyBrowserCapability(ctx, issued.Token, "http://localhost:5173")
	require.Error(t, err)
}
