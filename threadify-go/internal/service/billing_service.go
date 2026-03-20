package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"threadify-go/shared/billing"
	sharedconfig "threadify-go/shared/config"

	"threadify-go/shared/database"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

const creditTopupAppliedKeyTTL = 90 * 24 * time.Hour

type BillingService struct {
	*billing.BillingService
	realPlanRepo interfaces.PlanRepository
	billingRepo  interfaces.BillingRepository
	valkeyClient interfaces.ValkeyClient
	planSvc      interfaces.PlanService
	logger       *zap.Logger
}

func NewBillingService(
	billingProvider billing.BillingProvider,
	planRepo interfaces.PlanRepository,
	billingRepo interfaces.BillingRepository,
	subConfig *sharedconfig.SubscriptionConfig,
	billingConfig *sharedconfig.BillingConfig,
	valkeyClient interfaces.ValkeyClient,
	planSvc interfaces.PlanService,
	logger *zap.Logger,
) *BillingService {
	sharedSvc := billing.NewBillingService(billingProvider, planRepo, subConfig, billingConfig, logger)

	return &BillingService{
		BillingService: sharedSvc,
		realPlanRepo:   planRepo,
		billingRepo:    billingRepo,
		valkeyClient:   valkeyClient,
		planSvc:        planSvc,
		logger:         logger,
	}
}

func (s *BillingService) ChargeCreditTopup(ctx context.Context, companyID string, eventID string, billingCycleStart time.Time, amountMillicents int64) error {
	amountCents := amountMillicents / 1000
	if amountCents <= 0 {
		return nil
	}

	account, err := s.realPlanRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find credit account for credit topup: %w", err)
	}

	if account.CreditAutoTopupMillicents <= 0 || account.CreditMaxMonthlyChargeMillicents <= 0 {
		return fmt.Errorf("auto-topup disabled for company %s", companyID)
	}

	paymentStatus := billing.PaymentStatusPending
	if s.BillingProvider.SkipInvoicing() {
		paymentStatus = billing.PaymentStatusPaid
	}

	now := time.Now().UTC()

	snapshotID := eventID
	if snapshotID == "" {
		snapshotID = uuid.New().String()
	}

	snapshot := &billing.BillingSnapshot{
		ID:                 snapshotID,
		CompanyID:          companyID,
		PeriodStart:        billingCycleStart,
		PeriodEnd:          now,
		TotalCents:         amountCents,
		Reason:             billing.SnapshotReasonCreditTopup,
		ProviderName:       s.BillingProvider.Name(),
		PaymentStatus:      paymentStatus,
		ExternalCustomerID: account.ExternalCustomerID,
		CreatedAt:          now,
	}

	if err := s.billingRepo.CreateSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("persist credit topup snapshot: %w", err)
	}

	if !s.BillingProvider.SkipInvoicing() {
		result, invoiceErr := s.BillingProvider.IssueTopupInvoice(snapshot)
		if result != nil && result.ExternalInvoiceID != "" {
			snapshot.ExternalInvoiceID = result.ExternalInvoiceID
			if updateErr := s.billingRepo.UpdateSnapshotInvoiceID(ctx, snapshot.ID, result.ExternalInvoiceID); updateErr != nil {
				s.logger.Error("failed to persist invoice ID on snapshot",
					zap.String("snapshot_id", snapshot.ID),
					zap.String("invoice_id", result.ExternalInvoiceID),
					zap.Error(updateErr),
				)
			}
		}

		if invoiceErr != nil {
			s.logger.Error("failed to create credit topup invoice", zap.Error(invoiceErr), zap.String("company_id", companyID))
			if updateErr := s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, billing.PaymentStatusFailed); updateErr != nil {
				s.logger.Warn("failed to mark snapshot as failed after invoice error — snapshot may be stuck in Pending",
					zap.String("snapshot_id", snapshot.ID),
					zap.String("company_id", companyID),
					zap.Error(updateErr),
				)
			}
			snapshot.PaymentStatus = billing.PaymentStatusFailed
		}
	}

	s.logger.Info("credit topup snapshot created and billed",
		zap.String("company_id", companyID),
		zap.Int64("total_cents", amountCents),
		zap.String("payment_status", string(snapshot.PaymentStatus)),
	)

	return nil
}

