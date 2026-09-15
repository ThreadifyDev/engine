package handlers

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	sharedauth "threadify-go/shared/auth"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/registry"
	"threadify-go/shared/repository"
)

type profileTypeRepo struct {
	repository.EntityProfileTypeRepository
	created *domain.EntityProfileType
	err     error
}

func (r *profileTypeRepo) CreateProfileType(_ context.Context, p *domain.EntityProfileType) error {
	r.created = p
	return r.err
}

func TestEngineCreateProfileType(t *testing.T) {
	for _, test := range []struct {
		name, body, company string
		status              int
		err                 error
	}{
		{"create", `{"name":" Customers ","type":["customer_id"]}`, "company-a", 201, nil},
		{"missing company", `{}`, "", 401, nil},
		{"blank name", `{"name":" ","type":["customer_id"]}`, "company-a", 400, nil},
		{"blank ref", `{"name":"Customers","type":[" "]}`, "company-a", 400, nil},
		{"duplicate ref", `{"name":"Customers","type":["customer_id","customer_id"]}`, "company-a", 400, nil},
		{"company injection", `{"name":"Customers","type":["customer_id"],"company_id":"company-b"}`, "company-a", 400, nil},
		{"unsupported metrics", `{"name":"Customers","type":["customer_id"],"metrics":[]}`, "company-a", 400, nil},
		{"multiple documents", `{} {}`, "company-a", 400, nil},
		{"duplicate", `{"name":"Customers","type":["customer_id"]}`, "company-a", 409, serror.ErrEntityProfileTypeAlreadyExists},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &profileTypeRepo{err: test.err}
			router := gin.New()
			router.POST("/", func(c *gin.Context) { c.Set(sharedauth.CtxCompanyID, test.company) }, CreateProfileType(repo))
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/", strings.NewReader(test.body))
			router.ServeHTTP(w, req)
			require.Equal(t, test.status, w.Code, w.Body.String())
			if test.status == 201 {
				require.Equal(t, "company-a", repo.created.CompanyID)
				require.Equal(t, "customers", repo.created.Slug)
				require.Contains(t, w.Body.String(), `"id":`)
			} else if test.status < 409 {
				require.Nil(t, repo.created)
			}
		})
	}
}

type profileLookup struct {
	repository.EntityProfileTypeRepository
	value *domain.EntityProfileType
}

func (r *profileLookup) GetProfileTypeByID(context.Context, string) (*domain.EntityProfileType, error) {
	return r.value, nil
}

type profileWrites struct {
	repository.EntityProfileRepository
	called bool
	err    error
}

func (r *profileWrites) CreateProfile(context.Context, *domain.EntityProfile) error {
	r.called = true
	return r.err
}
func TestProfileCreationScopeAndQuota(t *testing.T) {
	for _, test := range []struct {
		name, company string
		code          int
		err           error
	}{
		{"another tenant", "other-company", 404, nil},
		{"quota", "company-a", 429, registry.ErrLimit},
		{"license unavailable", "company-a", 503, registry.ErrUnverified},
	} {
		t.Run(test.name, func(t *testing.T) {
			types := &profileLookup{value: &domain.EntityProfileType{ID: "e3bb38a5-c3d0-42fc-b59c-d3882d06c627", CompanyID: test.company}}
			profiles := &profileWrites{err: test.err}
			r := gin.New()
			r.PUT("/", func(c *gin.Context) { c.Set(sharedauth.CtxCompanyID, "company-a") }, PutProfile(types, profiles))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("PUT", "/", strings.NewReader(`{"type_id":"e3bb38a5-c3d0-42fc-b59c-d3882d06c627","ref_value":"C-1","name":"Customer"}`)))
			require.Equal(t, test.code, w.Code, w.Body.String())
			require.Equal(t, test.code != 404, profiles.called)
		})
	}
}
