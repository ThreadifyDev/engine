// service_account_handler_test.go
package tests

import (
	"embed"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	"threadify-go/api/internal/models"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/rbac"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/permissions.json testdata/roles.json
var rbacFS embed.FS

func TestServiceAccountHandler_CRUD(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_1"
		saID      = "sa_789"
	)

	// All routes registered once — the switch-per-case anti-pattern is gone.
	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		loader, err := rbac.NewLoaderFromFS(rbacFS, "testdata/permissions.json", "testdata/roles.json")
		if err != nil {
			panic(err) // loader construction failing is a setup bug, not a test case
		}
		h := handlers.NewServiceAccountHandler(deps.ServiceAccountSvc, loader)
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
		r.POST("/service-accounts", h.CreateServiceAccount)
		r.GET("/service-accounts/:id", h.GetServiceAccount)
		r.PUT("/service-accounts/:id", h.UpdateServiceAccount)
		r.DELETE("/service-accounts/:id", h.DeleteServiceAccount)
		return r
	}

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name:   "create_success",
			method: "POST",
			path:   "/service-accounts",
			body:   map[string]string{"name": "my-sa", "role": "admin"},
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					CreateServiceAccount(gomock.Any(), companyID, userID, gomock.Any()).
					Return(&models.ServiceAccount{ID: saID, Name: "my-sa"}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "create_malformed_json",
			method:     "POST",
			path:       "/service-accounts",
			body:       "{bad}",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "get_success",
			method: "GET",
			path:   "/service-accounts/" + saID,
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					GetServiceAccount(gomock.Any(), saID, companyID).
					Return(&models.ServiceAccount{ID: saID, Name: "my-sa"}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "update_success",
			method: "PUT",
			path:   "/service-accounts/" + saID,
			body:   map[string]string{"name": "new-name"},
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					UpdateServiceAccount(gomock.Any(), saID, companyID, gomock.Any()).
					Return(&models.ServiceAccount{ID: saID, Name: "new-name"}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "update_not_found",
			method: "PUT",
			path:   "/service-accounts/missing",
			body:   map[string]string{"name": "x"},
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					UpdateServiceAccount(gomock.Any(), "missing", companyID, gomock.Any()).
					Return(nil, serror.NewDomainError("not found", http.StatusNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "delete_success",
			method: "DELETE",
			path:   "/service-accounts/" + saID,
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					DeleteServiceAccount(gomock.Any(), saID, companyID).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "delete_service_error",
			method: "DELETE",
			path:   "/service-accounts/" + saID,
			setupMock: func(d *common.MockedHandlers) {
				d.ServiceAccountSvc.EXPECT().
					DeleteServiceAccount(gomock.Any(), saID, companyID).
					Return(errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newRouter(deps), tt.method, tt.path, tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestServiceAccountHandler_GetPermissions(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_1"
	)

	tests := []struct {
		name       string
		scope      string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
		assertBody func(t *testing.T, body []byte)
	}{
		{
			name:       "success_api_level",
			scope:      "api_level",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusOK,
			assertBody: func(t *testing.T, body []byte) {
				var resp struct {
					Roles map[string]rbac.Role `json:"roles"`
				}
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.NotEmpty(t, resp.Roles)
			},
		},
		{
			name:       "invalid_scope",
			scope:      "invalid",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)

			loader, err := rbac.NewLoaderFromFS(rbacFS, "testdata/permissions.json", "testdata/roles.json")
			require.NoError(t, err)

			h := handlers.NewServiceAccountHandler(deps.ServiceAccountSvc, loader)
			r := common.SetupTestRouter()
			r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
			r.GET("/permissions/:scope", h.GetPermissions)

			w := common.DoRequest(t, r, "GET", "/permissions/"+tt.scope, nil)
			assert.Equal(t, tt.wantStatus, w.Code)

			if tt.assertBody != nil {
				tt.assertBody(t, w.Body.Bytes())
			}
		})
	}
}
