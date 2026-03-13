package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/billing"
	sharedconfig "threadify-go/shared/config"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

type BillingService struct {
	*billing.BillingService
	realPlanRepo *postgres.PlanRepository
	billingRepo  *postgres.BillingRepository
	valkeyClient interfaces.ValkeyClient
	planSvc      *PlanService
	logger       *zap.Logger
}

func NewBillingService(
	billingProvider billing.BillingProvider,
	planRepo *postgres.PlanRepository,
	billingRepo *postgres.BillingRepository,
	subConfig *sharedconfig.SubscriptionConfig,
	billingConfig *sharedconfig.BillingConfig,
	valkeyClient interfaces.ValkeyClient,
	planSvc *PlanService,
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

func (s *BillingService) SnapshotAndBill(ctx context.Context, companyID string, periodStart, periodEnd time.Time, reason billing.SnapshotReason) error {
	plan, err := s.PlanRepo.GetCompanyPlan(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find plan for billing: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("no plan found for company %s", companyID)
	}

	meter, err := s.PlanRepo.GetCurrentUsageMeter(ctx, companyID)
	if err != nil {
		return fmt.Errorf("find usage meter for billing: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no usage meter found for company %s", companyID)
	}

	tierLimits := s.SubConfig.GetTierLimits(string(plan.SubscriptionTier))
	if tierLimits == nil {
		return fmt.Errorf("unknown tier: %s", plan.SubscriptionTier)
	}

	ingressFinal := meter.BandwidthIngressBalance
	if balance, ok := s.readBalanceFromValkey(ctx, fmt.Sprintf("%singress:%s", database.BalanceKeyPrefix, companyID)); ok {
		ingressFinal = balance
	}

	egressFinal := meter.BandwidthEgressBalance
	if balance, ok := s.readBalanceFromValkey(ctx, fmt.Sprintf("%segress:%s", database.BalanceKeyPrefix, companyID)); ok {
		egressFinal = balance
	}

	meterForBilling := *meter
	meterForBilling.BandwidthIngressBalance = ingressFinal
	meterForBilling.BandwidthEgressBalance = egressFinal

	lineItems := calculateOverageLineItems(&meterForBilling, tierLimits)

	var totalCents int64
	for _, item := range lineItems {
		totalCents += item.AmountCents
	}

	consecutiveCount := 0
	prevSnapshot, err := s.billingRepo.FindLatestSnapshot(ctx, companyID)
	if err == nil && prevSnapshot != nil {
		if len(lineItems) > 0 {
			consecutiveCount = prevSnapshot.ConsecutiveOverageCount + 1
		}
	}

	isCycleEnd := reason == billing.SnapshotReasonMonthlyRenewal || reason == billing.SnapshotReasonYearlyRenewal

	paymentStatus := billing.PaymentStatusNoCharge
	if totalCents > 0 && !s.BillingProvider.SkipInvoicing() {
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
		ProviderName:            s.BillingProvider.Name(),
		ConsecutiveOverageCount: consecutiveCount,
		PaymentStatus:           paymentStatus,
		ExternalCustomerID:      plan.ExternalCustomerID,
		ExternalSubscriptionID:  plan.ExternalSubscriptionID,
		CreatedAt:               time.Now(),
	}

	if err := s.billingRepo.CreateSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("persist billing snapshot: %w", err)
	}

	if totalCents > 0 && !s.BillingProvider.SkipInvoicing() {
		result, invoiceErr := s.BillingProvider.IssueOverage(snapshot)
		if result != nil && result.ExternalInvoiceID != "" {
			snapshot.ExternalInvoiceID = result.ExternalInvoiceID
			_ = s.billingRepo.UpdateSnapshotInvoiceID(ctx, snapshot.ID, result.ExternalInvoiceID)
		}

		if invoiceErr != nil {
			s.logger.Error("failed to create overage invoice", zap.Error(invoiceErr), zap.String("company_id", companyID))
			_ = s.billingRepo.UpdateSnapshotPaymentStatus(ctx, snapshot.ID, billing.PaymentStatusFailed)
			snapshot.PaymentStatus = billing.PaymentStatusFailed
		}
	}

	s.logger.Info("billing snapshot created",
		zap.String("company_id", companyID),
		zap.Int64("total_cents", totalCents),
		zap.String("payment_status", string(snapshot.PaymentStatus)),
	)

	if reason == billing.SnapshotReasonOverageOnly {
		if err := s.resetBalances(ctx, companyID, meter.ID, tierLimits); err != nil {
			return fmt.Errorf("reset balances: %w", err)
		}
	}

	return nil
}

