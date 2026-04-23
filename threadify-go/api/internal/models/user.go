package models

import "time"

type User struct {
	ID                       string     `json:"id"`
	CompanyID                string     `json:"company_id"`
	Email                    string     `json:"email"`
	AuthUserID               *string    `json:"-"` // Internal identity mapping for Auth migration/sync
	FullName                 *string    `json:"full_name,omitempty"`
	JobRole                  *string    `json:"job_role,omitempty"`
	EmailVerified            bool       `json:"email_verified"`
	OnboardingCompleted      bool       `json:"onboarding_completed"`
	FirstInstrumentationDone bool       `json:"first_instrumentation_done"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	LastLoginAt              *time.Time `json:"last_login_at,omitempty"`
}

type Company struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	ExternalCustomerID string    `json:"-"`
	Industry           *string   `json:"industry,omitempty"`
	Size               *string   `json:"size,omitempty"` // small, medium, large, enterprise
	UseCase            *string   `json:"use_case,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ServiceAccount struct {
	ID          string     `json:"id"`
	CompanyID   string     `json:"company_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	IsActive    bool       `json:"is_active"`
	CreatedBy   *string    `json:"created_by,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Permission struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	CreatedAt   time.Time `json:"created_at"`
}

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

type AuthResponse struct {
	Email                     string `json:"email,omitempty"`
	Token                     string `json:"token,omitempty"`
	User                      *User  `json:"user,omitempty"`
	OTPRequired               bool   `json:"otp_required"`
	EmailVerificationRequired bool   `json:"email_verification_required,omitempty"`
	Message                   string `json:"message,omitempty"`
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

type UpdateProfileRequest struct {
	FullName    string `json:"full_name"`
	JobRole     string `json:"job_role"`
	Industry    string `json:"industry"`
	CompanySize string `json:"company_size"`
	UseCase     string `json:"use_case"`
}

type UserProfileResult struct {
	User    *User
	Company *Company
}

type TeamMember struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  *string   `json:"full_name,omitempty"`
	JobRole   *string   `json:"job_role,omitempty"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
