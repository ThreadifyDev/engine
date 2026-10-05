package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"threadify-go/shared/actionmapping"
)

type testActionMappingStore struct {
	settings actionmapping.Settings
	saves    int
}

func (s *testActionMappingStore) Load(context.Context, string) (actionmapping.Settings, error) {
	return s.settings, nil
}
func (s *testActionMappingStore) Save(_ context.Context, _ string, revision string, rules []actionmapping.Rule) (actionmapping.Settings, error) {
	if revision != s.settings.Revision {
		return actionmapping.Settings{}, actionmapping.ErrConflict
	}
	s.saves++
	s.settings = actionmapping.Settings{Rules: rules, Revision: "next"}
	return s.settings, nil
}

func TestActionMappingsHTTPAuthorizationAndValidation(t *testing.T) {
	s, _ := browserFixture(t)
	_, token := userTestOwner(t, s)
	store := &testActionMappingStore{settings: actionmapping.Settings{Rules: []actionmapping.Rule{}}}
	validated := 0
	validate := func(_ context.Context, _ string, rule actionmapping.Rule) error {
		validated++
		if rule.Step != "checkout" {
			return errors.New("unknown step")
		}
		return nil
	}
	handler := s.Wrap(s.ActionMappingsHandler(store, validate, http.NotFoundHandler()))
	call := func(method, body string, session, csrf bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8083/v1/engine/browser-action-mappings", strings.NewReader(body))
		r.Header.Set("Origin", s.origin)
		r.Header.Set("Content-Type", "application/json")
		if session {
			r.AddCookie(&http.Cookie{Name: s.cookieName(r, "session"), Value: token})
		}
		if csrf {
			v := s.signed("csrf", token)
			r.AddCookie(&http.Cookie{Name: s.cookieName(r, "csrf"), Value: v})
			r.Header.Set(BrowserCSRFHeader, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, call("GET", "", false, false).Code)
	require.Equal(t, 200, call("GET", "", true, false).Code)
	require.Equal(t, 403, call("PUT", `{"rules":[],"revision":""}`, true, false).Code)
	require.Equal(t, 400, call("PUT", `{"rules":[{"action":"a","contract":"order","step":"checkout"}],"revision":""}`, true, true).Code)
	require.Equal(t, 400, call("PUT", `{"rules":[{"action":"a","contract":"order","version":1,"step":"unknown"}],"revision":""}`, true, true).Code)
	for _, pattern := range []string{"regex:", "regex:[", "regex:(?=checkout)"} {
		body := `{"rules":[{"action":"` + pattern + `","contract":"order","version":1,"step":"checkout"}],"revision":""}`
		require.Equal(t, 400, call("PUT", body, true, true).Code)
	}
	require.Zero(t, store.saves)
	saved := call("PUT", `{"rules":[{"action":"regex:(?i)^checkout.{1,3}=confirmed$","contract":"order","version":1,"step":"checkout"}],"revision":""}`, true, true)
	require.Equal(t, 200, saved.Code, saved.Body.String())
	require.Equal(t, 2, validated)
	require.Equal(t, 1, store.saves)
	require.Equal(t, "regex:(?i)^checkout.{1,3}=confirmed$", store.settings.Rules[0].Action)
	require.Equal(t, 409, call("PUT", `{"rules":[],"revision":""}`, true, true).Code)
	require.Equal(t, 405, call("DELETE", "", true, true).Code)
}
