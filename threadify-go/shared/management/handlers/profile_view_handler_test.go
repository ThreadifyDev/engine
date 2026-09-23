package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/rbac"
)

const viewTypeID = "6e9f8212-d54d-4c86-9f67-c0aa9b213d11"

type viewStoreStub struct {
	company, actor string
	calls          int
	err            error
}

func (s *viewStoreStub) Get(_ context.Context, company, id string) (*domain.SavedProfileView, error) {
	s.company = company
	s.calls++
	return &domain.SavedProfileView{ProfileTypeID: id, Requests: []domain.ProfileViewRequest{}}, s.err
}
func (s *viewStoreStub) Save(_ context.Context, company, id, actor string, revision int64, v domain.ProfileViewDefinition) (*domain.SavedProfileView, error) {
	s.company = company
	s.actor = actor
	s.calls++
	return &domain.SavedProfileView{ProfileTypeID: id, Definition: &v, Revision: revision + 1}, s.err
}
func viewRouter(t *testing.T, store *viewStoreStub, role, company string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	fs := fstest.MapFS{"permissions": {Data: []byte(`{}`)}, "roles": {Data: []byte(`{"app_level":{"editor":{"permissions":["entity_profile_type.read","entity_profile_type.update"]},"reader":{"permissions":["entity_profile_type.read"]}},"api_level":{"service":{"permissions":["entity_profile_type.*"]}}}`)}}
	roles, err := rbac.NewLoaderFromFS(fs, "permissions", "roles")
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(sharedauth.CtxCompanyID, company)
		c.Set(sharedauth.CtxUserID, "actor")
		c.Set(sharedauth.CtxRoles, []string{role})
	})
	h := NewProfileViewHandler(store, roles)
	router.GET("/views/:id", h.Get)
	router.PUT("/views/:id", h.Save)
	return router
}
func viewRequest(t *testing.T, router *gin.Engine, method, payload string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, "/views/"+viewTypeID, bytes.NewBufferString(payload))
	r.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, r)
	return w
}

const validViewBody = `{"expected_revision":0,"definition":{"schemaVersion":1,"title":"Health","description":"","range":"30d","columns":2,"blocks":[{"id":"health","source":"deliveryHealth","title":"Health"}]}}`

func TestProfileViewPermissionsAndScope(t *testing.T) {
	for _, tc := range []struct {
		role, company, method string
		status                int
	}{{"reader", "company", "GET", 200}, {"reader", "company", "PUT", 403}, {"editor", "company", "PUT", 200}, {"service", "company", "PUT", 200}, {"unknown", "company", "GET", 403}, {"editor", "", "GET", 401}} {
		t.Run(tc.role+tc.company+tc.method, func(t *testing.T) {
			store := &viewStoreStub{}
			w := viewRequest(t, viewRouter(t, store, tc.role, tc.company), tc.method, validViewBody)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			if tc.status == 200 {
				require.Equal(t, "company", store.company)
				if tc.method == "PUT" {
					require.Equal(t, "actor", store.actor)
				} else {
					var out map[string]any
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
					require.Equal(t, false, out["can_manage"])
				}
			} else {
				require.Zero(t, store.calls)
			}
		})
	}
}
func TestProfileViewRejectsInvalidRequests(t *testing.T) {
	for _, payload := range []string{
		`{}`, `null`, strings.Replace(validViewBody, `"expected_revision":0,`, "", 1), strings.Replace(validViewBody, `"expected_revision":0`, `"expected_revision":-1`, 1),
		strings.Replace(validViewBody, `"deliveryHealth"`, `"history"`, 1), strings.Replace(validViewBody, `"title":"Health"`, `"query":"select *","title":"Health"`, 1),
		strings.Replace(validViewBody, `"definition":`, `"company_id":"other","definition":`, 1), strings.Replace(validViewBody, `"definition":`, `"requests":[],"definition":`, 1),
		validViewBody + ` {}`, strings.Repeat(" ", 33<<10) + validViewBody,
	} {
		store := &viewStoreStub{}
		w := viewRequest(t, viewRouter(t, store, "editor", "company"), "PUT", payload)
		require.Equal(t, 400, w.Code)
		require.Zero(t, store.calls)
	}
}
func TestProfileViewErrors(t *testing.T) {
	for _, tc := range []struct {
		err     error
		code    int
		message string
	}{{domain.ErrProfileViewConflict, 409, "PROFILE_VIEW_CONFLICT"}, {serror.ErrEntityProfileTypeNotFound, 404, "profile type not found"}, {errors.New("password=private database internal"), 500, "Could not access"}} {
		w := viewRequest(t, viewRouter(t, &viewStoreStub{err: tc.err}, "editor", "company"), "PUT", validViewBody)
		require.Equal(t, tc.code, w.Code)
		require.Contains(t, w.Body.String(), tc.message)
		require.NotContains(t, w.Body.String(), "private")
	}
}
