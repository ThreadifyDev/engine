package billing

import (
	"context"

	"threadify-go/shared/domain"
)

//go:generate mockgen -package=sharedmocks -destination=../mocks/plan_service_mock.go -source=domain.go
type PlanService interface {
	ChargeContract(ctx context.Context, companyID string) error
	ChargeContractVersion(ctx context.Context, companyID string) error
	DecrementEgress(ctx context.Context, companyID string, bytes int64) error
	DecrementIngress(ctx context.Context, companyID string, count int64) error
	DecrementLLMUsage(ctx context.Context, companyID string, tokens int64) error
	GetCurrentLimits(ctx context.Context, companyID string) (*domain.CreditAccount, error)
	GetExternalCustomerID(ctx context.Context, companyID string) (string, error)
	ProvisionSubscription(ctx context.Context, companyID, externalCustomerID string, initialAmount int64) error
	InvalidatePlanCache(ctx context.Context, companyID string)
	ProcessRollovers(ctx context.Context) error
	CheckPayloadSize(ctx context.Context, account *domain.CreditAccount, payloadBytes int64) error
	CheckRateLimit(ctx context.Context, account *domain.CreditAccount) (bool, error)
	CheckCreditAvailable(ctx context.Context, companyID, meter string, amount int64) error
	CheckBalancePositive(ctx context.Context, companyID string) (*domain.CreditAccount, error)
}
