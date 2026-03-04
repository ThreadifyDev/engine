package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

const (
	planCachePrefix = "plan:limits:"
	planCacheTTL    = 30 * time.Second

	// Valkey key prefixes for real-time balance tracking
	balanceKeyPrefix = "plan:balance:"
)

// balanceKey returns the Valkey key for a specific meter balance
func balanceKey(companyID, meter string) string {
	return balanceKeyPrefix + meter + ":" + companyID
}

type PlanService struct {
	planRepo      *postgres.PlanRepository
	subConfig     *config.SubscriptionConfig
	valkeyClient  interfaces.ValkeyClient
	natsPublisher *natsrepo.ArchivalPublisher
	logger        *zap.Logger
}

func NewPlanService(
	planRepo *postgres.PlanRepository,
	subConfig *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	logger *zap.Logger,
) *PlanService {
	return &PlanService{
		planRepo:     planRepo,
		subConfig:    subConfig,
		valkeyClient: valkeyClient,
		logger:       logger,
	}
}

// SetNATSPublisher sets the NATS publisher for async usage sync.
// Called after PlanService creation when NATS is available.
func (s *PlanService) SetNATSPublisher(publisher *natsrepo.ArchivalPublisher) {
	s.natsPublisher = publisher
}

func (s *PlanService) ProvisionSubscription(ctx context.Context, companyID string, tier models.PlanTier, billingCycle models.BillingCycle) error {
	limits := s.subConfig.GetTierLimits(string(tier))
	if limits == nil {
		return fmt.Errorf("unknown subscription tier: %s", tier)
	}

	now := time.Now().UTC()
	var billingEnd time.Time
	switch billingCycle {
	case models.BillingCycleMonthly:
		billingEnd = now.AddDate(0, 1, 0)
	case models.BillingCycleYearly:
		billingEnd = now.AddDate(1, 0, 0)
	default:
		return fmt.Errorf("unsupported billing cycle: %s", billingCycle)
	}

	plan := &models.CompanyPlan{
		ID:               uuid.New().String(),
		CompanyID:        companyID,
		SubscriptionTier: tier,
		BillingCycle:     billingCycle,
		BillingStart:     now,
		BillingEnd:       billingEnd,
	}

	if err := s.planRepo.CreatePlan(ctx, plan); err != nil {
		return fmt.Errorf("provision subscription - create plan: %w", err)
	}

	meter := &models.UsageMeter{
		ID:                      uuid.New().String(),
		CompanyID:               companyID,
		BillingCycleStart:       now,
		BandwidthIngressBalance: limits.BandwidthIngress,
		BandwidthEgressBalance:  limits.BandwidthEgress,
		LLMCreditsBalance:       limits.LLMCredits,
		MaxBandwidthIngress:     limits.BandwidthIngress,
		MaxBandwidthEgress:      limits.BandwidthEgress,
		MaxTeamSeats:            limits.TeamSeats,
		MaxContractLimit:        limits.ContractLimit,
		MaxRateLimit:            limits.RateLimit,
		MaxPayloadBytes:         limits.MaxPayloadBytes,
		MaxLLMCredits:           limits.LLMCredits,
		HotStorageDays:          limits.HotStorageDays,
		ColdStorageDays:         limits.ColdStorageDays,
		Support:                 limits.Support,
	}

	if err := s.planRepo.CreateUsageMeter(ctx, meter); err != nil {
		return fmt.Errorf("provision subscription - create usage meter: %w", err)
	}

	// Seed Valkey balance keys for real-time tracking
	s.seedBalanceKeys(ctx, companyID, meter)
	s.setCacheEntry(ctx, companyID, meter)

	s.logger.Info("provisioned subscription",
		zap.String("company_id", companyID),
		zap.String("tier", string(tier)),
		zap.String("billing_cycle", string(billingCycle)),
	)

	return nil
}

// seedBalanceKeys sets initial balance values in Valkey for real-time decrement tracking
func (s *PlanService) seedBalanceKeys(ctx context.Context, companyID string, meter *models.UsageMeter) {
	balances := map[string]int64{
		"ingress":     meter.BandwidthIngressBalance,
		"egress":      meter.BandwidthEgressBalance,
		"llm_credits": meter.LLMCreditsBalance,
	}

	for meterName, balance := range balances {
		key := balanceKey(companyID, meterName)
		if err := s.valkeyClient.Set(ctx, key, strconv.FormatInt(balance, 10), 0); err != nil {
			s.logger.Warn("failed to seed balance key",
				zap.String("key", key),
				zap.Error(err),
			)
		}
	}
}

func (s *PlanService) GetCompanyPlan(ctx context.Context, companyID string) (*models.CompanyPlan, error) {
	return s.planRepo.FindPlanByCompanyID(ctx, companyID)
}

func (s *PlanService) GetCurrentLimits(ctx context.Context, companyID string) (*models.UsageMeter, error) {
	cacheKey := planCachePrefix + companyID
	cached, err := s.valkeyClient.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		var meter models.UsageMeter
		if jsonErr := json.Unmarshal([]byte(cached), &meter); jsonErr == nil {
			return &meter, nil
		}
	}

	meter, err := s.planRepo.FindCurrentUsageMeter(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("get current limits: %w", err)
	}
	if meter == nil {
		return nil, nil
	}

	s.setCacheEntry(ctx, companyID, meter)
	return meter, nil
}

