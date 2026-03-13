package billing

import (
	"context"
	"errors"
	"fmt"
	"threadify-go/shared/config"

	"go.uber.org/zap"
)

var (
	ErrNoActiveSubscription = errors.New("no active subscription associated with this account")
	ErrInvalidTier          = errors.New("invalid subscription tier")
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

func (s *BillingService) CreateCheckoutSession(ctx context.Context, companyID, tier, billingCycle string) (string, error) {
	tierConfig := s.SubConfig.GetTierLimits(tier)
	if tierConfig == nil {
		return "", ErrInvalidTier
	}

	providerParams, err := s.BillingConfig.GetProviderParams(tier, billingCycle)
	if err != nil {
		return "", err
	}

	params := &CheckoutSessionParams{
		CompanyID:      companyID,
		Tier:           tier,
		BillingCycle:   billingCycle,
		SuccessURL:     s.BillingConfig.SuccessURL,
		CancelURL:      s.BillingConfig.CancelURL,
		ProviderParams: providerParams,
	}

	checkoutURL, err := s.BillingProvider.CreateCheckoutSession(params)
	if err != nil {
		return "", fmt.Errorf("failed to initialize checkout: %w", err)
	}

	return checkoutURL, nil
}

func (s *BillingService) GetCurrentPlan(ctx context.Context, companyID string) (*CompanyPlan, *UsageMeter, *config.TierLimits, error) {
	plan, err := s.PlanRepo.GetCompanyPlan(ctx, companyID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to retrieve plan: %w", err)
	}

	meter, err := s.PlanRepo.GetCurrentUsageMeter(ctx, companyID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to retrieve usage meter: %w", err)
	}

	var tierLimits *config.TierLimits
	if plan != nil {
		tierLimits = s.SubConfig.GetTierLimits(string(plan.SubscriptionTier))
	}

	return plan, meter, tierLimits, nil
}

func (s *BillingService) CancelSubscription(ctx context.Context, companyID string) error {
	plan, err := s.PlanRepo.GetCompanyPlan(ctx, companyID)
	if err != nil {
		return fmt.Errorf("failed to retrieve plan: %w", err)
	}

	if plan == nil || plan.Status == StatusCancelled {
		return ErrNoActiveSubscription
	}

	if plan.ExternalSubscriptionID != "" {
		if err := s.BillingProvider.CancelSubscription(plan.ExternalSubscriptionID); err != nil {
			return fmt.Errorf("failed to cancel subscription on provider: %w", err)
		}
	}

	if err := s.PlanRepo.MarkPlanCancelled(ctx, companyID); err != nil {
		return fmt.Errorf("failed to update plan status: %w", err)
	}

	return nil
}

func (s *BillingService) GetTiersConfig() *config.SubscriptionConfig {
	return s.SubConfig
}

func (s *BillingService) GetBillingConfig() *config.BillingConfig {
	return s.BillingConfig
}
