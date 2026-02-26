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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Industry  *string   `json:"industry,omitempty"`
	Size      *string   `json:"size,omitempty"` // small, medium, large, enterprise
	UseCase   *string   `json:"use_case,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

type APIKey struct {
	ID               string     `json:"id"`
	KeyHash          string     `json:"-"` // Never expose key hash
	KeyPrefix        string     `json:"key_prefix"`
	Name             string     `json:"name"`
	UserID           *string    `json:"user_id,omitempty"`
	ServiceAccountID *string    `json:"service_account_id,omitempty"`
	CompanyID        string     `json:"company_id"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
}

// Request/Response DTOs

type SignupRequest struct {
	CompanyName string  `json:"company_name" binding:"required,min=2,max=255"`
	Email       string  `json:"email" binding:"required,email"`
	Password    string  `json:"password" binding:"required,min=12,max=128"`
	FullName    string  `json:"full_name"`
	JobRole     string  `json:"job_role"`
	Industry    *string `json:"industry"`
	CompanySize *string `json:"company_size"`
	UseCase     *string `json:"use_case"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	Token   string `json:"token"`
	User    *User  `json:"user"`
	Message string `json:"message,omitempty"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=12,max=128"`
}

type VerifyEmailRequest struct {
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
