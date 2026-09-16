package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                       string
	CompanyID                string
	Email                    string
	AuthUserID               *string
	FullName                 *string
	JobRole                  *string
	EmailVerified            bool
	OnboardingCompleted      bool
	FirstInstrumentationDone bool
	CreatedAt                time.Time
	UpdatedAt                time.Time
	LastLoginAt              *time.Time
}

func (u *User) IsOnboarded() bool {
	return u.OnboardingCompleted
}

func (u *User) GenerateArchivedEmail() string {
	return fmt.Sprintf("archived-%s-%s", uuid.New().String(), u.Email)
}

type Company struct {
	ID                 string
	Name               string
	ExternalCustomerID string
	Industry           *string
	Size               *string
	UseCase            *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (c *Company) HasDetails() bool {
	return c.Industry != nil || c.Size != nil || c.UseCase != nil
}

type ServiceAccount struct {
	ID          string
	CompanyID   string
	Name        string
	Description *string
	IsActive    bool
	CreatedBy   *string
	LastUsedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Permission struct {
	ID          string
	Name        string
	Description string
	Resource    string
	Action      string
	CreatedAt   time.Time
}

type TeamMember struct {
	ID        string
	Email     string
	FullName  *string
	JobRole   *string
	Role      string
	CreatedAt time.Time
}

func (c *Company) CanUpdateDetails(newIndustry, newSize, newUseCase *string) error {
	hasNewDetails := (newIndustry != nil && *newIndustry != "") ||
		(newSize != nil && *newSize != "") ||
		(newUseCase != nil && *newUseCase != "")

	if !c.HasDetails() && !hasNewDetails {
		return fmt.Errorf("Company details are required for first-time setup")
	}
	if c.HasDetails() && hasNewDetails {
		return fmt.Errorf("company details cannot be modified after initial setup")
	}
	return nil
}
