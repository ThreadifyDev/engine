package auth

import (
	"context"
	"testing"
	"time"

	"threadify-go/api/internal/models"
	"threadify-go/api/internal/service"
	authmocks "threadify-go/api/internal/service/mocks/service/auth"
	"threadify-go/api/internal/service/tests/common"
	sharedauth "threadify-go/shared/auth"
	serror "threadify-go/shared/errors"
	sharedmocks "threadify-go/shared/mocks"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const (
	testEncryptionKey = "30313233343536373839616263646566"
)

func TestAuthService_Signup(t *testing.T) {
	const (
		email       = "test@example.com"
		password    = "Password123!"
		companyName = "Test Co"
	)

	tests := []struct {
		name      string
		req       *models.SignupRequest
		setupMock func(deps *common.MockedDeps, pool *authmocks.MockDBPool, tx *authmocks.MockTx)
		wantErr   bool
	}{
		{
			name: "success_regular_signup",
			req: &models.SignupRequest{
				Email:       email,
				Password:    password,
				CompanyName: companyName,
			},
			setupMock: func(deps *common.MockedDeps, pool *authmocks.MockDBPool, tx *authmocks.MockTx) {
				pool.EXPECT().Begin(gomock.Any()).Return(tx, nil)
				tx.EXPECT().Rollback(gomock.Any()).AnyTimes()
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(nil, nil)
				deps.CompanyRepo.EXPECT().CreateTx(gomock.Any(), tx, gomock.Any()).Return(nil)
				deps.UserRepo.EXPECT().CreateTx(gomock.Any(), tx, gomock.Any()).Return(nil)
				deps.UserRoleRepo.EXPECT().AssignRoleToUserTx(gomock.Any(), tx, gomock.Any(), "owner", "system").Return(nil)
				deps.OutboxRepo.EXPECT().CreateTx(gomock.Any(), tx, gomock.Any()).Return(nil)
				tx.EXPECT().Commit(gomock.Any()).Return(nil)
			},
		},
		{
			name: "user_already_exists",
			req: &models.SignupRequest{
				Email:       email,
				Password:    password,
				CompanyName: companyName,
			},
			setupMock: func(deps *common.MockedDeps, pool *authmocks.MockDBPool, tx *authmocks.MockTx) {
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(&models.User{ID: "existing"}, nil)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			deps := common.NewMockDeps(t)
			pool := authmocks.NewMockDBPool(ctrl)
			tx := authmocks.NewMockTx(ctrl)
			authClient := sharedmocks.NewMockAuthClient(ctrl)

			tt.setupMock(deps, pool, tx)

			svc := deps.NewAuthService(pool, authClient, nil, testEncryptionKey)

			err := svc.Signup(context.Background(), tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthService_Login(t *testing.T) {
	const (
		email    = "test@example.com"
		password = "Password123!"
		clientIP = "127.0.0.1"
	)

	tests := []struct {
		name      string
		req       *models.LoginRequest
		setupMock func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient)
		wantErr   bool
		wantOTP   bool
		errType   error
	}{
		{
			name: "success_supabase_login",
			req:  &models.LoginRequest{Email: email, Password: password},
			setupMock: func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient) {
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(&models.User{ID: "user_123"}, nil)
				authClient.EXPECT().LoginWithPassword(gomock.Any(), email, password, clientIP).Return("auth_user_id", &sharedauth.AuthUserInfo{Email: email, Sub: "auth_user_id"}, nil)
				deps.UserRepo.EXPECT().GetPasswordHash(gomock.Any(), email).Return("", nil)
				authClient.EXPECT().GenerateLoginOTP(gomock.Any(), email).Return("123456", nil)
				deps.EmailSvc.EXPECT().SendLoginOTPEmail(gomock.Any(), email, "123456").Return(nil)
			},
			wantOTP: true,
		},
		{
			name: "legacy_user_migration_required",
			req:  &models.LoginRequest{Email: email, Password: password},
			setupMock: func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient) {
				hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(&models.User{ID: "user_123", AuthUserID: nil}, nil)
				authClient.EXPECT().LoginWithPassword(gomock.Any(), email, password, clientIP).Return("", nil, sharedauth.ErrAuthInvalidCredentials)
				deps.UserRepo.EXPECT().GetPasswordHash(gomock.Any(), email).Return(string(hash), nil)
				deps.OutboxRepo.EXPECT().ExistsPendingByReference(gomock.Any(), models.EventTypeMigrateLegacyUser, "user_123").Return(false, nil)
				deps.OutboxRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
				// Note: OTP is sent by the migration worker for legacy users, so GenerateLoginOTP is not called in the main login method for this path
			},
			wantOTP: true,
		},
		{
			name: "invalid_credentials",
			req:  &models.LoginRequest{Email: email, Password: "wrong_password"},
			setupMock: func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient) {
				deps.UserRepo.EXPECT().FindByEmail(gomock.Any(), email).Return(&models.User{ID: "user_123"}, nil)
				authClient.EXPECT().LoginWithPassword(gomock.Any(), email, "wrong_password", clientIP).Return("", nil, sharedauth.ErrAuthInvalidCredentials)
				deps.UserRepo.EXPECT().GetPasswordHash(gomock.Any(), email).Return("", serror.ErrNotFound)
			},
			wantErr: true,
			errType: service.ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			deps := common.NewMockDeps(t)
			authClient := sharedmocks.NewMockAuthClient(ctrl)
			pool := authmocks.NewMockDBPool(ctrl)

			tt.setupMock(deps, authClient)

			svc := deps.NewAuthService(pool, authClient, nil, testEncryptionKey)

			resp, err := svc.Login(context.Background(), tt.req, clientIP)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantOTP, resp.OTPRequired)
			}
		})
	}
}

