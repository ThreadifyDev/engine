package service_test

import (
	"context"
	"testing"
	"time"

	"threadify-go/api/internal/domain"
	"threadify-go/api/internal/service/tests/common"
	serror "threadify-go/shared/errors"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testEncryptionKey = []byte("01234567890123456789abcdefabcdef")
	testFrontendURL   = "https://app.threadify.ai"
)

func TestTeamInvitationService_SendInvitation(t *testing.T) {
	const (
		companyID = "comp_123"
		email     = "test@example.com"
		role      = "admin"
		invitedBy = "user_456"
	)
	expiryDuration := 24 * time.Hour

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		validate  func(t *testing.T, invitation *domain.TeamInvitation, err error)
	}{
		{
			name: "success",
			setupMock: func(deps *common.MockedDeps) {
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(nil, nil)
				deps.InvitationRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
				deps.OutboxRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
			},
			validate: func(t *testing.T, invitation *domain.TeamInvitation, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, invitation)
				assert.Equal(t, companyID, invitation.CompanyID)
				assert.Equal(t, email, invitation.Email)
				assert.Equal(t, role, invitation.Role)
			},
		},
		{
			name: "user_already_exists",
			setupMock: func(deps *common.MockedDeps) {
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(&domain.User{ID: "user_123"}, nil)
			},
			validate: func(t *testing.T, invitation *domain.TeamInvitation, err error) {
				t.Helper()
				require.Error(t, err)
				assert.ErrorContains(t, err, "user with this email already exists")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewTeamInvitationService(nil, testEncryptionKey, testFrontendURL)
			tt.setupMock(deps)

			invitation, err := svc.SendInvitation(context.Background(), companyID, email, role, invitedBy, expiryDuration)
			tt.validate(t, invitation, err)
		})
	}
}

func TestTeamInvitationService_ValidateToken(t *testing.T) {
	const token = "valid_token"

	tests := []struct {
		name       string
		invitation *domain.TeamInvitation
		wantErr    string
	}{
		{
			name: "success",
			invitation: &domain.TeamInvitation{
				Token:     token,
				Status:    "pending",
				ExpiresAt: time.Now().Add(time.Hour),
			},
		},
		{
			name: "expired",
			invitation: &domain.TeamInvitation{
				Token:     token,
				Status:    "pending",
				ExpiresAt: time.Now().Add(-time.Hour),
			},
			wantErr: "expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewTeamInvitationService(nil, testEncryptionKey, testFrontendURL)

			deps.InvitationRepo.EXPECT().GetByToken(gomock.Any(), token).Return(tt.invitation, nil)
			if tt.invitation != nil && tt.invitation.Status == "pending" && time.Now().Before(tt.invitation.ExpiresAt) {
				deps.CompanyRepo.EXPECT().FindByID(gomock.Any(), tt.invitation.CompanyID).Return(&domain.Company{Name: "Test Co"}, nil)
			}

			res, err := svc.ValidateToken(context.Background(), token)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, res)
			}
		})
	}
}

func TestTeamInvitationService_CancelInvitation(t *testing.T) {
	const invitationID = "inv_123"

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps)
		wantErr   bool
	}{
		{
			name: "success",
			setupMock: func(d *common.MockedDeps) {
				d.InvitationRepo.EXPECT().Delete(gomock.Any(), invitationID).Return(nil)
			},
		},
		{
			name: "not_found",
			setupMock: func(d *common.MockedDeps) {
				d.InvitationRepo.EXPECT().Delete(gomock.Any(), invitationID).Return(serror.ErrInvitationNotFound)
			},
			wantErr: true,
		},
		{
			name: "error",
			setupMock: func(d *common.MockedDeps) {
				d.InvitationRepo.EXPECT().Delete(gomock.Any(), invitationID).Return(assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockDeps(t)
			svc := deps.NewTeamInvitationService(nil, testEncryptionKey, testFrontendURL)
			tt.setupMock(deps)

			err := svc.CancelInvitation(context.Background(), invitationID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
