package auth

import (
	"context"
	"errors"
)

var (
	ErrAuthInvalidCredentials = errors.New("invalid email or password")
	ErrAuthUserAlreadyExists  = errors.New("user with this email already exists")
	ErrAuthUserNotFound       = errors.New("auth user not found")
	ErrAuthInvalidEmail       = errors.New("invalid email address")
	ErrAuthExpiredToken       = errors.New("token has expired")
	ErrAuthInvalidToken       = errors.New("invalid or already used token")
	ErrAuthRateLimit          = errors.New("too many requests, please try again later")
)

type AuthUserInfo struct {
	Sub   string
	Email string

	EmailVerified bool
}

type AuthClient interface {
	RegisterUser(ctx context.Context, email, password, fullName, localUserID, companyID string) (string, error)
	LoginWithPassword(ctx context.Context, email, password, clientIP string) (string, *AuthUserInfo, error)
	Authenticate(ctx context.Context, email, password, clientIP string) error
	RequestPasswordReset(ctx context.Context, email string) error
	GeneratePasswordResetLink(ctx context.Context, email string) (string, error)
	GenerateSignupLink(ctx context.Context, email string) (string, error)
	ResetPasswordWithToken(ctx context.Context, token, newPassword string) error
	VerifyEmailWithToken(ctx context.Context, token string) (string, error)
	UpdatePassword(ctx context.Context, authUserID, email, newPassword string) (string, error)
	FindUserIDByEmail(ctx context.Context, email string) (string, error)
}
