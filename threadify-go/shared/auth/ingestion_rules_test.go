package auth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"threadify-go/shared/ingestion"
)

type testIngestionStore struct {
	settings ingestion.Settings
	err      error
	saves    int
}

func (s *testIngestionStore) Load(context.Context, string) (ingestion.Settings, error) {
	return s.settings, s.err
}
func (s *testIngestionStore) Record(context.Context, string, int, int) error { return nil }
func (s *testIngestionStore) Save(_ context.Context, _ string, revision string, filters []string) (ingestion.Settings, error) {
	if s.err != nil {
		return ingestion.Settings{}, s.err
	}
	if revision != s.settings.Revision {
		return ingestion.Settings{}, ingestion.ErrConflict
	}
	s.saves++
	s.settings = ingestion.Settings{Filters: filters, Revision: "next"}
	return s.settings, nil
}
func TestIngestionRulesHTTPAuthorizationAndPreview(t *testing.T) {
	s, _ := browserFixture(t)
	owner, token := userTestOwner(t, s)
	store := &testIngestionStore{settings: ingestion.Settings{Filters: []string{}}}
	handler := s.Wrap(s.IngestionRulesHandler(store, http.NotFoundHandler()))
	call := func(method, path, body string, session, csrf bool, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1:8083"+path, strings.NewReader(body))
		req.Header.Set("Origin", s.origin)
		req.Header.Set("Content-Type", "application/json")
		if session {
			req.AddCookie(&http.Cookie{Name: s.cookieName(req, "session"), Value: token})
		}
		if csrf {
			v := s.signed("csrf", token)
			req.AddCookie(&http.Cookie{Name: s.cookieName(req, "csrf"), Value: v})
			req.Header.Set(BrowserCSRFHeader, v)
		}
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	path := "/v1/engine/ingestion-rules"
	require.Equal(t, 401, call("GET", path, "", false, false, "").Code)
	require.Equal(t, 403, call("PUT", path, `{"filters":[],"revision":""}`, true, false, "").Code)
	require.Equal(t, 200, call("GET", path, "", true, false, "").Code)
	for _, body := range []string{`{}`, `{"filters":null,"revision":""}`, `{"filters":["a*b"],"revision":""}`, `{"filters":[],"revision":"","unexpected":true}`} {
		require.Equal(t, 400, call("PUT", path, body, true, true, "").Code)
	}
	preview := call("POST", path+"/preview", `{"filters":["internal.*"],"span_names":["internal.cache","refund"]}`, true, true, "")
	require.Equal(t, 200, preview.Code)
	require.Contains(t, preview.Body.String(), `"dropped":1`)
	require.Zero(t, store.saves)
	saved := call("PUT", path, `{"filters":["internal.*"],"revision":""}`, true, true, "")
	require.Equal(t, 200, saved.Code, saved.Body.String())
	require.Equal(t, 1, store.saves)
	require.Equal(t, 409, call("PUT", path, `{"filters":[],"revision":""}`, true, true, "").Code)
	// CLI uses the same administration endpoint with an API key, without browser cookies.
	_, err := s.pool.Exec(context.Background(), `INSERT INTO api_keys(id,company_id,user_id,key_hash) VALUES('filter-admin',$1,$2,$3)`, owner.CompanyID, owner.UserID, browserHash("filter-admin-key"))
	require.NoError(t, err)
	require.Equal(t, 200, call("GET", path, "", false, false, "filter-admin-key").Code)
	cleared := call("PUT", path, `{"filters":[],"revision":"next"}`, false, false, "filter-admin-key")
	require.Equal(t, 200, cleared.Code, cleared.Body.String())
	require.Empty(t, store.settings.Filters)
	// A role change must take effect even though the credential is still present.
	_, err = s.pool.Exec(context.Background(), `UPDATE user_roles SET role_name='viewer' WHERE principal_id=$1`, owner.UserID)
	require.NoError(t, err)
	read := call("GET", path, "", false, false, "filter-admin-key")
	require.Equal(t, 200, read.Code)
	var response struct {
		CanManage bool `json:"can_manage"`
	}
	require.NoError(t, json.Unmarshal(read.Body.Bytes(), &response))
	require.False(t, response.CanManage)
	require.Equal(t, 403, call("PUT", path, `{"filters":[],"revision":"next"}`, false, false, "filter-admin-key").Code)
	store.err = errors.New("offline")
	require.Equal(t, 503, call("GET", path, "", false, false, "filter-admin-key").Code)
	require.Equal(t, 405, call("DELETE", path, "", false, false, "filter-admin-key").Code)
	require.Equal(t, 404, call("GET", "/different", "", false, false, "filter-admin-key").Code)
}
