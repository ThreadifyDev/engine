package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/billing"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

type BillingService struct {
	planRepo        *postgres.PlanRepository
	billingRepo     *postgres.BillingRepository
	subscriptionCfg *config.SubscriptionConfig
	valkeyClient    interfaces.ValkeyClient
	invoiceProvider interfaces.InvoiceProvider
	planSvc         *PlanService
	logger          *zap.Logger
}

func NewBillingService(
	planRepo *postgres.PlanRepository,
	billingRepo *postgres.BillingRepository,
	subscriptionCfg *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	invoiceProvider interfaces.InvoiceProvider,
	planSvc *PlanService,
	logger *zap.Logger,
) *BillingService {
	return &BillingService{
		planRepo:        planRepo,
		billingRepo:     billingRepo,
		subscriptionCfg: subscriptionCfg,
		valkeyClient:    valkeyClient,
		invoiceProvider: invoiceProvider,
		planSvc:         planSvc,
		logger:          logger,
	}
}

func (s *BillingService) SnapshotAndBill(ctx context.Context, companyID string, periodStart, periodEnd time.Time, reason billing.SnapshotReason) error {
	plan, err := s.planRepo.FindPlanByCompanyID(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find plan for billing: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("no plan found for company %s", companyID)
	}

	meter, err := s.planRepo.FindCurrentUsageMeter(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find usage meter for billing: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no usage meter found for company %s", companyID)
	}

	tierCfg := s.subscriptionCfg.GetTierLimits(string(plan.SubscriptionTier))
	if tierCfg == nil {
		return fmt.Errorf("unknown tier %q for company %s", plan.SubscriptionTier, companyID)
	}

	ingressFinal := meter.BandwidthIngressBalance
	if balance, ok := s.readBalanceFromValkey(ctx, fmt.Sprintf("%singress:%s", database.BalanceKeyPrefix, companyID)); ok {
		ingressFinal = balance
	} else {
		s.logger.Warn("billing: using stale postgres ingress balance — live valkey counter unavailable",
			zap.String("company_id", companyID),
			zap.Int64("postgres_balance", ingressFinal),
		)
	}

	egressFinal := meter.BandwidthEgressBalance
	if balance, ok := s.readBalanceFromValkey(ctx, fmt.Sprintf("%segress:%s", database.BalanceKeyPrefix, companyID)); ok {
		egressFinal = balance
	} else {
		s.logger.Warn("billing: using stale postgres egress balance — live valkey counter unavailable",
			zap.String("company_id", companyID),
			zap.Int64("postgres_balance", egressFinal),
		)
	}

	meterForBilling := *meter
	meterForBilling.BandwidthIngressBalance = ingressFinal
	meterForBilling.BandwidthEgressBalance = egressFinal

	lineItems := calculateOverageLineItems(&meterForBilling, tierCfg)

	var totalCents int64
	for _, item := range lineItems {
		totalCents += item.AmountCents
	}

	consecutiveCount := 0
	prevSnapshot, err := s.billingRepo.FindLatestSnapshot(ctx, companyID)
	if err != nil {
		s.logger.Warn("failed to find previous snapshot, starting count at 0",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
	}

	hasOverage := len(lineItems) > 0 && totalCents > 0
	if hasOverage {
		if prevSnapshot != nil {
			consecutiveCount = prevSnapshot.ConsecutiveOverageCount + 1
		} else {
			consecutiveCount = 1
		}
	}

	isCycleEnd := reason == billing.SnapshotReasonMonthlyRenewal || reason == billing.SnapshotReasonYearlyRenewal

	paymentStatus := billing.PaymentStatusNoCharge
	if totalCents > 0 && !s.invoiceProvider.SkipInvoicing() {
		paymentStatus = billing.PaymentStatusPending
	}

	snapshot := &billing.BillingSnapshot{
		ID:                      uuid.New().String(),
		CompanyID:               companyID,
		Tier:                    plan.SubscriptionTier,
		PeriodStart:             periodStart,
		PeriodEnd:               periodEnd,
		Reason:                  reason,
		IsCycleEnd:              isCycleEnd,
		IngressBalanceFinal:     ingressFinal,
		EgressBalanceFinal:      egressFinal,
		MaxIngress:              meter.MaxBandwidthIngress,
		MaxEgress:               meter.MaxBandwidthEgress,
		LineItems:               lineItems,
		TotalCents:              totalCents,
		ProviderName:            s.invoiceProvider.Name(),
		ConsecutiveOverageCount: consecutiveCount,
		PaymentStatus:           paymentStatus,
		ExternalCustomerID:      plan.ExternalCustomerID,
		ExternalSubscriptionID:  plan.ExternalSubscriptionID,
	}

	if err := s.billingRepo.CreateSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("persist billing snapshot: %w", err)
	}

	if totalCents > 0 && !s.invoiceProvider.SkipInvoicing() {
		result, invoiceErr := s.invoiceProvider.IssueOverage(snapshot)

		if result != nil && result.ExternalInvoiceID != "" {
			snapshot.ExternalInvoiceID = result.ExternalInvoiceID
			if repoErr := s.billingRepo.UpdateSnapshotInvoiceID(ctx, snapshot.ID, result.ExternalInvoiceID); repoErr != nil {
				s.logger.Error("CRITICAL: failed to persist external invoice ID — manual reconciliation required",
					zap.String("snapshot_id", snapshot.ID),
					zap.String("external_invoice_id", result.ExternalInvoiceID),
					zap.String("company_id", companyID),
					zap.Error(repoErr),
				)
			}
		}

		if invoiceErr != nil {
			newStatus := billing.PaymentStatusFailed
			if reason == billing.SnapshotReasonOverageOnly {
				s.logger.Error("immediate overage invoice failed — webhook will handle suspension",
					zap.String("company_id", companyID),
					zap.String("snapshot_id", snapshot.ID),
					zap.Error(invoiceErr),
				)
			} else {
				s.logger.Error("failed to queue overage items on upcoming subscription invoice — no webhook will fire, manual retry required",
					zap.String("company_id", companyID),
					zap.String("snapshot_id", snapshot.ID),
					zap.String("reason", string(reason)),
					zap.Error(invoiceErr),
				)
			}
			if repoErr := s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, newStatus); repoErr != nil {
				s.logger.Error("failed to persist invoice failure status",
					zap.String("snapshot_id", snapshot.ID),
					zap.Error(repoErr),
				)
			}
			snapshot.PaymentStatus = newStatus
		} else if reason == billing.SnapshotReasonOverageOnly && (result == nil || strings.TrimSpace(result.ExternalInvoiceID) == "") {
			newStatus := billing.PaymentStatusFailed
			if repoErr := s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, newStatus); repoErr != nil {
				s.logger.Error("failed to persist missing-invoice failure status",
					zap.String("snapshot_id", snapshot.ID),
					zap.Error(repoErr),
				)
			}
			snapshot.PaymentStatus = newStatus
			s.logger.Error("overage invoice returned success without external invoice ID",
				zap.String("company_id", companyID),
				zap.String("snapshot_id", snapshot.ID),
				zap.String("provider", s.invoiceProvider.Name()),
			)
		}
	}

	s.logger.Info("billing snapshot created",
		zap.String("company_id", companyID),
		zap.String("tier", string(plan.SubscriptionTier)),
		zap.Int64("total_cents", totalCents),
		zap.Int("consecutive_overage", consecutiveCount),
		zap.String("reason", string(reason)),
		zap.String("payment_status", string(snapshot.PaymentStatus)),
	)

	if reason == billing.SnapshotReasonOverageOnly {
		if err := s.resetBalances(ctx, companyID, tierCfg); err != nil {
			return fmt.Errorf("reset balances: %w", err)
		}
	}

	return nil
}

