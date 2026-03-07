package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
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
	logger          *zap.Logger
}

func NewBillingService(
	planRepo *postgres.PlanRepository,
	billingRepo *postgres.BillingRepository,
	subscriptionCfg *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	invoiceProvider interfaces.InvoiceProvider,
	logger *zap.Logger,
) *BillingService {
	return &BillingService{
		planRepo:        planRepo,
		billingRepo:     billingRepo,
		subscriptionCfg: subscriptionCfg,
		valkeyClient:    valkeyClient,
		invoiceProvider: invoiceProvider,
		logger:          logger,
	}
}

func (s *BillingService) SnapshotAndBill(ctx context.Context, companyID string, periodStart, periodEnd time.Time, reason models.SnapshotReason) error {
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

	// Calculate overage line items
	lineItems := calculateOverageLineItems(meter, tierCfg)

	var totalCents int64
	for _, item := range lineItems {
		totalCents += item.AmountCents
	}

	// Determine consecutive overage count
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
	// If no overage, consecutiveCount stays 0 (streak resets)

	snapshot := &models.BillingSnapshot{
		ID:                      uuid.New().String(),
		CompanyID:               companyID,
		Tier:                    plan.SubscriptionTier,
		PeriodStart:             periodStart,
		PeriodEnd:               periodEnd,
		Reason:                  reason,
		IsCycleEnd:              reason == models.SnapshotReasonMonthlyRenewal || reason == models.SnapshotReasonYearlyRenewal,
		IngressBalanceFinal:     meter.BandwidthIngressBalance,
		EgressBalanceFinal:      meter.BandwidthEgressBalance,
		MaxIngress:              meter.MaxBandwidthIngress,
		MaxEgress:               meter.MaxBandwidthEgress,
		LineItems:               lineItems,
		TotalCents:              totalCents,
		ProviderName:            s.invoiceProvider.Name(),
		ConsecutiveOverageCount: consecutiveCount,
	}

	if err := s.billingRepo.CreateSnapshot(ctx, snapshot); err != nil {
		return fmt.Errorf("persist billing snapshot: %w", err)
	}

	// Send to invoice provider (Stripe, Paystack, NoOp, etc.)
	if totalCents > 0 {
		result, invoiceErr := s.invoiceProvider.IssueOverage(ctx, snapshot)
		if invoiceErr != nil {
			s.logger.Error("invoice provider failed",
				zap.String("company_id", companyID),
				zap.String("snapshot_id", snapshot.ID),
				zap.String("provider", s.invoiceProvider.Name()),
				zap.Error(invoiceErr),
			)
			// Don't return error — snapshot is already persisted, invoice can be retried
		} else if result != nil && result.ExternalInvoiceID != "" {
			snapshot.ExternalInvoiceID = result.ExternalInvoiceID
			if repoErr := s.billingRepo.UpdateSnapshotInvoiceID(ctx, snapshot.ID, result.ExternalInvoiceID); repoErr != nil {
				s.logger.Error("failed to update snapshot invoice ID",
					zap.String("snapshot_id", snapshot.ID),
					zap.Error(repoErr),
				)
			}
		}
	}

	s.logger.Info("billing snapshot created",
		zap.String("company_id", companyID),
		zap.String("tier", string(plan.SubscriptionTier)),
		zap.Int64("total_cents", totalCents),
		zap.Int("consecutive_overage", consecutiveCount),
		zap.String("reason", string(reason)),
	)

	// Reset balances on EVERY snapshot because usage quotas are strictly monthly
	if err := s.resetBalances(ctx, companyID, meter, tierCfg); err != nil {
		return fmt.Errorf("reset balances: %w", err)
	}

	return nil
}

// resetBalances resets both Valkey counters and PostgreSQL balances to max limits.
func (s *BillingService) resetBalances(ctx context.Context, companyID string, meter *models.UsageMeter, tierCfg *config.TierLimits) error {
	// Reset Valkey counters
	ingressKey := fmt.Sprintf("plan:balance:ingress:%s", companyID)
	egressKey := fmt.Sprintf("plan:balance:egress:%s", companyID)

	if err := s.valkeyClient.Set(ctx, ingressKey, fmt.Sprintf("%d", tierCfg.BandwidthIngress), 0); err != nil {
		s.logger.Error("failed to reset valkey ingress balance", zap.Error(err))
	}
	if err := s.valkeyClient.Set(ctx, egressKey, fmt.Sprintf("%d", tierCfg.BandwidthEgress), 0); err != nil {
		s.logger.Error("failed to reset valkey egress balance", zap.Error(err))
	}

	// Invalidate the cached plan limits so next read refreshes
	cacheKey := fmt.Sprintf("plan:limits:%s", companyID)
	if err := s.valkeyClient.Delete(ctx, cacheKey); err != nil {
		s.logger.Error("failed to invalidate plan cache on cycle reset", zap.Error(err))
	}

	s.logger.Info("balances reset for new cycle",
		zap.String("company_id", companyID),
		zap.Int64("new_ingress", tierCfg.BandwidthIngress),
		zap.Int64("new_egress", tierCfg.BandwidthEgress),
	)
	return nil
}

