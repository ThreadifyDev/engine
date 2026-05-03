package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"threadify-go/shared/billing"
	sharedconfig "threadify-go/shared/config"
	"threadify-go/shared/database"
	shareddomain "threadify-go/shared/domain"
	sharedrepo "threadify-go/shared/repository"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

const (
	creditTopupAppliedKeyTTL = 90 * 24 * time.Hour
	millicentsPerCent        = 1000
)

type BillingOrchestrator struct {
	*billing.BillingService
	billingProvider billing.BillingProvider
	billingRepo     domain.BillingRepository
	valkey          domain.ValkeyStringClient
	creditAtomic    domain.ValkeyCreditAtomic
	streamClient    domain.ValkeyStreamClient
	planRepo        sharedrepo.PlanRepository
	planSvc         domain.PlanService
	logger          *zap.Logger
}

func NewBillingOrchestrator(
	billingProvider billing.BillingProvider,
	planRepo sharedrepo.PlanRepository,
	billingRepo domain.BillingRepository,
	subConfig *sharedconfig.SubscriptionConfig,
	billingConfig *sharedconfig.BillingConfig,
	valkey domain.ValkeyStringClient,
	creditAtomic domain.ValkeyCreditAtomic,
	streamClient domain.ValkeyStreamClient,
	planSvc domain.PlanService,
	logger *zap.Logger,
) *BillingOrchestrator {
	sharedSvc := billing.NewBillingService(billingProvider, planRepo, subConfig, billingConfig, logger)
	return &BillingOrchestrator{
		BillingService:  sharedSvc,
		billingProvider: billingProvider,
		billingRepo:     billingRepo,
		valkey:          valkey,
		creditAtomic:    creditAtomic,
		streamClient:    streamClient,
		planRepo:        planRepo,
		planSvc:         planSvc,
		logger:          logger,
	}
}

func (s *BillingOrchestrator) ChargeCreditTopup(ctx context.Context, companyID string, eventID string, billingCycleStart time.Time, amountMillicents int64) error {
	amountCents := amountMillicents / millicentsPerCent
	if amountCents <= 0 {
		s.logger.Debug("credit topup amount is zero or negative, skipping", zap.Int64("amount_millicents", amountMillicents))
		return nil
	}

	account, err := s.planRepo.GetCreditAccount(ctx, companyID)
	if err != nil {
		s.logger.Error("failed to find credit account for credit topup", zap.Error(err))
		return fmt.Errorf("find credit account for credit topup: %w", err)
	}

	if account.CreditAutoTopupMillicents <= 0 || account.CreditMaxMonthlyChargeMillicents <= 0 {
		s.logger.Warn("auto-topup disabled for company", zap.String("company_id", companyID))
		return fmt.Errorf("auto-topup disabled for company %s", companyID)
	}

	paymentStatus := shareddomain.PaymentStatusPending
	if s.billingProvider.SkipInvoicing() {
		s.logger.Debug("skipping invoicing, marking payment as paid")
		paymentStatus = shareddomain.PaymentStatusPaid
	}

	now := time.Now().UTC()

	snapshotID := eventID
	if snapshotID == "" {
		snapshotID = uuid.New().String()
	}

	snapshot := &shareddomain.BillingSnapshot{
		ID:                 snapshotID,
		CompanyID:          companyID,
		PeriodStart:        billingCycleStart,
		PeriodEnd:          now,
		TotalCents:         amountCents,
		Reason:             shareddomain.SnapshotReasonCreditTopup,
		ProviderName:       s.billingProvider.Name(),
		PaymentStatus:      paymentStatus,
		ExternalCustomerID: account.ExternalCustomerID,
		CreatedAt:          now,
	}

	if err := s.billingRepo.CreateSnapshot(ctx, snapshot); err != nil {
		s.logger.Error("failed to persist credit topup snapshot", zap.Error(err))
		return fmt.Errorf("persist credit topup snapshot: %w", err)
	}

	if !s.billingProvider.SkipInvoicing() {
		result, invoiceErr := s.billingProvider.IssueTopupInvoice(snapshot)
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
			if updateErr := s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, shareddomain.PaymentStatusFailed); updateErr != nil {
				s.logger.Warn("failed to mark snapshot as failed after invoice error — snapshot may be stuck in Pending",
					zap.String("snapshot_id", snapshot.ID),
					zap.String("company_id", companyID),
					zap.Error(updateErr),
				)
			}
			snapshot.PaymentStatus = shareddomain.PaymentStatusFailed
		}
	}

	s.logger.Info("credit topup snapshot created and billed",
		zap.String("company_id", companyID),
		zap.Int64("total_cents", amountCents),
		zap.String("payment_status", string(snapshot.PaymentStatus)),
	)

	return nil
}