func (s *BillingService) ApplyCreditTopup(ctx context.Context, snapshot *billing.BillingSnapshot) error {
	amountMillicents := snapshot.TotalCents * 1000
	if amountMillicents <= 0 {
		return nil
	}

	appliedKey := database.CreditTopupAppliedKeyPrefix + snapshot.ID
	if snapshot.ExternalInvoiceID != "" {
		appliedKey = database.CreditTopupAppliedKeyPrefix + snapshot.ExternalInvoiceID
	}

	applied, err := s.valkeyClient.SetNX(ctx, appliedKey, "1", creditTopupAppliedKeyTTL)
	if err != nil {
		s.logger.Error("failed to mark credit topup as applied", zap.Error(err), zap.String("company_id", snapshot.CompanyID))
		return fmt.Errorf("apply credit topup: mark applied: %w", err)
	}
	if !applied {
		s.logger.Info("credit topup already applied", zap.String("company_id", snapshot.CompanyID))
		return nil
	}

	keys := billing.KeysFor(snapshot.CompanyID)

	if _, err := s.valkeyClient.IncrBy(ctx, keys.Balance, amountMillicents); err != nil {
		s.logger.Error("failed to increment credit balance", zap.Error(err), zap.String("company_id", snapshot.CompanyID))
		return fmt.Errorf("apply credit topup: increment balance: %w", err)
	}
	if err := s.valkeyClient.Delete(ctx, keys.Pending); err != nil {
		s.logger.Warn("failed to delete pending key", zap.Error(err), zap.String("company_id", snapshot.CompanyID))
	}

	s.planSvc.InvalidatePlanCache(ctx, snapshot.CompanyID)

	s.writeCreditTopupToOutbox(ctx, snapshot.CompanyID, amountMillicents, snapshot.PeriodStart)
	return nil
}

func (s *BillingService) ClearCreditTopupPending(ctx context.Context, companyID string) error {
	keys := billing.KeysFor(companyID)
	return s.valkeyClient.Delete(ctx, keys.Pending)
}

func (s *BillingService) writeCreditTopupToOutbox(ctx context.Context, companyID string, amountMillicents int64, billingCycleStart time.Time) {
	eventData := map[string]interface{}{
		fieldEventID:           uuid.NewString(),
		fieldCompanyID:         companyID,
		fieldMeter:             "credit_topup",
		fieldAmount:            strconv.FormatInt(amountMillicents, 10),
		fieldBillingCycleStart: billingCycleStart.Format(time.RFC3339Nano),
		fieldTimestamp:         time.Now().UTC().Format(time.RFC3339Nano),
	}

	if _, err := s.valkeyClient.XAdd(ctx, database.UsageOutboxStreamKey, "*", eventData); err != nil {
		s.logger.Error("failed to write credit topup event to outbox (fail-open)",
			zap.String("company_id", companyID),
			zap.Int64("amount_millicents", amountMillicents),
			zap.Error(err),
		)
	}

	s.logger.Debug("credit topup event written to outbox", zap.String("company_id", companyID), zap.Int64("amount_millicents", amountMillicents))
}

func (s *BillingService) DisableAutoTopup(ctx context.Context, companyID string) error {
	return s.realPlanRepo.DisableAutoTopup(ctx, companyID)
}

func (s *BillingService) LinkAndMarkSnapshotPaid(ctx context.Context, snapshotID string, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByID(ctx, snapshotID, externalInvoiceID)
}

func (s *BillingService) MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotFailedByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billing.BillingSnapshot, error) {
	return s.billingRepo.FindSnapshotByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	return s.realPlanRepo.FindCompanyByExternalCustomerID(ctx, externalCustomerID)
}

func (s *BillingService) ProvisionSubscription(ctx context.Context, companyID string, externalCustomerID string, initialAmount, maxMonthly int64) error {
	return s.planSvc.ProvisionSubscription(ctx, companyID, externalCustomerID, initialAmount, maxMonthly)
}

func (s *BillingService) ProcessRollovers(ctx context.Context) error {
	return s.planSvc.ProcessRollovers(ctx)
}