func TestAuthService_ResetPassword(t *testing.T) {
	tests := []struct {
		name      string
		req       *models.ResetPasswordRequest
		setupMock func(authClient *sharedmocks.MockAuthClient)
		wantErr   error
	}{
		{
			name: "success",
			req:  &models.ResetPasswordRequest{Token: "valid", Password: "NewPassword123!"},
			setupMock: func(authClient *sharedmocks.MockAuthClient) {
				authClient.EXPECT().ResetPasswordWithOTP(gomock.Any(), "valid", "NewPassword123!").Return(nil)
			},
		},
		{
			name: "invalid_token",
			req:  &models.ResetPasswordRequest{Token: "invalid", Password: "NewPassword123!"},
			setupMock: func(authClient *sharedmocks.MockAuthClient) {
				authClient.EXPECT().ResetPasswordWithOTP(gomock.Any(), "invalid", "NewPassword123!").Return(sharedauth.ErrAuthInvalidToken)
			},
			wantErr: service.ErrAuthInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			deps := common.NewMockDeps(t)
			authClient := sharedmocks.NewMockAuthClient(ctrl)
			pool := authmocks.NewMockDBPool(ctrl)

			tt.setupMock(authClient)

			svc := deps.NewAuthService(pool, authClient, nil, testEncryptionKey)
			err := svc.ResetPassword(context.Background(), tt.req)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthService_VerifyEmail(t *testing.T) {
	const (
		email = "test@example.com"
		token = "123456"
	)

	tests := []struct {
		name      string
		setupMock func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient)
		wantErr   error
	}{
		{
			name: "success_new_verification",
			setupMock: func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient) {
				authClient.EXPECT().VerifyEmailWithOTP(gomock.Any(), email, token).Return("access_token", &sharedauth.AuthUserInfo{Sub: "auth_id", Email: email}, nil)
				deps.UserRepo.EXPECT().FindByAuthUserID(gomock.Any(), "auth_id").Return(&models.User{ID: "user_123", Email: email, EmailVerified: false}, nil)
				deps.UserRepo.EXPECT().UpdateEmailVerified(gomock.Any(), "user_123", true).Return(nil)
				deps.UserRepo.EXPECT().UpdateLastLogin(gomock.Any(), "user_123").Return(nil)
				deps.EmailSvc.EXPECT().SendWelcomeEmail(gomock.Any(), email, gomock.Any()).Return(nil)
			},
		},
		{
			name: "token_expired",
			setupMock: func(deps *common.MockedDeps, authClient *sharedmocks.MockAuthClient) {
				authClient.EXPECT().VerifyEmailWithOTP(gomock.Any(), email, token).Return("", nil, sharedauth.ErrAuthExpiredToken)
			},
			wantErr: service.ErrExpiredToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			deps := common.NewMockDeps(t)
			authClient := sharedmocks.NewMockAuthClient(ctrl)
			pool := authmocks.NewMockDBPool(ctrl)

			tt.setupMock(deps, authClient)

			svc := deps.NewAuthService(pool, authClient, nil, testEncryptionKey)
			_, err := svc.VerifyEmail(context.Background(), &models.VerifyEmailRequest{Email: email, Token: token})

			// Wait a bit for the async welcome email goroutine
			time.Sleep(200 * time.Millisecond)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
