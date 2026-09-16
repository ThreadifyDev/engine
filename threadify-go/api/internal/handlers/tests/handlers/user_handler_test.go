package tests

import (
	"errors"
	"net/http"
	"testing"

	"threadify-go/api/internal/handlers"
	"threadify-go/api/internal/handlers/tests/common"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/management/domain"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func newUserRouter(deps *common.MockedHandlers, companyID, userID string) *gin.Engine {
	h := handlers.NewUserHandler(deps.UserSvc)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
	r.Use(func(c *gin.Context) { c.Set("roles", []string{"admin"}); c.Next() })

	r.GET("/profile", h.GetProfile)
	r.POST("/profile", h.UpdateProfile)
	r.GET("/members", h.ListTeamMembers)
	r.DELETE("/members/:id", h.RemoveTeamMember)

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
				d.UserSvc.EXPECT().
					GetProfile(gomock.Any(), userID, companyID).
					Return(&domain.UserProfile{
						User:    &domain.User{ID: userID, Email: "user@example.com"},
						Company: &domain.Company{ID: companyID, Name: "Acme Corp"},
					}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "user_not_found",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					GetProfile(gomock.Any(), userID, companyID).
					Return(nil, serror.ErrUserNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					GetProfile(gomock.Any(), userID, companyID).
					Return(nil, errors.New("service fail"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name: "minimal_profile_success",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					GetProfile(gomock.Any(), userID, companyID).
					Return(&domain.UserProfile{
						User:    &domain.User{ID: userID},
						Company: &domain.Company{ID: companyID},
					}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "company_forbidden",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					GetProfile(gomock.Any(), userID, companyID).
					Return(nil, serror.NewDomainError("Forbidden", http.StatusForbidden))
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
				d.UserSvc.EXPECT().
					UpdateProfile(gomock.Any(), userID, companyID, gomock.Any()).
					Return(&domain.User{ID: userID}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "validation_error_missing_fields",
			body: map[string]string{
				"full_name": "",
			},
			setupMock: func(d *common.MockedHandlers) {
				// Validation happens in handler before service call
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
				d.UserSvc.EXPECT().
					UpdateProfile(gomock.Any(), userID, companyID, gomock.Any()).
					Return(nil, serror.NewDomainError("Forbidden", http.StatusForbidden))
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "malformed_json",
			body: "{bad}",
			setupMock: func(d *common.MockedHandlers) {
				// Malformed JSON fails before service call
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
				d.UserSvc.EXPECT().
					ListTeamMembers(gomock.Any(), companyID).
					Return([]*domain.TeamMember{{ID: "u1", Email: "u1@test.com", Role: "admin"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "service_error",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					ListTeamMembers(gomock.Any(), companyID).
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

func TestUserHandler_RemoveTeamMember(t *testing.T) {
	const (
		companyID     = "comp_123"
		currentUserID = "user_admin"
		targetUserID  = "user_to_remove"
	)

	tests := []struct {
		name       string
		memberID   string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name:     "cannot_remove_self",
			memberID: currentUserID,
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					RemoveTeamMember(gomock.Any(), currentUserID, companyID, currentUserID).
					Return(serror.NewDomainError("Cannot remove yourself", http.StatusBadRequest))
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "user_not_found",
			memberID: "non-existent",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					RemoveTeamMember(gomock.Any(), currentUserID, companyID, "non-existent").
					Return(serror.ErrUserNotFound)
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name:     "forbidden_cross_company",
			memberID: "other_user",
			setupMock: func(d *common.MockedHandlers) {
				d.UserSvc.EXPECT().
					RemoveTeamMember(gomock.Any(), currentUserID, companyID, "other_user").
					Return(serror.NewDomainError("Forbidden", http.StatusForbidden))
			},
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newUserRouter(deps, companyID, currentUserID), "DELETE", "/members/"+tt.memberID, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
