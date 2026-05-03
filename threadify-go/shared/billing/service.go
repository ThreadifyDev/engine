package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"threadify-go/shared/config"
	"threadify-go/shared/domain"
	serror "threadify-go/shared/errors"
	"threadify-go/shared/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type BillingService struct {
	billingProvider BillingProvider
	planRepo        repository.PlanRepository
	subConfig       *config.SubscriptionConfig
	billingConfig   *config.BillingConfig
	logger          *zap.Logger
}

func NewBillingService(
	billingProvider BillingProvider,
	planRepo repository.PlanRepository,
	subConfig *config.SubscriptionConfig,
	billingConfig *config.BillingConfig,
	logger *zap.Logger,
) *BillingService {
	return &BillingService{
		billingProvider: billingProvider,
		planRepo:        planRepo,
		subConfig:       subConfig,
		billingConfig:   billingConfig,
		logger:          logger,
	}
}

func (s *BillingService) GetCreditAccount(ctx context.Context, companyID string) (*domain.CreditAccount, error) {
	account, err := s.planRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		s.logger.Error("get credit account: failed to retrieve account",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to retrieve credit account: %w", err)
	}

	s.logger.Info("get credit account: account retrieved successfully",
		zap.String("company_id", companyID),
	)
	return account, nil
}

func (s *BillingService) CreateCheckoutSession(ctx context.Context, companyID string, amountMillicents int64) (string, error) {
	if amountMillicents <= 0 {
		s.logger.Error("create checkout session: amount must be greater than zero",
			zap.String("company_id", companyID),
			zap.Int64("amount_millicents", amountMillicents),
		)
		return "", fmt.Errorf("billing: amount must be greater than zero")
	}

	extCustID, err := s.planRepo.GetExternalCustomerID(ctx, companyID)
	if err != nil {
		s.logger.Error("create checkout session: failed to retrieve external customer id",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return "", fmt.Errorf("get external customer id: %w", err)
	}

	params := domain.CheckoutSessionParams{
		CompanyID:               companyID,
		InitialAmountMillicents: amountMillicents,
		SuccessURL:              s.billingConfig.SuccessURL,
		CancelURL:               s.billingConfig.CancelURL,
		ExternalCustomerID:      extCustID,
	}

	checkoutSessionURL, err := s.billingProvider.CreateCheckoutSession(params)
	if err != nil {
		s.logger.Error("create checkout session: failed to create session",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return "", fmt.Errorf("create checkout session: %w", err)
	}

	s.logger.Info("create checkout session: session created successfully",
		zap.String("company_id", companyID),
		zap.String("checkout_session_url", checkoutSessionURL),
	)
	return checkoutSessionURL, nil
}

func (s *BillingService) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
	err := s.planRepo.UpdateMaxMonthlyCharge(ctx, companyID, maxMonthlyMillicents)
	if err != nil {
		s.logger.Error("update max monthly charge: failed to update max monthly charge",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return fmt.Errorf("update max monthly charge: %w", err)
	}

	s.logger.Info("update max monthly charge: max monthly charge updated successfully",
		zap.String("company_id", companyID),
		zap.Int64("max_monthly_millicents", maxMonthlyMillicents),
	)
	return nil
}

func (s *BillingService) ProvisionSignupCredits(ctx context.Context, companyID string) error {
	if s.subConfig.SignupCreditsMillicents <= 0 {
		s.logger.Warn("provision signup credits: signup credits are disabled",
			zap.String("company_id", companyID),
		)
		return nil
	}

	account, err := s.planRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		s.logger.Error("provision signup credits: failed to retrieve credit account",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return fmt.Errorf("get credit account: %w", err)
	}
	if account != nil {
		s.logger.Info("provision signup credits: credit account already exists",
			zap.String("company_id", companyID),
		)
		return nil
	}

	start := time.Now().UTC().Truncate(24 * time.Hour)
	newAccount := newSignupCreditAccount(
		companyID,
		start,
		s.subConfig.SignupCreditsMillicents,
		s.subConfig.Credit.RateLimitTPS,
		s.subConfig.Credit.PayloadLimitBytes,
	)

	if err := s.planRepo.CreateCreditAccount(ctx, newAccount); err != nil {
		if errors.Is(err, serror.ErrDuplicateCreditAccount) {
			s.logger.Warn("provision signup credits: credit account already exists",
				zap.String("company_id", companyID),
			)
			return nil
		}
		s.logger.Error("provision signup credits: failed to create credit account",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
		return fmt.Errorf("create credit account: %w", err)
	}

	s.logger.Info("provision signup credits: signup credits provisioned successfully",
		zap.String("company_id", companyID),
		zap.Int64("amount_millicents", s.subConfig.SignupCreditsMillicents),
	)

	return nil
}

func newSignupCreditAccount(
	companyID string,
	billingCycleStart time.Time,
	balanceMillicents int64,
	rateLimitTPS int64,
	payloadLimitBytes int64,
) *domain.CreditAccount {
	return &domain.CreditAccount{
		ID:                               uuid.NewString(),
		CompanyID:                        companyID,
		BillingCycleStart:                billingCycleStart,
		CreditBalanceMillicents:          balanceMillicents,
		CreditMinBalanceMillicents:       0,
		CreditMaxMonthlyChargeMillicents: domain.CreditDisabled,
		CreditAutoTopupMillicents:        0,
		CreditMonthlyChargedMillicents:   0,
		RateLimitTPS:                     rateLimitTPS,
		PayloadLimitBytes:                payloadLimitBytes,
	}
}
