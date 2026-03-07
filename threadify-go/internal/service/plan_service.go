package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
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

	// Valkey key prefixes for real-time balance tracking
	balanceKeyPrefix       = "plan:balance:"
	usageContractKeyPrefix = "plan:usage:contracts:"
	usageUserKeyPrefix     = "plan:usage:users:"
)

var ErrSubscriptionExpired = fmt.Errorf("subscription expired")

// balanceKey returns the Valkey key for a specific meter balance
func balanceKey(companyID, meter string) string {
	return balanceKeyPrefix + meter + ":" + companyID
}

func usageContractKey(companyID string) string {
	return usageContractKeyPrefix + companyID
}

func usageUserKey(companyID string) string {
	return usageUserKeyPrefix + companyID
}

type PlanService struct {
	planRepo      *postgres.PlanRepository
	contractRepo  *postgres.ContractRepository
	actorRepo     *postgres.ActorRepository
	subConfig     *config.SubscriptionConfig
	valkeyClient  interfaces.ValkeyClient
	luaScripts    interfaces.LuaScriptManager
	natsPublisher *natsrepo.ArchivalPublisher
	logger        *zap.Logger
	cacheTTL      time.Duration
}

func NewPlanService(
	planRepo *postgres.PlanRepository,
	contractRepo *postgres.ContractRepository,
	actorRepo *postgres.ActorRepository,
	subConfig *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	luaScripts interfaces.LuaScriptManager,
	natsPublisher *natsrepo.ArchivalPublisher,
	logger *zap.Logger,
	cacheTTLMs int,
) *PlanService {
	ttl := time.Duration(10000) * time.Millisecond
	if cacheTTLMs > 0 {
		ttl = time.Duration(cacheTTLMs) * time.Millisecond
	}

	return &PlanService{
		planRepo:      planRepo,
		contractRepo:  contractRepo,
		actorRepo:     actorRepo,
		subConfig:     subConfig,
		valkeyClient:  valkeyClient,
		luaScripts:    luaScripts,
		natsPublisher: natsPublisher,
		logger:        logger,
		cacheTTL:      ttl,
	}
}

