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

func (s *BillingService) CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents int64) (string, error) {

	extCustID, _ := s.PlanRepo.GetExternalCustomerID(ctx, companyID)

	params := CheckoutSessionParams{
		CompanyID:               companyID,
		InitialAmountMillicents: amountMillicents,
		SuccessURL:              s.BillingConfig.SuccessURL,
		CancelURL:               s.BillingConfig.CancelURL,
		ExternalCustomerID:      extCustID,
	}

	return s.BillingProvider.CreateCheckoutSession(params)
}

func (s *BillingService) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
	return s.PlanRepo.UpdateMaxMonthlyCharge(ctx, companyID, maxMonthlyMillicents)
}
