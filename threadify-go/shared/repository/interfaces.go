package repository

import (
	"context"

	"threadify-go/shared/models"
)

//go:generate mockgen -package=sharedmocks -destination=../mocks/repository_mocks.go -source=interfaces.go
type PlanRepository interface {
	GetCreditAccount(ctx context.Context, companyID string) (*models.CreditAccount, error)
	GetExternalCustomerID(ctx context.Context, companyID string) (string, error)
	SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error
	FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error)
	ListCompaniesForRollover(ctx context.Context) (map[string]string, error)
	CreateCreditAccount(ctx context.Context, account *models.CreditAccount) error
	UpdateCumulativeMonthlyCharge(ctx context.Context, id string, amount int64) error
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error
	UpdateTopupSettings(ctx context.Context, companyID string, autoTopupAmount, minBalance int64) error
}

type EntityProfileTypeRepository interface {
	CreateProfileType(ctx context.Context, profileType *models.EntityProfileType) error
	GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*models.EntityProfileType, error)
	GetProfileTypeByType(ctx context.Context, companyID, profileType string) (*models.EntityProfileType, error)
	UpdateProfileType(ctx context.Context, profileType *models.EntityProfileType) error
	ArchiveProfileType(ctx context.Context, companyID, profileTypeID string) error
}

type EntityProfileRepository interface {
	CreateProfile(ctx context.Context, profile *models.EntityProfile) error
	GetProfileByRefKey(ctx context.Context, companyID, profileTypeID, refKey string) (*models.EntityProfile, error)
	GetProfileMetrics(ctx context.Context, entityProfileID string) (*models.EntityProfileMetrics, error)
	GetProfileByIDWithMetrics(ctx context.Context, companyID, entityProfileID string) (*models.EntityProfile, *models.EntityProfileMetrics, error)
	GetProfileWithMetrics(ctx context.Context, companyID, typeName, refKey string) (*models.EntityProfile, *models.EntityProfileMetrics, error)
	ListProfilesByType(ctx context.Context, companyID, typeName, search string, limit, offset int) ([]*ProfileWithMetrics, int, error)
}
