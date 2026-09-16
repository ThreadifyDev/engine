package tests

import (
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

func newInvitationRouter(deps *common.MockedHandlers, companyID, userID string) *gin.Engine {
	h := handlers.NewTeamInvitationHandler(deps.InvitationSvc)
	r := common.SetupTestRouter()
	r.Use(common.WithAuthContext(common.AuthIDs{CompanyID: companyID, UserID: userID}))
	r.POST("/invitations", h.SendInvitation)
	r.GET("/invitations", h.ListInvitations)
	r.DELETE("/invitations/:id", h.CancelInvitation)
	return r
}

func TestTeamInvitationHandler_SendInvitation(t *testing.T) {
	const (
		companyID = "comp_123"
		adminID   = "user_admin"
	)

	tests := []struct {
		name       string
		body       any
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name: "success",
			body: map[string]string{"email": "new@test.com", "role": "member"},
			setupMock: func(d *common.MockedHandlers) {
				d.InvitationSvc.EXPECT().
					SendInvitation(gomock.Any(), companyID, "new@test.com", "member", adminID, gomock.Any()).
					Return(&domain.TeamInvitation{ID: "inv_1"}, nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "invalid_email",
			body:       map[string]string{"email": "bad-email", "role": "member"},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing_role",
			body:       map[string]string{"email": "new@test.com"},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "already_invited_conflict",
			body: map[string]string{"email": "exists@test.com", "role": "member"},
			setupMock: func(d *common.MockedHandlers) {
				d.InvitationSvc.EXPECT().
					SendInvitation(gomock.Any(), companyID, "exists@test.com", "member", adminID, gomock.Any()).
					Return(nil, serror.NewDomainError("Already invited", http.StatusConflict))
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "malformed_json",
			body:       "{inv",
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newInvitationRouter(deps, companyID, adminID), "POST", "/invitations", tt.body)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}

func TestTeamInvitationHandler_InvitationManagement(t *testing.T) {
	const (
		companyID = "comp_123"
		invID     = "inv_789"
	)

	tests := []struct {
		name       string
		method     string
		path       string
		setupMock  func(*common.MockedHandlers)
		wantStatus int
	}{
		{
			name:   "list_success",
			method: "GET",
			path:   "/invitations",
			setupMock: func(d *common.MockedHandlers) {
				d.InvitationSvc.EXPECT().
					ListByCompany(gomock.Any(), companyID).
					Return([]*domain.TeamInvitation{{ID: "inv_1"}}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "cancel_success",
			method: "DELETE",
			path:   "/invitations/" + invID,
			setupMock: func(d *common.MockedHandlers) {
				d.InvitationSvc.EXPECT().
					GetByID(gomock.Any(), invID).
					Return(&domain.TeamInvitation{ID: invID, CompanyID: companyID, Status: "pending"}, nil)
				d.InvitationSvc.EXPECT().
					CancelInvitation(gomock.Any(), invID).
					Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "cancel_not_found",
			method: "DELETE",
			path:   "/invitations/missing",
			setupMock: func(d *common.MockedHandlers) {
				d.InvitationSvc.EXPECT().
					GetByID(gomock.Any(), "missing").
					Return(nil, serror.NewDomainError("not found", http.StatusNotFound))
			},
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)
			w := common.DoRequest(t, newInvitationRouter(deps, companyID, "u1"), tt.method, tt.path, nil)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
