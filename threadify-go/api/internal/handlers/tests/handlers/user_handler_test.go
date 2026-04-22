package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	"threadify-go/api/internal/models"
	serror "threadify-go/shared/errors"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func newUserRouter(deps *common.MockedHandlers, companyID, userID string) *gin.Engine {
	h := handlers.NewUserHandler(deps.UserRepo, deps.CompanyRepo, deps.APIKeySvc, deps.UserRoleRepo)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))

	r.GET("/profile", h.GetProfile)
	r.POST("/profile", h.UpdateProfile)
	r.GET("/members", h.ListTeamMembers)

	return r
}

func TestUserHandler_GetProfile(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_123"
	)

	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().
					FindByID(gomock.Any(), userID).
					Return(&models.User{ID: userID, Email: "user@example.com"}, nil)
				d.CompanyRepo.EXPECT().
					FindByID(gomock.Any(), companyID).
					Return(&models.Company{ID: companyID, Name: "Acme Corp"}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "user_not_found",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().
					FindByID(gomock.Any(), userID).
					Return(nil, serror.ErrUserNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "service_error_company",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
				d.CompanyRepo.EXPECT().FindByID(gomock.Any(), companyID).Return(nil, errors.New("db fail"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "minimal_profile_success",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
				d.CompanyRepo.EXPECT().FindByID(gomock.Any(), companyID).Return(&models.Company{ID: companyID}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "company_forbidden",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
				d.CompanyRepo.EXPECT().FindByID(gomock.Any(), companyID).Return(nil, serror.NewDomainError("Forbidden", http.StatusForbidden))
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newUserRouter(deps, companyID, userID), "GET", "/profile", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestUserHandler_UpdateProfile(t *testing.T) {
	const (
		companyID = "comp_123"
		userID    = "user_123"
	)

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success_user_only",
			body: map[string]string{
				"full_name": "New Name",
				"job_role":  "Manager",
			},
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
				d.CompanyRepo.EXPECT().FindByID(gomock.Any(), companyID).Return(&models.Company{
					Industry: new(string),
				}, nil)
				d.UserRepo.EXPECT().UpdateProfile(gomock.Any(), userID, gomock.Any(), gomock.Any(), true).Return(nil)
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "validation_error_missing_fields",
			body: map[string]string{
				"full_name": "",
			},
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "forbidden_update_existing_company",
			body: map[string]string{
				"full_name": "New Name",
				"job_role":  "Manager",
				"industry":  "Tech",
			},
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
				d.CompanyRepo.EXPECT().FindByID(gomock.Any(), companyID).Return(&models.Company{
					Industry: new(string),
				}, nil)
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "malformed_json",
			body: "{bad}",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().FindByID(gomock.Any(), userID).Return(&models.User{ID: userID}, nil)
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newUserRouter(deps, companyID, userID), "POST", "/profile", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestUserHandler_ListTeamMembers(t *testing.T) {
	const companyID = "comp_123"

	tests := []struct {
		name       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().
					ListByCompanyID(gomock.Any(), companyID).
					Return([]*models.User{{ID: "u1", Email: "u1@test.com"}}, nil)
				d.UserRoleRepo.EXPECT().
					GetUserRoles(gomock.Any(), "u1").
					Return([]string{"admin"}, nil).AnyTimes()
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.UserRepo.EXPECT().
					ListByCompanyID(gomock.Any(), companyID).
					Return(nil, errors.New("db error"))
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newUserRouter(deps, companyID, "user_1"), "GET", "/members", nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
