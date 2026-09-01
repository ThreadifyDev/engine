package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	shareddomain "threadify-go/shared/domain"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestEntityProfileTypeHandler_CRUD(t *testing.T) {
	const (
		companyID = "comp_123"
		eptID     = "ept_456"
		eptSlug   = "test_type"
		userID    = "user_1"
	)

	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		h := handlers.NewEntityProfileTypeHandler(deps.EntityProfileSvc)
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
		r.PUT("/entity-profile-types/:slug", h.ApplyEntityProfileType)
		r.DELETE("/entity-profile-types/:slug", h.ArchiveEntityProfileType)
		r.GET("/entity-profile-types", h.ListEntityProfileTypes)
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
			name:   "apply_create_success",
			method: "PUT",
			path:   "/entity-profile-types/" + eptSlug,
			body: map[string]any{
				"name":        "Test Type",
				"description": "Test Desc",
				"type":        []string{"contract", "agreement"},
			},
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ApplyEntityProfileType(gomock.Any(), companyID, eptSlug, gomock.Any(), false).
					Return(&domain.ApplyEntityProfileTypeResult{Status: "created", Profile: &shareddomain.EntityProfileType{ID: eptID}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "apply_validation_error_empty_name",
			method:     "PUT",
			path:       "/entity-profile-types/test",
			body:       map[string]any{"name": ""},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "apply_malformed_json",
			method:     "PUT",
			path:       "/entity-profile-types/test",
			body:       "{bad}",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "apply_dry_run_success",
			method: "PUT",
			path:   "/entity-profile-types/test_type?dry_run=true",
			body:   map[string]any{"name": "Test Type", "description": "New Desc", "type": []string{"id"}},
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ApplyEntityProfileType(gomock.Any(), companyID, "test_type", gomock.Any(), true).
					Return(&domain.ApplyEntityProfileTypeResult{Status: "updated", DryRun: true, Profile: &shareddomain.EntityProfileType{ID: eptID}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "apply_rejects_implicit_rename",
			method:     "PUT",
			path:       "/entity-profile-types/old_name",
			body:       map[string]any{"name": "New Name", "type": []string{"id"}},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "archive_success",
			method: "DELETE",
			path:   "/entity-profile-types/" + eptSlug,
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ArchiveEntityProfileType(gomock.Any(), companyID, eptSlug).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "archive_not_found",
			method: "DELETE",
			path:   "/entity-profile-types/missing",
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ArchiveEntityProfileType(gomock.Any(), companyID, "missing").
					Return(serror.NewDomainError("not found", http.StatusNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "list_success",
			method: "GET",
			path:   "/entity-profile-types",
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ListEntityProfileTypes(gomock.Any(), companyID).
					Return([]*shareddomain.EntityProfileType{{ID: "ept_1", Name: "Type 1"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "list_service_error",
			method: "GET",
			path:   "/entity-profile-types",
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ListEntityProfileTypes(gomock.Any(), companyID).
					Return(nil, errors.New("db error"))
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
