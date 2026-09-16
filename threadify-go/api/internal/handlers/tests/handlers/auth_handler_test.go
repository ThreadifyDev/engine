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

func newAuthRouter(deps *common.MockedHandlers) *gin.Engine {
	h := handlers.NewAuthHandler(deps.AuthSvc)
	r := common.SetupTestRouter()

	r.POST("/login", h.Login)
	r.POST("/signup", h.Signup)
	r.POST("/verify-email", h.VerifyEmail)
	r.POST("/logout", h.Logout)
	r.POST("/forgot-password", h.ForgotPassword)
	r.POST("/reset-password", h.ResetPassword)
	r.POST("/resend-verification", h.ResendVerificationEmail)

	return r
}

func TestAuthHandler_AuthFlows(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		body          any
		setAuthHeader bool
		setupMock     func(*common.MockedHandlers)
		wantStatus    int
	}{
		{
			name:   "login_success",
			method: "POST",
			path:   "/login",
			body:   map[string]string{"email": "test@test.com", "password": "Passw0rd!123456"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					Login(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&domain.AuthSession{Token: "jwt.token.abc"}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "login_invalid_credentials",
			method: "POST",
			path:   "/login",
			body:   map[string]string{"email": "test@test.com", "password": "wrong"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					Login(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, serror.NewDomainError("Invalid credentials", http.StatusUnauthorized))
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "signup_invalid_email",
			method:     "POST",
			path:       "/signup",
			body:       map[string]string{"email": "not-an-email", "password": "Passw0rd!123456", "full_name": "X", "company_name": "Test Co"},
			setupMock:  func(d *common.MockedHandlers) {},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:   "signup_success",
			method: "POST",
			path:   "/signup",
			body:   map[string]string{"email": "new@test.com", "password": "Passw0rd!123456", "full_name": "New User", "company_name": "Test Co"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					Signup(gomock.Any(), gomock.Any()).
					Return(nil)
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:   "signup_conflict",
			method: "POST",
			path:   "/signup",
			body:   map[string]string{"email": "exists@test.com", "password": "Passw0rd!123456", "full_name": "X", "company_name": "Test Co"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					Signup(gomock.Any(), gomock.Any()).
					Return(serror.NewDomainError("User already exists", http.StatusConflict))
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:   "verify_email_success",
			method: "POST",
			path:   "/verify-email",
			body:   map[string]string{"email": "test@test.com", "token": "valid-token"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					VerifyEmail(gomock.Any(), gomock.Any()).
					Return(&domain.AuthSession{Token: "verified-jwt"}, nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "verify_email_fail",
			method: "POST",
			path:   "/verify-email",
			body:   map[string]string{"email": "test@test.com", "token": "bad-token"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().
					VerifyEmail(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("invalid"))
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:          "logout_success",
			method:        "POST",
			path:          "/logout",
			body:          nil,
			setAuthHeader: true,
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().Logout(gomock.Any(), "valid-token").Return(nil)
			},
			wantStatus: http.StatusNoContent,
		},
		{
			name:   "forgot_password_success",
			method: "POST",
			path:   "/forgot-password",
			body:   map[string]string{"email": "test@test.com"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().ForgotPassword(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "reset_password_success",
			method: "POST",
			path:   "/reset-password",
			body:   map[string]string{"token": "valid-token", "password": "Password123!"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().ResetPassword(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantStatus: http.StatusOK,
		},
		{
			name:   "resend_verification_email_success",
			method: "POST",
			path:   "/resend-verification",
			body:   map[string]string{"email": "test@test.com"},
			setupMock: func(d *common.MockedHandlers) {
				d.AuthSvc.EXPECT().ResendVerificationEmail(gomock.Any(), gomock.Any()).Return(nil)
			},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := common.NewMockedHandlers(t)
			tt.setupMock(deps)

			req := common.BuildRequest(t, tt.method, tt.path, tt.body)
			if tt.setAuthHeader {
				req.Header.Set("Authorization", "Bearer valid-token")
			}

			w := common.DoRequestFromReq(t, newAuthRouter(deps), req)
			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}
}