func (s *PlanService) GetRateLimit(ctx context.Context, companyID string) (int, error) {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return 0, err
	}
	if meter == nil {
		return 0, nil
	}
	return meter.MaxRateLimit, nil
}

func (s *PlanService) CheckPayloadSize(ctx context.Context, companyID string, payloadBytes int64) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("check payload size: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}
	if payloadBytes > meter.MaxPayloadBytes {
		return fmt.Errorf("payload size %d bytes exceeds limit of %d bytes. Please upgrade your plan", payloadBytes, meter.MaxPayloadBytes)
	}
	return nil
}

func (s *PlanService) CheckTeamSeatQuota(ctx context.Context, companyID string, currentCount int) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("check team seat quota: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}
	if meter.MaxTeamSeats == -1 {
		return nil
	}
	if currentCount >= meter.MaxTeamSeats {
		return fmt.Errorf("team seat limit reached (%d/%d). Please upgrade your plan", currentCount, meter.MaxTeamSeats)
	}
	return nil
}

func (s *PlanService) CheckContractQuota(ctx context.Context, companyID string, currentCount int) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("check contract quota: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}
	if meter.MaxContractLimit == -1 {
		return nil
	}
	if currentCount >= meter.MaxContractLimit {
		return fmt.Errorf("contract limit reached (%d/%d). Please upgrade your plan", currentCount, meter.MaxContractLimit)
	}
	return nil
}

// DecrementIngress decrements bandwidth ingress balance in Valkey.
// Starter: strict (hard cap — rolls back if balance goes negative).
// Growth: soft (allows negative balance for overage metering).
// Publishes async NATS event for DB sync via archiver.
func (s *PlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	plan, err := s.GetCompanyPlan(ctx, companyID)
	if err != nil {
		return fmt.Errorf("decrement ingress: %w", err)
	}
	if plan == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}

	key := balanceKey(companyID, "ingress")
	newBalance, err := s.valkeyClient.DecrBy(ctx, key, count)
	if err != nil {
		return fmt.Errorf("decrement ingress in valkey: %w", err)
	}

	tierLimits := s.subConfig.GetTierLimits(string(plan.SubscriptionTier))

	// Starter tier: hard cap — roll back if balance went negative
	if tierLimits != nil && !tierLimits.OverageAllowed && newBalance < 0 {
		// Roll back the decrement
		if _, incrErr := s.valkeyClient.IncrBy(ctx, key, count); incrErr != nil {
			s.logger.Error("failed to rollback ingress decrement", zap.Error(incrErr))
		}
		return fmt.Errorf("bandwidth ingress limit reached. Please upgrade your plan")
	}

	// Growth tier: log overage warning
	if newBalance < 0 {
		s.logger.Warn("bandwidth ingress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}

	// Publish async NATS event for DB sync
	s.publishUsageEvent(companyID, "bandwidth_ingress", count)

	return nil
}

// DecrementEgress decrements bandwidth egress balance in Valkey.
// Both tiers have egress overage pricing, so always soft decrement.
func (s *PlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	key := balanceKey(companyID, "egress")
	newBalance, err := s.valkeyClient.DecrBy(ctx, key, bytes)
	if err != nil {
		return fmt.Errorf("decrement egress in valkey: %w", err)
	}

	if newBalance < 0 {
		s.logger.Warn("bandwidth egress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}

	s.publishUsageEvent(companyID, "bandwidth_egress", bytes)
	return nil
}

func (s *PlanService) DecrementLLMCredits(ctx context.Context, companyID string, credits int64) error {
	key := balanceKey(companyID, "llm_credits")
	_, err := s.valkeyClient.DecrBy(ctx, key, credits)
	if err != nil {
		return fmt.Errorf("decrement llm credits in valkey: %w", err)
	}

	s.publishUsageEvent(companyID, "llm_credits", credits)
	return nil
}

// publishUsageEvent fires an async NATS event for the archiver to sync to PostgreSQL
func (s *PlanService) publishUsageEvent(companyID, meter string, amount int64) {
	if s.natsPublisher == nil {
		return
	}

	event := map[string]interface{}{
		"company_id": companyID,
		"meter":      meter,
		"amount":     amount,
	}

	s.natsPublisher.PublishAsync("usage.sync", event)
}

func (s *PlanService) InvalidatePlanCache(ctx context.Context, companyID string) {
	s.invalidateCache(ctx, companyID)
}

func (s *PlanService) setCacheEntry(ctx context.Context, companyID string, meter *models.UsageMeter) {
	data, err := json.Marshal(meter)
	if err != nil {
		s.logger.Warn("failed to marshal usage meter for cache", zap.Error(err))
		return
	}
	if setErr := s.valkeyClient.Set(ctx, planCachePrefix+companyID, string(data), planCacheTTL); setErr != nil {
		s.logger.Warn("failed to set plan cache in valkey", zap.Error(setErr))
	}
}

func (s *PlanService) invalidateCache(ctx context.Context, companyID string) {
	if delErr := s.valkeyClient.Delete(ctx, planCachePrefix+companyID); delErr != nil {
		s.logger.Warn("failed to invalidate plan cache in valkey", zap.Error(delErr))
	}
}