// calculateOverageLineItems builds invoice line items for any meter that went negative.
func calculateOverageLineItems(meter *models.UsageMeter, tierCfg *config.TierLimits) []models.InvoiceLineItem {
	var items []models.InvoiceLineItem

	// Ingress overage
	if meter.BandwidthIngressBalance < 0 && tierCfg.BandwidthIngressOverageCentsPerMillion > 0 {
		overage := -meter.BandwidthIngressBalance // abs
		millions := overage / 1_000_000
		if overage%1_000_000 > 0 {
			millions++ // round up partial millions
		}
		items = append(items, models.InvoiceLineItem{
			Meter:       "bandwidth_ingress",
			OverageQty:  overage,
			UnitLabel:   "per 1M requests",
			RateCents:   tierCfg.BandwidthIngressOverageCentsPerMillion,
			AmountCents: millions * int64(tierCfg.BandwidthIngressOverageCentsPerMillion),
		})
	}

	// Egress overage
	if meter.BandwidthEgressBalance < 0 && tierCfg.BandwidthEgressOverageCentsPerGB > 0 {
		overage := -meter.BandwidthEgressBalance // abs (bytes)
		const gb = 1_073_741_824                 // 1 GB in bytes
		gbs := overage / gb
		if overage%gb > 0 {
			gbs++ // round up partial GBs
		}
		items = append(items, models.InvoiceLineItem{
			Meter:       "bandwidth_egress",
			OverageQty:  overage,
			UnitLabel:   "per GB",
			RateCents:   tierCfg.BandwidthEgressOverageCentsPerGB,
			AmountCents: gbs * int64(tierCfg.BandwidthEgressOverageCentsPerGB),
		})
	}

	return items
}

type NoOpInvoiceProvider struct {
	logger *zap.Logger
}

func NewNoOpInvoiceProvider(logger *zap.Logger) *NoOpInvoiceProvider {
	return &NoOpInvoiceProvider{logger: logger}
}

func (p *NoOpInvoiceProvider) Name() string { return "noop" }

func (p *NoOpInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	p.logger.Info("invoice generated (no-op)",
		zap.String("company_id", snapshot.CompanyID),
		zap.String("snapshot_id", snapshot.ID),
		zap.Int64("total_cents", snapshot.TotalCents),
		zap.Int("line_items", len(snapshot.LineItems)),
		zap.Int("consecutive_overage", snapshot.ConsecutiveOverageCount),
	)
	return &interfaces.InvoiceResult{
		ExternalInvoiceID: "",
		ProviderName:      "noop",
	}, nil
}

// StripeInvoiceProvider is a stub for Stripe integration.
// TODO: Implement actual Stripe API calls when ready.
type StripeInvoiceProvider struct {
	apiKey string
	logger *zap.Logger
}

func NewStripeInvoiceProvider(apiKey string, logger *zap.Logger) *StripeInvoiceProvider {
	return &StripeInvoiceProvider{apiKey: apiKey, logger: logger}
}

func (p *StripeInvoiceProvider) Name() string { return "stripe" }

func (p *StripeInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	p.logger.Warn("stripe invoice provider not yet implemented",
		zap.String("company_id", snapshot.CompanyID),
		zap.Int64("total_cents", snapshot.TotalCents),
	)
	return &interfaces.InvoiceResult{
		ExternalInvoiceID: "",
		ProviderName:      "stripe",
	}, nil
}

type PaystackInvoiceProvider struct {
	secretKey string
	logger    *zap.Logger
}

func NewPaystackInvoiceProvider(secretKey string, logger *zap.Logger) *PaystackInvoiceProvider {
	return &PaystackInvoiceProvider{secretKey: secretKey, logger: logger}
}

func (p *PaystackInvoiceProvider) Name() string { return "paystack" }

func (p *PaystackInvoiceProvider) IssueOverage(_ context.Context, snapshot *models.BillingSnapshot) (*interfaces.InvoiceResult, error) {
	p.logger.Warn("paystack invoice provider not yet implemented",
		zap.String("company_id", snapshot.CompanyID),
		zap.Int64("total_cents", snapshot.TotalCents),
	)
	return &interfaces.InvoiceResult{
		ExternalInvoiceID: "",
		ProviderName:      "paystack",
	}, nil
}