func (s *BillingService) readBalanceFromValkey(ctx context.Context, key string) (int64, bool) {
	raw, err := s.valkeyClient.Get(ctx, key)
	if err != nil || strings.TrimSpace(raw) == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func (s *BillingService) resetBalances(ctx context.Context, companyID, meterID string, tierCfg *sharedconfig.TierLimits) error {
	ingressKey := fmt.Sprintf("%singress:%s", database.BalanceKeyPrefix, companyID)
	egressKey := fmt.Sprintf("%segress:%s", database.BalanceKeyPrefix, companyID)

	_ = s.valkeyClient.Set(ctx, ingressKey, fmt.Sprintf("%d", tierCfg.BandwidthIngress), 0)
	_ = s.valkeyClient.Set(ctx, egressKey, fmt.Sprintf("%d", tierCfg.BandwidthEgress), 0)

	cacheKey := fmt.Sprintf("%s%s", database.PlanCachePrefix, companyID)
	_ = s.valkeyClient.Delete(ctx, cacheKey)

	return s.realPlanRepo.ResetMeterBalances(ctx, meterID, tierCfg.BandwidthIngress, tierCfg.BandwidthEgress)
}

func (s *BillingService) RenewAndReset(ctx context.Context, companyID string, snapshot *billing.BillingSnapshot) error {
	plan, err := s.PlanRepo.GetCompanyPlan(ctx, companyID)
	if err != nil || plan == nil {
		return fmt.Errorf("find plan for renewal: %w", err)
	}

	tierCfg := s.SubConfig.GetTierLimits(string(plan.SubscriptionTier))
	if tierCfg == nil {
		return fmt.Errorf("unknown tier: %s", plan.SubscriptionTier)
	}

	if err := s.planSvc.RenewSubscriptionFromBillingEnd(ctx, companyID, plan.SubscriptionTier, plan.BillingCycle, plan.BillingEnd, plan.ExternalCustomerID, plan.ExternalSubscriptionID); err != nil {
		return err
	}

	newMeter, err := s.PlanRepo.GetCurrentUsageMeter(ctx, companyID)
	if err != nil || newMeter == nil {
		return fmt.Errorf("failed to get new meter for reset: %w", err)
	}

	return s.resetBalances(ctx, companyID, newMeter.ID, tierCfg)
}

func (s *BillingService) SuspendCompany(ctx context.Context, companyID string) error {
	key := fmt.Sprintf("plan:suspended:%s", companyID)
	return s.valkeyClient.Set(ctx, key, "1", 0)
}

func (s *BillingService) LiftSuspension(ctx context.Context, companyID string) error {
	key := fmt.Sprintf("plan:suspended:%s", companyID)
	return s.valkeyClient.Delete(ctx, key)
}

func (s *BillingService) CancelPlan(ctx context.Context, companyID string) error {
	if err := s.PlanRepo.MarkPlanCancelled(ctx, companyID); err != nil {
		return err
	}
	return s.SuspendCompany(ctx, companyID)
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

func (s *BillingService) FindPendingSnapshotBySubscriptionID(ctx context.Context, externalSubscriptionID string) (*billing.BillingSnapshot, error) {
	return s.billingRepo.FindPendingSnapshotBySubscriptionID(ctx, externalSubscriptionID)
}

func (s *BillingService) GetCompanyIDByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	return s.realPlanRepo.FindCompanyByExternalCustomerID(ctx, externalCustomerID)
}

func (s *BillingService) ProvisionSubscription(ctx context.Context, companyID string, tier billing.PlanTier, billingCycle billing.BillingCycle, externalCustomerID, externalSubscriptionID string) error {
	if err := s.planSvc.ProvisionSubscription(ctx, companyID, tier, billingCycle, externalCustomerID, externalSubscriptionID); err != nil {
		return err
	}
	return s.LiftSuspension(ctx, companyID)
}

func calculateOverageLineItems(meter *billing.UsageMeter, tierCfg *sharedconfig.TierLimits) []billing.InvoiceLineItem {
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
		const gb = 1024 * 1024 * 1024
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
