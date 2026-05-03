package dto

import (
	"time"
)

type User struct {
	ID                       string     `json:"id"`
	Email                    string     `json:"email"`
	FullName                 *string    `json:"full_name"`
	JobRole                  *string    `json:"job_role"`
	EmailVerified            bool       `json:"email_verified"`
	OnboardingCompleted      bool       `json:"onboarding_completed"`
	FirstInstrumentationDone bool       `json:"first_instrumentation_done"`
	LastLoginAt              *time.Time `json:"last_login_at"`
}

type Company struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Industry *string `json:"industry"`
	Size     *string `json:"company_size"`
	UseCase  *string `json:"use_case"`
}

type UserProfileResult struct {
	User    *User    `json:"user"`
	Company *Company `json:"company"`
}

type UpdateProfileRequest struct {
	FullName    string `json:"full_name" binding:"required"`
	JobRole     string `json:"job_role" binding:"required"`
	Industry    string `json:"industry"`
	CompanySize string `json:"company_size"`
	UseCase     string `json:"use_case"`
}

type TeamMember struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	JobRole   string    `json:"job_role"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
