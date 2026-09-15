package auth

import (
	"context"
	"errors"
)

var ErrManagedAuth = errors.New("identity is managed by Fused Registry; sign in through the Engine")

// ManagedAuthClient rejects retired password operations, including queued legacy outbox jobs.
type ManagedAuthClient struct{}

// RegisterUser delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) RegisterUser(ctx context.Context, email, password, fullName, localUserID, companyID string) (string, error) {
	return "", ErrManagedAuth
}

// LoginWithPassword delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) LoginWithPassword(ctx context.Context, email, password, clientIP string) (string, *AuthUserInfo, error) {
	return "", nil, ErrManagedAuth
}

// Authenticate delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) Authenticate(ctx context.Context, email, password, clientIP string) error {
	return ErrManagedAuth
}

// RequestPasswordReset delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) RequestPasswordReset(ctx context.Context, email string) error {
	return ErrManagedAuth
}

// GeneratePasswordResetToken delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) GeneratePasswordResetToken(ctx context.Context, email string) (string, error) {
	return "", ErrManagedAuth
}

// GenerateSignupOTP delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) GenerateSignupOTP(ctx context.Context, email string) (string, error) {
	return "", ErrManagedAuth
}

// GenerateLoginOTP delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) GenerateLoginOTP(ctx context.Context, email string) (string, error) {
	return "", ErrManagedAuth
}

// ResetPasswordWithOTP delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) ResetPasswordWithOTP(ctx context.Context, token, newPassword string) error {
	return ErrManagedAuth
}

// VerifyEmailWithOTP delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) VerifyEmailWithOTP(ctx context.Context, email, token string) (string, *AuthUserInfo, error) {
	return "", nil, ErrManagedAuth
}

// Logout delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) Logout(ctx context.Context, accessToken string) error { return ErrManagedAuth }

// UpdatePassword delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) UpdatePassword(ctx context.Context, authUserID, email, newPassword string) (string, error) {
	return "", ErrManagedAuth
}

// FindUserIDByEmail delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) FindUserIDByEmail(ctx context.Context, email string) (string, error) {
	return "", ErrManagedAuth
}

// UpdateUserEmail delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) UpdateUserEmail(ctx context.Context, authUserID, newEmail string) error {
	return ErrManagedAuth
}

// DeleteUser delegates identity management to Registry instead of a local password provider.
func (ManagedAuthClient) DeleteUser(ctx context.Context, authUserID string) error {
	return ErrManagedAuth
}
