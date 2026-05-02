package repository

import (
	"context"

	"threadify-go/shared/domain"
)

//go:generate mockgen -package=sharedmocks -destination=../mocks/repository_mocks.go -source=repository.go
type PlanRepository interface {
	GetCreditAccount(ctx context.Context, companyID string) (*domain.CreditAccount, error)
	GetExternalCustomerID(ctx context.Context, companyID string) (string, error)
	SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error
	FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error)
	ListCompaniesForRollover(ctx context.Context) (map[string]string, error)
	CreateCreditAccount(ctx context.Context, account *domain.CreditAccount) error
	UpdateCumulativeMonthlyCharge(ctx context.Context, id string, amount int64) error
	UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error
	UpdateTopupSettings(ctx context.Context, companyID string, autoTopupAmount, minBalance int64) error
}

type EntityProfileTypeRepository interface {
	CreateProfileType(ctx context.Context, profileType *domain.EntityProfileType) error
	GetProfileTypesByCompanyID(ctx context.Context, companyID string) ([]*domain.EntityProfileType, error)
	GetProfileTypeByID(ctx context.Context, profileTypeID string) (*domain.EntityProfileType, error)
	GetProfileTypeByType(ctx context.Context, companyID, profileType string) (*domain.EntityProfileType, error)
	UpdateProfileType(ctx context.Context, profileType *domain.EntityProfileType) error
	ArchiveProfileType(ctx context.Context, companyID, profileTypeID string) error
	ListMetricsTemplates(ctx context.Context) ([]*domain.MetricsTemplate, error)
	GetMetricsTemplate(ctx context.Context, templateID string) (*domain.MetricsTemplate, error)
	ValidateMetricsSQL(ctx context.Context, sql string, params map[string]any) error
}
type EntityProfileRepository interface {
	CreateProfile(ctx context.Context, profile *domain.EntityProfile) error
	GetProfileByRefKey(ctx context.Context, companyID, profileTypeID, refKey string) (*domain.EntityProfile, error)
	GetProfileByID(ctx context.Context, companyID, profileID string) (*domain.EntityProfile, error)
	GetProfileByTypeName(ctx context.Context, companyID, typeSlug, refKey string) (*domain.EntityProfile, error)
	ListProfilesByType(ctx context.Context, companyID, typeName, search string, limit, offset int) ([]*domain.EntityProfile, int, error)
}
