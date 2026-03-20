package billing

import (
	"context"
	"fmt"
	"threadify-go/shared/config"

	"go.uber.org/zap"
)

type BillingService struct {
	BillingProvider BillingProvider
	PlanRepo        PlanRepository
	SubConfig       *config.SubscriptionConfig
	BillingConfig   *config.BillingConfig
	Logger          *zap.Logger
}

func NewBillingService(
	billingProvider BillingProvider,
	planRepo PlanRepository,
	subConfig *config.SubscriptionConfig,
	billingConfig *config.BillingConfig,
	logger *zap.Logger,
) *BillingService {
	return &BillingService{
		BillingProvider: billingProvider,
		PlanRepo:        planRepo,
		SubConfig:       subConfig,
		BillingConfig:   billingConfig,
		Logger:          logger,
	}
}

func (s *BillingService) GetCreditAccount(ctx context.Context, companyID string) (*CreditAccount, error) {
	account, err := s.PlanRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credit account: %w", err)
	}
	return account, nil
}

func (s *BillingService) CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents, maxMonthlyMillicents int64) (string, error) {
	if maxMonthlyMillicents < amountMillicents {
		maxMonthlyMillicents = amountMillicents
	}

	extCustID, _ := s.PlanRepo.GetExternalCustomerID(ctx, companyID)

	params := CheckoutSessionParams{
		CompanyID:               companyID,
		InitialAmountMillicents: amountMillicents,
		MaxMonthlyMillicents:    maxMonthlyMillicents,
		SuccessURL:              s.BillingConfig.SuccessURL,
		CancelURL:               s.BillingConfig.CancelURL,
		ExternalCustomerID:      extCustID,
	}

	return s.BillingProvider.CreateCheckoutSession(params)
}