func (s *BillingService) RenewAndReset(ctx context.Context, companyID string, snapshot *billing.BillingSnapshot) error {
	plan, err := s.planRepo.FindPlanByCompanyID(ctx, companyID)
	if err != nil {
		return fmt.Errorf("renew and reset: find plan: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("renew and reset: no plan found for company %s", companyID)
	}

	tierCfg := s.subscriptionCfg.GetTierLimits(string(plan.SubscriptionTier))
	if tierCfg == nil {
		return fmt.Errorf("renew and reset: unknown tier %q for company %s", plan.SubscriptionTier, companyID)
	}

	if err := s.planSvc.RenewSubscriptionFromBillingEnd(ctx, companyID, plan.SubscriptionTier, plan.BillingCycle, plan.BillingEnd, plan.ExternalCustomerID, plan.ExternalSubscriptionID); err != nil {
		return fmt.Errorf("renew and reset: renew subscription: %w", err)
	}

	if err := s.resetBalances(ctx, companyID, tierCfg); err != nil {
		return fmt.Errorf("renew and reset: reset balances: %w", err)
	}

	return nil
}

func (s *BillingService) ProvisionSubscription(ctx context.Context, companyID string, tier billing.PlanTier, billingCycle billing.BillingCycle, externalCustomerID, externalSubscriptionID string) error {
	return s.planSvc.ProvisionSubscription(ctx, companyID, tier, billingCycle, externalCustomerID, externalSubscriptionID)
}

func (s *BillingService) readBalanceFromValkey(ctx context.Context, key string) (int64, bool) {
	raw, err := s.valkeyClient.Get(ctx, key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return 0, false
	}

	value, parseErr := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if parseErr != nil {
		s.logger.Warn("failed to parse valkey meter balance, falling back to postgres value",
			zap.String("key", key),
			zap.String("value", raw),
			zap.Error(parseErr),
		)
		return 0, false
	}
	return value, true
}

func (s *BillingService) resetBalances(ctx context.Context, companyID string, tierCfg *config.TierLimits) error {
	ingressKey := fmt.Sprintf("%singress:%s", database.BalanceKeyPrefix, companyID)
	egressKey := fmt.Sprintf("%segress:%s", database.BalanceKeyPrefix, companyID)

	var resetErr error

	if err := s.valkeyClient.Set(ctx, ingressKey, fmt.Sprintf("%d", tierCfg.BandwidthIngress), 0); err != nil {
		s.logger.Error("failed to reset valkey ingress balance", zap.Error(err))
		resetErr = errors.Join(resetErr, fmt.Errorf("reset valkey ingress balance: %w", err))
	}
	if err := s.valkeyClient.Set(ctx, egressKey, fmt.Sprintf("%d", tierCfg.BandwidthEgress), 0); err != nil {
		s.logger.Error("failed to reset valkey egress balance", zap.Error(err))
		resetErr = errors.Join(resetErr, fmt.Errorf("reset valkey egress balance: %w", err))
	}

	cacheKey := fmt.Sprintf("%s%s", database.PlanCachePrefix, companyID)
	if err := s.valkeyClient.Delete(ctx, cacheKey); err != nil {
		s.logger.Warn("failed to invalidate plan cache on cycle reset", zap.Error(err))
	}

	if err := s.planRepo.ResetMeterBalances(ctx, companyID, tierCfg.BandwidthIngress, tierCfg.BandwidthEgress); err != nil {
		s.logger.Error("failed to reset postgres balances on cycle reset", zap.Error(err))
		resetErr = errors.Join(resetErr, fmt.Errorf("reset postgres balances: %w", err))
	}

	if resetErr != nil {
		return resetErr
	}

	s.logger.Info("balances reset for new cycle",
		zap.String("company_id", companyID),
		zap.Int64("new_ingress", tierCfg.BandwidthIngress),
		zap.Int64("new_egress", tierCfg.BandwidthEgress),
	)
	return nil
}

func (s *BillingService) SuspendCompany(ctx context.Context, companyID string) error {
	key := fmt.Sprintf("plan:suspended:%s", companyID)
	if err := s.valkeyClient.Set(ctx, key, "1", 0); err != nil {
		return fmt.Errorf("suspend company: %w", err)
	}
	s.logger.Warn("company suspended", zap.String("company_id", companyID))
	return nil
}

func (s *BillingService) LiftSuspension(ctx context.Context, companyID string) error {
	key := fmt.Sprintf("plan:suspended:%s", companyID)
	if err := s.valkeyClient.Delete(ctx, key); err != nil {
		return fmt.Errorf("lift suspension: %w", err)
	}
	s.logger.Info("company suspension lifted", zap.String("company_id", companyID))
	return nil
}

func (s *BillingService) CancelPlan(ctx context.Context, companyID string) error {
	if err := s.planRepo.MarkPlanCancelled(ctx, companyID); err != nil {
		return fmt.Errorf("cancel plan: %w", err)
	}
	if err := s.SuspendCompany(ctx, companyID); err != nil {
		return fmt.Errorf("cancel plan: suspend: %w", err)
	}
	s.logger.Warn("plan cancelled", zap.String("company_id", companyID))
	return nil
}

func (s *BillingService) MarkSnapshotPaid(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) LinkAndMarkSnapshotPaid(ctx context.Context, snapshotID string, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotPaidByID(ctx, snapshotID, externalInvoiceID)
}

func (s *BillingService) MarkSnapshotFailed(ctx context.Context, externalInvoiceID string) error {
	return s.billingRepo.MarkSnapshotFailedByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	return s.planRepo.FindCompanyByExternalCustomerID(ctx, externalCustomerID)
}

func (s *BillingService) FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billing.BillingSnapshot, error) {
	return s.billingRepo.FindSnapshotByInvoiceID(ctx, externalInvoiceID)
}

