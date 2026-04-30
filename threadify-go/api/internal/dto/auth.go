package dto

import "time"

type SignupRequest struct {
	CompanyName     string  `json:"company_name"`
	Email           string  `json:"email" binding:"required,email"`
	Password        string  `json:"password" binding:"required,min=12,max=128"`
	FullName        string  `json:"full_name"`
	JobRole         string  `json:"job_role"`
	Industry        *string `json:"industry"`
	CompanySize     *string `json:"company_size"`
	UseCase         *string `json:"use_case"`
	InvitationToken *string `json:"invitation_token"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthUser struct {
	ID                       string     `json:"id"`
	CompanyID                string     `json:"company_id"`
	Email                    string     `json:"email"`
	FullName                 *string    `json:"full_name"`
	JobRole                  *string    `json:"job_role"`
	EmailVerified            bool       `json:"email_verified"`
	OnboardingCompleted      bool       `json:"onboarding_completed"`
	FirstInstrumentationDone bool       `json:"first_instrumentation_done"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	LastLoginAt              *time.Time `json:"last_login_at"`
}

type AuthResponse struct {
	Email                     string    `json:"email,omitempty"`
	Token                     string    `json:"token,omitempty"`
	User                      *AuthUser `json:"user,omitempty"`
	OTPRequired               bool      `json:"otp_required"`
	EmailVerificationRequired bool      `json:"email_verification_required,omitempty"`
	Message                   string    `json:"message,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=12,max=128"`
}

type VerifyEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
	Token string `json:"token" binding:"required"`
}
type ResendVerificationEmailRequest struct {
	Email string `json:"email" binding:"required,email"`
}