func (s *BillingOrchestrator) ApplyCreditTopup(ctx context.Context, snapshot *shareddomain.BillingSnapshot) error {
	amountMillicents := snapshot.TotalCents * millicentsPerCent
	if amountMillicents <= 0 {
		s.logger.Debug("credit topup amount is zero or negative, skipping", zap.Int64("total_cents", snapshot.TotalCents))
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
		s.logger.Debug("credit topup already applied", zap.String("company_id", snapshot.CompanyID))
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
	s.logger.Debug("credit topup applied", zap.String("company_id", snapshot.CompanyID), zap.Int64("amount_millicents", amountMillicents))
	return nil
}

func (s *BillingOrchestrator) ClearCreditTopupPending(ctx context.Context, companyID string) error {
	keys := billing.KeysFor(companyID)
	err := s.valkey.Del(ctx, keys.Pending)
	if err != nil {
		s.logger.Error("failed to clear credit topup pending", zap.Error(err), zap.String("company_id", companyID))
		return fmt.Errorf("clear credit topup pending: %w", err)
	}
	s.logger.Debug("cleared credit topup pending", zap.String("company_id", companyID))
	return nil
}

func (s *BillingOrchestrator) writeCreditTopupToOutbox(ctx context.Context, companyID string, amountMillicents int64, billingCycleStart time.Time) {
	eventData := map[string]interface{}{
		fieldEventID:           uuid.NewString(),
		fieldCompanyID:         companyID,
		fieldMeter:             shareddomain.MeterCreditTopup,
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
	err := s.billingRepo.MarkSnapshotPaidByID(ctx, snapshotID, externalInvoiceID)
	if err != nil {
		s.logger.Error("failed to mark snapshot paid", zap.Error(err), zap.String("snapshot_id", snapshotID), zap.String("external_invoice_id", externalInvoiceID))
		return fmt.Errorf("mark snapshot paid: %w", err)
	}
	return nil
}

func (s *BillingOrchestrator) MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error {
	err := s.billingRepo.MarkSnapshotPaidByInvoiceID(ctx, externalInvoiceID)
	if err != nil {
		s.logger.Error("failed to mark snapshot paid", zap.Error(err), zap.String("external_invoice_id", externalInvoiceID))
		return fmt.Errorf("mark snapshot paid: %w", err)
	}
	return nil
}

func (s *BillingOrchestrator) MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error {
	err := s.billingRepo.MarkSnapshotFailedByInvoiceID(ctx, externalInvoiceID)
	if err != nil {
		s.logger.Error("failed to mark snapshot failed", zap.Error(err), zap.String("external_invoice_id", externalInvoiceID))
		return fmt.Errorf("mark snapshot failed: %w", err)
	}
	return nil
}

func (s *BillingOrchestrator) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*shareddomain.BillingSnapshot, error) {
	snapshot, err := s.billingRepo.FindSnapshotByInvoiceID(ctx, externalInvoiceID)
	if err != nil {
		s.logger.Error("failed to find snapshot by invoice ID", zap.Error(err), zap.String("external_invoice_id", externalInvoiceID))
		return nil, fmt.Errorf("find snapshot by invoice ID: %w", err)
	}
	return snapshot, nil
}

func (s *BillingOrchestrator) GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	companyID, err := s.planRepo.FindCompanyByExternalCustomerID(ctx, externalCustomerID)
	if err != nil {
		s.logger.Error("failed to get company ID by external customer ID", zap.Error(err), zap.String("external_customer_id", externalCustomerID))
		return "", fmt.Errorf("get company ID by external customer ID: %w", err)
	}
	return companyID, nil
}

func (s *BillingOrchestrator) ProvisionSubscription(ctx context.Context, companyID string, externalCustomerID string, initialAmount int64) error {
	err := s.planSvc.ProvisionSubscription(ctx, companyID, externalCustomerID, initialAmount)
	if err != nil {
		s.logger.Error("failed to provision subscription", zap.Error(err), zap.String("company_id", companyID), zap.String("external_customer_id", externalCustomerID))
		return fmt.Errorf("provision subscription: %w", err)
	}
	return nil
}

func (s *BillingOrchestrator) ProcessRollovers(ctx context.Context) error {
	err := s.planSvc.ProcessRollovers(ctx)
	if err != nil {
		s.logger.Error("failed to process rollovers", zap.Error(err))
		return fmt.Errorf("process rollovers: %w", err)
	}
	return nil
}
