package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	serror "threadify-go/shared/errors"
	sharedmodels "threadify-go/shared/models"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestEntityProfileTypeHandler_CRUD(t *testing.T) {
	const (
		companyID = "comp_123"
		eptID     = "ept_456"
		userID    = "user_1"
	)

	newRouter := func(deps *common.MockedHandlers) *gin.Engine {
		h := handlers.NewEntityProfileTypeHandler(deps.EntityProfileSvc)
		r := common.SetupTestRouter()
		r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
		r.POST("/entity-profile-types", h.CreateEntityProfileType)
		r.PUT("/entity-profile-types/:id", h.UpdateEntityProfileType)
		r.DELETE("/entity-profile-types/:id", h.ArchiveEntityProfileType)
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
			name:   "create_success",
			method: "POST",
			path:   "/entity-profile-types",
			body: map[string]any{
				"name":        "Test Type",
				"description": "Test Desc",
				"type":        "contract",
			},
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					CreateEntityProfileType(gomock.Any(), companyID, gomock.Any()).
					Return(&sharedmodels.EntityProfileType{ID: eptID}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "create_validation_error_empty_name",
			method:     "POST",
			path:       "/entity-profile-types",
			body:       map[string]any{"name": ""},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "create_malformed_json",
			method:     "POST",
			path:       "/entity-profile-types",
			body:       "{bad}",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "update_success",
			method: "PUT",
			path:   "/entity-profile-types/" + eptID,
			body:   map[string]any{"name": "New Name", "description": "New Desc"},
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					UpdateEntityProfileType(gomock.Any(), companyID, eptID, gomock.Any()).
					Return(&sharedmodels.EntityProfileType{ID: eptID}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "update_not_found",
			method: "PUT",
			path:   "/entity-profile-types/missing",
			body:   map[string]any{"name": "X"},
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					UpdateEntityProfileType(gomock.Any(), companyID, "missing", gomock.Any()).
					Return(nil, serror.NewDomainError("not found", http.StatusNotFound))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "archive_success",
			method: "DELETE",
			path:   "/entity-profile-types/" + eptID,
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ArchiveEntityProfileType(gomock.Any(), companyID, eptID).
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
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:   "list_success",
			method: "GET",
			path:   "/entity-profile-types",
			setupMock: func(d *common.MockedHandlers) {
				d.EntityProfileSvc.EXPECT().
					ListEntityProfileTypes(gomock.Any(), companyID).
					Return([]*sharedmodels.EntityProfileType{{ID: "ept_1", Name: "Type 1"}}, nil)
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
