package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"threadify-go/shared/billing"
	sharedconfig "threadify-go/shared/config"
	"threadify-go/shared/database"
	billingmodels "threadify-go/shared/models"
	sharedrepo "threadify-go/shared/repository"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/types"
	"go.uber.org/zap"
)

const (
	creditTopupAppliedKeyTTL = 90 * 24 * time.Hour
	millicentsPerCent        = 1000
)

type BillingOrchestrator struct {
	*billing.BillingService
	billingRepo  types.BillingRepository
	valkey       types.ValkeyStringClient
	creditAtomic types.ValkeyCreditAtomic
	streamClient types.ValkeyStreamClient
	planSvc      types.PlanService
	logger       *zap.Logger
}

func NewBillingOrchestrator(
	billingProvider billing.BillingProvider,
	planRepo sharedrepo.PlanRepository,
	billingRepo types.BillingRepository,
	subConfig *sharedconfig.SubscriptionConfig,
	billingConfig *sharedconfig.BillingConfig,
	valkey types.ValkeyStringClient,
	creditAtomic types.ValkeyCreditAtomic,
	streamClient types.ValkeyStreamClient,
	planSvc types.PlanService,
	logger *zap.Logger,
) *BillingOrchestrator {
	sharedSvc := billing.NewBillingService(billingProvider, planRepo, subConfig, billingConfig, logger)
	return &BillingOrchestrator{
		BillingService: sharedSvc,
		billingRepo:    billingRepo,
		valkey:         valkey,
		creditAtomic:   creditAtomic,
		streamClient:   streamClient,
		planSvc:        planSvc,
		logger:         logger,
	}
}

func (s *BillingOrchestrator) ChargeCreditTopup(ctx context.Context, companyID string, eventID string, billingCycleStart time.Time, amountMillicents int64) error {
	amountCents := amountMillicents / millicentsPerCent
	if amountCents <= 0 {
		return nil
	}

	account, err := s.PlanRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find credit account for credit topup: %w", err)
	}

	if account.CreditAutoTopupMillicents <= 0 || account.CreditMaxMonthlyChargeMillicents <= 0 {
		return fmt.Errorf("auto-topup disabled for company %s", companyID)
	}

	paymentStatus := billingmodels.PaymentStatusPending
	if s.BillingProvider.SkipInvoicing() {
		paymentStatus = billingmodels.PaymentStatusPaid
	}

	now := time.Now().UTC()

	snapshotID := eventID
	if snapshotID == "" {
		snapshotID = uuid.New().String()
	}

	snapshot := &billingmodels.BillingSnapshot{
		ID:                 snapshotID,
		CompanyID:          companyID,
		PeriodStart:        billingCycleStart,
		PeriodEnd:          now,
		TotalCents:         amountCents,
		Reason:             billingmodels.SnapshotReasonCreditTopup,
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
			if updateErr := s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, billingmodels.PaymentStatusFailed); updateErr != nil {
				s.logger.Warn("failed to mark snapshot as failed after invoice error — snapshot may be stuck in Pending",
					zap.String("snapshot_id", snapshot.ID),
					zap.String("company_id", companyID),
					zap.Error(updateErr),
				)
			}
			snapshot.PaymentStatus = billingmodels.PaymentStatusFailed
		}
	}

	s.logger.Info("credit topup snapshot created and billed",
		zap.String("company_id", companyID),
		zap.Int64("total_cents", amountCents),
		zap.String("payment_status", string(snapshot.PaymentStatus)),
	)

	return nil
}

func (s *BillingOrchestrator) ApplyCreditTopup(ctx context.Context, snapshot *billingmodels.BillingSnapshot) error {
	amountMillicents := snapshot.TotalCents * millicentsPerCent
	if amountMillicents <= 0 {
		return nil
	}

	appliedKey := database.CreditTopupAppliedKeyPrefix + snapshot.ID
	if snapshot.ExternalInvoiceID != "" {
		appliedKey = database.CreditTopupAppliedKeyPrefix + snapshot.ExternalInvoiceID
	}

	applied, err := s.valkey.SetNX(ctx, appliedKey, "1", creditTopupAppliedKeyTTL)
	if err != nil {
		s.logger.Error("failed to mark credit topup as applied", zap.Error(err), zap.String("company_id", snapshot.CompanyID))
		return fmt.Errorf("apply credit topup: mark applied: %w", err)
	}

	if !applied {
		s.logger.Info("credit topup already applied", zap.String("company_id", snapshot.CompanyID))
		return nil
	}

	keys := billing.KeysFor(snapshot.CompanyID)

	if _, err := s.creditAtomic.ApplyCreditTopupAtomic(ctx, keys.Balance, keys.Pending, amountMillicents); err != nil {
		_ = s.valkey.Del(ctx, appliedKey)
		s.logger.Error("failed to apply credit topup", zap.Error(err), zap.String("company_id", snapshot.CompanyID))
		return fmt.Errorf("apply credit topup: %w", err)
	}

	s.planSvc.InvalidatePlanCache(ctx, snapshot.CompanyID)

	s.writeCreditTopupToOutbox(ctx, snapshot.CompanyID, amountMillicents, snapshot.PeriodStart)
	return nil
}

func (s *BillingOrchestrator) ClearCreditTopupPending(ctx context.Context, companyID string) error {
	keys := billing.KeysFor(companyID)
	return s.valkey.Del(ctx, keys.Pending)
}

func (s *BillingOrchestrator) writeCreditTopupToOutbox(ctx context.Context, companyID string, amountMillicents int64, billingCycleStart time.Time) {
	eventData := map[string]interface{}{
		fieldEventID:           uuid.NewString(),
		fieldCompanyID:         companyID,
		fieldMeter:             billingmodels.MeterCreditTopup,
		fieldAmount:            strconv.FormatInt(amountMillicents, 10),
		fieldBillingCycleStart: billingCycleStart.Format(time.RFC3339Nano),
		fieldTimestamp:         time.Now().UTC().Format(time.RFC3339Nano),
	}

	if _, err := s.streamClient.XAdd(ctx, database.UsageOutboxStreamKey, "*", eventData); err != nil {
		s.logger.Error("failed to write credit topup event to outbox (fail-open)",
			zap.String("company_id", companyID),
			zap.Int64("amount_millicents", amountMillicents),
			zap.Error(err),
		)
	}

	s.logger.Debug("credit topup event written to outbox", zap.String("company_id", companyID), zap.Int64("amount_millicents", amountMillicents))
}

func (s *BillingOrchestrator) LinkAndMarkSnapshotPaid(ctx context.Context, snapshotID string, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByID(ctx, snapshotID, externalInvoiceID)
}

func (s *BillingOrchestrator) MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingOrchestrator) MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotFailedByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingOrchestrator) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billingmodels.BillingSnapshot, error) {
	return s.billingRepo.FindSnapshotByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingOrchestrator) GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	return s.PlanRepo.FindCompanyByExternalCustomerID(ctx, externalCustomerID)
}

func (s *BillingOrchestrator) ProvisionSubscription(ctx context.Context, companyID string, externalCustomerID string, initialAmount int64) error {
	return s.planSvc.ProvisionSubscription(ctx, companyID, externalCustomerID, initialAmount)
}

func (s *BillingOrchestrator) ProcessRollovers(ctx context.Context) error {
	return s.planSvc.ProcessRollovers(ctx)
}