func (s *PlanService) ProvisionSubscription(ctx context.Context, companyID string, tier models.PlanTier, billingCycle models.BillingCycle) error {
	limits := s.subConfig.GetTierLimits(string(tier))
	if limits == nil {
		return fmt.Errorf("unknown subscription tier: %s", tier)
	}

	now := time.Now().UTC()
	billingEnd, err := computeBillingEnd(now, billingCycle)
	if err != nil {
		return err
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

	meter := newMeterFromLimits(companyID, tier, now, billingEnd, limits)

	if err := s.planRepo.CreateUsageMeter(ctx, meter); err != nil {
		return fmt.Errorf("provision subscription - create usage meter: %w", err)
	}

	if err := s.seedBalanceKeys(ctx, companyID, meter); err != nil {
		return fmt.Errorf("provision subscription - seed balances: %w", err)
	}
	s.setCacheEntry(ctx, companyID, meter)

	s.logger.Info("provisioned subscription",
		zap.String("company_id", companyID),
		zap.String("tier", string(tier)),
		zap.String("billing_cycle", string(billingCycle)),
	)

	return nil
}

func (s *PlanService) seedBalanceKeys(ctx context.Context, companyID string, meter *models.UsageMeter) error {
	ingressKey := balanceKey(companyID, "ingress")
	if err := s.valkeyClient.Set(ctx, ingressKey, strconv.FormatInt(meter.BandwidthIngressBalance, 10), 0); err != nil {
		return fmt.Errorf("seed ingress balance: %w", err)
	}
	egressKey := balanceKey(companyID, "egress")
	if err := s.valkeyClient.Set(ctx, egressKey, strconv.FormatInt(meter.BandwidthEgressBalance, 10), 0); err != nil {
		return fmt.Errorf("seed egress balance: %w", err)
	}
	return nil
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
			if !meter.BillingEnd.IsZero() && time.Now().After(meter.BillingEnd) {
				s.invalidateCache(ctx, companyID)
				return nil, ErrSubscriptionExpired
			}
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

	if !meter.BillingEnd.IsZero() && time.Now().After(meter.BillingEnd) {
		return nil, ErrSubscriptionExpired
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

func (s *PlanService) IncrementUserCount(ctx context.Context, companyID string) {
	key := usageUserKey(companyID)
	if _, _, err := s.luaScripts.CheckAndIncrQuota(ctx, key, -1, 1); err != nil {
		s.logger.Error("failed to increment user count in valkey (fail-open)", zap.String("company_id", companyID), zap.Error(err))
	}
}

func (s *PlanService) DecrementUserCount(ctx context.Context, companyID string) {
	key := usageUserKey(companyID)
	if _, err := s.valkeyClient.DecrBy(ctx, key, 1); err != nil {
		s.logger.Error("failed to decrement user count in valkey (fail-open)", zap.String("company_id", companyID), zap.Error(err))
	}
}

// Removed CheckContractQuota entirely to prevent split-call pattern in public API.

func (s *PlanService) ClaimContractSlot(ctx context.Context, companyID string) (int, error) {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return 0, err
	}

	if meter.MaxContractLimit == -1 {
		count, _, err := s.luaScripts.CheckAndIncrQuota(ctx, usageContractKey(companyID), -1, 1)
		return int(count), err
	}

	key := usageContractKey(companyID)
	count, allowed, err := s.luaScripts.CheckAndIncrQuota(ctx, key, meter.MaxContractLimit, 1)
	if err != nil {
		return 0, fmt.Errorf("valkey error during slot claim: %w", err)
	}

	if allowed == -1 {
		dbCount, dbErr := s.contractRepo.CountByCompany(ctx, companyID)
		if dbErr != nil {
			return 0, fmt.Errorf("seed failed: %w", dbErr)
		}
		s.valkeyClient.Set(ctx, key, strconv.Itoa(dbCount), 0)
		count, allowed, err = s.luaScripts.CheckAndIncrQuota(ctx, key, meter.MaxContractLimit, 1)
		if err != nil {
			return 0, err
		}
		if allowed == -1 {
			return 0, fmt.Errorf("contract claim failed: valkey unrecoverable missing key")
		}
	}

	if allowed == 0 {
		return int(count), fmt.Errorf("contract limit reached (%d/%d)", count, meter.MaxContractLimit)
	}

	return int(count), nil
}

func (s *PlanService) ReleaseContractSlot(ctx context.Context, companyID string) {
	key := usageContractKey(companyID)
	if _, err := s.valkeyClient.DecrBy(ctx, key, 1); err != nil {
		s.logger.Error("failed to release contract slot in valkey", zap.String("company_id", companyID), zap.Error(err))
	}
}

func (s *PlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("decrement ingress: %w", err)
	}

	// Use tier config for floor calculation
	floor := int64(-999999999999)
	tierLimits := s.subConfig.GetTierLimits(string(meter.SubscriptionTier))
	if tierLimits != nil && !tierLimits.OverageAllowed {
		floor = tierLimits.BandwidthIngress - tierLimits.BandwidthIngressHardCap
	}

	key := balanceKey(companyID, "ingress")
	newBalance, allowed, err := s.luaScripts.DecrementUsage(ctx, key, count, floor)
	if err != nil {
		s.logger.Error("failed to decrement ingress in valkey (fail-open)", zap.Error(err))
		// Fail open: publish usage event and allow the request to proceed
		s.publishUsageEvent(ctx, companyID, "bandwidth_ingress", count)
		return nil
	}

	if allowed == -1 {
		// Key missing, seed and retry one more time
		s.logger.Info("ingress balance key missing in valkey, seeding from DB", zap.String("company_id", companyID))
		dbMeter, dbErr := s.planRepo.FindCurrentUsageMeter(ctx, companyID)
		if dbErr == nil && dbMeter != nil {
			s.valkeyClient.Set(ctx, key, strconv.FormatInt(dbMeter.BandwidthIngressBalance, 10), 0)
			// Atomic retry
			newBalance, allowed, err = s.luaScripts.DecrementUsage(ctx, key, count, floor)
			if err != nil {
				s.publishUsageEvent(ctx, companyID, "bandwidth_ingress", count)
				return nil
			}
			// Fall through to quota check after retry
		} else {
			// Seeding failed, publish and allow (extreme fail-open)
			s.publishUsageEvent(ctx, companyID, "bandwidth_ingress", count)
			return nil
		}
	}

	if allowed == 0 {
		return fmt.Errorf("bandwidth ingress hard limit reached. Please upgrade your plan")
	}

	// Growth tier: log overage warning
	if newBalance < 0 {
		s.logger.Warn("bandwidth ingress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}

	s.publishUsageEvent(ctx, companyID, "bandwidth_ingress", count)

	return nil
}

// DecrementEgress decrements bandwidth egress balance in Valkey.
func (s *PlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	key := balanceKey(companyID, "egress")
	// Always soft capped, floor is -Infinity (proxied by a very low number)
	newBalance, allowed, err := s.luaScripts.DecrementUsage(ctx, key, bytes, -999999999999)
	if err != nil {
		s.logger.Error("failed to decrement egress in valkey (fail-open)", zap.Error(err))
		// Fail open
		s.publishUsageEvent(ctx, companyID, "bandwidth_egress", bytes)
		return nil
	}

	if allowed == -1 {
		// Key missing, seed and retry once
		s.logger.Info("egress balance key missing in valkey, seeding from DB", zap.String("company_id", companyID))
		dbMeter, dbErr := s.planRepo.FindCurrentUsageMeter(ctx, companyID)
		if dbErr == nil && dbMeter != nil {
			s.valkeyClient.Set(ctx, key, strconv.FormatInt(dbMeter.BandwidthEgressBalance, 10), 0)
			newBalance, allowed, err = s.luaScripts.DecrementUsage(ctx, key, bytes, -999999999999)
			if err != nil {
				s.publishUsageEvent(ctx, companyID, "bandwidth_egress", bytes)
				return nil
			}
			// Fall through to overage warning after retry
		} else {
			s.publishUsageEvent(ctx, companyID, "bandwidth_egress", bytes)
			return nil
		}
	}

	if newBalance < 0 {
		s.logger.Warn("bandwidth egress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}

	s.publishUsageEvent(ctx, companyID, "bandwidth_egress", bytes)
	return nil
}

func (s *PlanService) publishUsageEvent(ctx context.Context, companyID, meter string, amount int64) {
	_ = ctx // Accepted for future transactional outbox pattern
	event := map[string]interface{}{
		"company_id": companyID,
		"meter":      meter,
		"amount":     amount,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	s.logger.Debug("publishing usage event",
		zap.String("company_id", companyID),
		zap.String("meter", meter),
		zap.Int64("amount", amount),
	)

	s.natsPublisher.PublishAsync("usage.sync", event)
}

func (s *PlanService) RenewSubscription(ctx context.Context, companyID string, tier models.PlanTier, billingCycle models.BillingCycle) error {
	limits := s.subConfig.GetTierLimits(string(tier))
	if limits == nil {
		return fmt.Errorf("unknown subscription tier: %s", tier)
	}

	now := time.Now().UTC()
	billingEnd, err := computeBillingEnd(now, billingCycle)
	if err != nil {
		return err
	}

	if err := s.planRepo.UpdatePlanTier(ctx, companyID, tier, billingCycle, now, billingEnd); err != nil {
		return fmt.Errorf("renew subscription - update plan: %w", err)
	}

	meter := newMeterFromLimits(companyID, tier, now, billingEnd, limits)

	if err := s.planRepo.CreateUsageMeter(ctx, meter); err != nil {
		return fmt.Errorf("renew subscription - create usage meter: %w", err)
	}

	if err := s.seedBalanceKeys(ctx, companyID, meter); err != nil {
		return fmt.Errorf("renew subscription - seed balances: %w", err)
	}
	s.invalidateCache(ctx, companyID)

	s.logger.Info("renewed subscription",
		zap.String("company_id", companyID),
		zap.String("tier", string(tier)),
		zap.String("billing_cycle", string(billingCycle)),
	)

	return nil
}

func computeBillingEnd(start time.Time, cycle models.BillingCycle) (time.Time, error) {
	switch cycle {
	case models.BillingCycleMonthly:
		return start.AddDate(0, 1, 0), nil
	case models.BillingCycleYearly:
		return start.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported billing cycle: %s", cycle)
	}
}

func newMeterFromLimits(companyID string, tier models.PlanTier, start, billingEnd time.Time, limits *config.TierLimits) *models.UsageMeter {
	return &models.UsageMeter{
		ID:                      uuid.New().String(),
		CompanyID:               companyID,
		SubscriptionTier:        tier,
		BillingCycleStart:       start,
		BandwidthIngressBalance: limits.BandwidthIngress,
		BandwidthEgressBalance:  limits.BandwidthEgress,
		MaxBandwidthIngress:     limits.BandwidthIngress,
		MaxBandwidthEgress:      limits.BandwidthEgress,
		MaxTeamSeats:            limits.TeamSeats,
		MaxContractLimit:        limits.ContractLimit,
		MaxRateLimit:            limits.RateLimit,
		MaxPayloadBytes:         limits.MaxPayloadBytes,
		HotStorageDays:          limits.HotStorageDays,
		ColdStorageDays:         limits.ColdStorageDays,
		Support:                 limits.Support,
		BillingEnd:              billingEnd,
	}
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
	// Add jitter (±10% of TTL) to prevent thundering herd
	jitterRange := int64(s.cacheTTL / 5)
	jitter := time.Duration(rand.Int63n(jitterRange)) - (time.Duration(jitterRange) / 2)
	finalTTL := s.cacheTTL + jitter

	if !meter.BillingEnd.IsZero() {
		timeToExpiry := time.Until(meter.BillingEnd)
		if timeToExpiry < finalTTL {
			if timeToExpiry <= 0 {
				return // Don't cache expired stuff
			}
			finalTTL = timeToExpiry
		}
	}

	if setErr := s.valkeyClient.Set(ctx, planCachePrefix+companyID, string(data), finalTTL); setErr != nil {
		s.logger.Warn("failed to set plan cache in valkey", zap.Error(setErr))
	}
}

func (s *PlanService) invalidateCache(ctx context.Context, companyID string) {
	if delErr := s.valkeyClient.Delete(ctx, planCachePrefix+companyID); delErr != nil {
		s.logger.Warn("failed to invalidate plan cache in valkey", zap.Error(delErr))
	}
}