func (s *BillingService) FindPendingSnapshotBySubscriptionID(ctx context.Context, externalSubscriptionID string) (*billing.BillingSnapshot, error) {
	return s.billingRepo.FindPendingSnapshotBySubscriptionID(ctx, externalSubscriptionID)
}

func calculateOverageLineItems(meter *models.UsageMeter, tierCfg *config.TierLimits) []billing.InvoiceLineItem {
	var items []billing.InvoiceLineItem

	if meter.BandwidthIngressBalance < 0 && tierCfg.BandwidthIngressOverageCentsPerMillion > 0 {
		overage := -meter.BandwidthIngressBalance
		millions := overage / 1_000_000
		if overage%1_000_000 > 0 {
			millions++
		}
		items = append(items, billing.InvoiceLineItem{
			Meter:       "bandwidth_ingress",
			OverageQty:  overage,
			UnitLabel:   "per 1M requests",
			RateCents:   tierCfg.BandwidthIngressOverageCentsPerMillion,
			AmountCents: millions * int64(tierCfg.BandwidthIngressOverageCentsPerMillion),
		})
	}

	if meter.BandwidthEgressBalance < 0 && tierCfg.BandwidthEgressOverageCentsPerGB > 0 {
		overage := -meter.BandwidthEgressBalance
		const gb = 1_073_741_824
		gbs := overage / gb
		if overage%gb > 0 {
			gbs++
		}
		items = append(items, billing.InvoiceLineItem{
			Meter:       "bandwidth_egress",
			OverageQty:  overage,
			UnitLabel:   "per GB",
			RateCents:   tierCfg.BandwidthEgressOverageCentsPerGB,
			AmountCents: gbs * int64(tierCfg.BandwidthEgressOverageCentsPerGB),
		})
	}

	return items
}
