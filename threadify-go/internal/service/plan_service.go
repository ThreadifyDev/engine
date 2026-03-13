package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"threadify-go/shared/billing"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/repository/postgres"
	"go.uber.org/zap"
)

var ErrSubscriptionExpired = fmt.Errorf("subscription expired")

func balanceKey(companyID, meter string) string {
	return database.BalanceKeyPrefix + meter + ":" + companyID
}

func usageContractKey(companyID string) string {
	return database.UsageContractKeyPrefix + companyID
}

func usageUserKey(companyID string) string {
	return database.UsageUserKeyPrefix + companyID
}

type PlanService struct {
	planRepo     *postgres.PlanRepository
	contractRepo *postgres.ContractRepository
	actorRepo    *postgres.ActorRepository
	subConfig    *config.SubscriptionConfig
	valkeyClient interfaces.ValkeyClient
	luaScripts   interfaces.LuaScriptManager
	logger       *zap.Logger
	cacheTTL     time.Duration
}

func NewPlanService(
	planRepo *postgres.PlanRepository,
	contractRepo *postgres.ContractRepository,
	actorRepo *postgres.ActorRepository,
	subConfig *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	luaScripts interfaces.LuaScriptManager,
	logger *zap.Logger,
	cacheTTLMs int,
) *PlanService {
	ttl := time.Duration(10000) * time.Millisecond
	if cacheTTLMs > 0 {
		ttl = time.Duration(cacheTTLMs) * time.Millisecond
	}

	return &PlanService{
		planRepo:     planRepo,
		contractRepo: contractRepo,
		actorRepo:    actorRepo,
		subConfig:    subConfig,
		valkeyClient: valkeyClient,
		luaScripts:   luaScripts,
		logger:       logger,
		cacheTTL:     ttl,
	}
}

func (s *PlanService) Start() error {
	s.logger.Info("plan service started")
	return nil
}

func (s *PlanService) Stop() error {
	s.logger.Info("plan service gracefully stopped")
	return nil
}

func (s *PlanService) ProvisionSubscription(ctx context.Context, companyID string, tier billing.PlanTier, billingCycle billing.BillingCycle, externalCustomerID, externalSubscriptionID string) error {
	limits := s.subConfig.GetTierLimits(string(tier))
	if limits == nil {
		return fmt.Errorf("unknown subscription tier: %s", tier)
	}

	now := time.Now().UTC()
	billingEnd, err := computeBillingEnd(now, billingCycle)
	if err != nil {
		return err
	}

	plan := &billing.CompanyPlan{
		ID:                     uuid.New().String(),
		CompanyID:              companyID,
		SubscriptionTier:       tier,
		BillingCycle:           billingCycle,
		ExternalCustomerID:     externalCustomerID,
		ExternalSubscriptionID: externalSubscriptionID,
		BillingStart:           now,
		BillingEnd:             billingEnd,
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

func (s *PlanService) seedBalanceKeys(ctx context.Context, companyID string, meter *billing.UsageMeter) error {
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

func (s *PlanService) GetCompanyPlan(ctx context.Context, companyID string) (*billing.CompanyPlan, error) {
	return s.planRepo.GetCompanyPlan(ctx, companyID)
}

func (s *PlanService) GetCurrentLimits(ctx context.Context, companyID string) (*billing.UsageMeter, error) {
	cacheKey := database.PlanCachePrefix + companyID
	cached, err := s.valkeyClient.Get(ctx, cacheKey)
	if err == nil && cached != "" {
		var meter billing.UsageMeter
		if jsonErr := json.Unmarshal([]byte(cached), &meter); jsonErr == nil {
			if !meter.BillingEnd.IsZero() && time.Now().After(meter.BillingEnd) {
				s.invalidateCache(ctx, companyID)
				return nil, ErrSubscriptionExpired
			}
			return &meter, nil
		}
	}

	meter, err := s.planRepo.GetCurrentUsageMeter(ctx, companyID)
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
	if _, err := s.valkeyClient.IncrBy(ctx, key, 1); err != nil {
		s.logger.Error("failed to increment user count in valkey (fail-open)", zap.String("company_id", companyID), zap.Error(err))
	}
}

func (s *PlanService) DecrementUserCount(ctx context.Context, companyID string) {
	key := usageUserKey(companyID)
	_, allowed, err := s.luaScripts.DecrementUsage(ctx, key, 1, 0)
	if err != nil {
		s.logger.Error("failed to decrement user count in valkey (fail-open)", zap.String("company_id", companyID), zap.Error(err))
		return
	}
	if allowed == -1 {
		if setErr := s.valkeyClient.Set(ctx, key, "0", 0); setErr != nil {
			s.logger.Error("failed to seed missing user count key in valkey", zap.String("company_id", companyID), zap.Error(setErr))
		}
	}
}

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
	_, allowed, err := s.luaScripts.DecrementUsage(ctx, key, 1, 0)
	if err != nil {
		s.logger.Error("failed to release contract slot in valkey", zap.String("company_id", companyID), zap.Error(err))
		return
	}

	if allowed == -1 {
		dbCount, dbErr := s.contractRepo.CountByCompany(ctx, companyID)
		if dbErr != nil {
			s.logger.Error("failed to seed missing contract usage key from database", zap.String("company_id", companyID), zap.Error(dbErr))
			return
		}
		corrected := dbCount - 1
		if corrected < 0 {
			corrected = 0
		}
		if setErr := s.valkeyClient.Set(ctx, key, strconv.Itoa(corrected), 0); setErr != nil {
			s.logger.Error("failed to set contract usage key in valkey", zap.String("company_id", companyID), zap.Error(setErr))
		}
	}
}

func (s *PlanService) CheckIngressQuota(ctx context.Context, companyID string, count int64) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("check ingress: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}

	tierLimits := s.subConfig.GetTierLimits(string(meter.SubscriptionTier))
	if tierLimits == nil || tierLimits.OverageAllowed {
		return nil
	}

	key := balanceKey(companyID, "ingress")
	val, err := s.valkeyClient.Get(ctx, key)
	if err != nil {
		s.logger.Warn("check ingress balance failed in valkey (fail-open)", zap.Error(err))
		return nil
	}

	if val == "" {
		return nil
	}

	balance, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		s.logger.Warn("failed to parse ingress balance (fail-open)", zap.Error(err))
		return nil
	}

	hardCapFloor := tierLimits.BandwidthIngress - tierLimits.BandwidthIngressHardCap
	if hardCapFloor < 0 {
		hardCapFloor = 0
	}
	if balance-count < hardCapFloor {
		return fmt.Errorf("bandwidth ingress hard limit reached. Please upgrade your plan")
	}

	return nil
}

func (s *PlanService) writeUsageToOutbox(
	ctx context.Context,
	companyID, meter string,
	amount int64,
	billingCycleStart time.Time,
) {
	eventData := map[string]interface{}{
		fieldEventID:           uuid.NewString(),
		fieldCompanyID:         companyID,
		fieldMeter:             meter,
		fieldAmount:            strconv.FormatInt(amount, 10),
		fieldBillingCycleStart: billingCycleStart.Format(time.RFC3339),
		fieldTimestamp:         time.Now().UTC().Format(time.RFC3339),
	}

	if _, err := s.valkeyClient.XAdd(ctx, database.UsageOutboxStreamKey, "*", eventData); err != nil {
		s.logger.Error("failed to write usage event to outbox (fail-open)",
			zap.String("company_id", companyID),
			zap.String("meter", meter),
			zap.Int64("amount", amount),
			zap.Error(err),
		)
	}
}

func (s *PlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("decrement ingress: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}

	tierLimits := s.subConfig.GetTierLimits(string(meter.SubscriptionTier))
	if tierLimits != nil && tierLimits.OverageAllowed {
		s.writeUsageToOutbox(ctx, companyID, "bandwidth_ingress", count, meter.BillingCycleStart)
		return nil
	}

	floor := int64(-999999999999)
	if tierLimits != nil {
		floor = tierLimits.BandwidthIngress - tierLimits.BandwidthIngressHardCap
	}

	key := balanceKey(companyID, "ingress")
	newBalance, allowed, err := s.decrementUsageWithOutbox(ctx, key, count, floor, companyID, "bandwidth_ingress", meter.BillingCycleStart)
	if err != nil {
		s.logger.Error("failed to decrement ingress in valkey (fail-open)", zap.Error(err))
		return nil
	}

	if allowed == -1 {
		s.logger.Info("ingress balance key missing in valkey, seeding from DB", zap.String("company_id", companyID))
		dbMeter, dbErr := s.planRepo.GetCurrentUsageMeter(ctx, companyID)
		if dbErr == nil && dbMeter != nil {
			s.valkeyClient.Set(ctx, key, strconv.FormatInt(dbMeter.BandwidthIngressBalance, 10), 0)
			newBalance, allowed, err = s.decrementUsageWithOutbox(ctx, key, count, floor, companyID, "bandwidth_ingress", meter.BillingCycleStart)
			if err != nil {
				s.logger.Error("failed ingress decrement retry in valkey (fail-open)", zap.Error(err))
				return nil
			}
		} else {
			s.logger.Error("failed to seed missing ingress balance key from database (fail-open)",
				zap.String("company_id", companyID),
				zap.Error(dbErr),
			)
			return nil
		}
	}

	if allowed == 0 {
		return fmt.Errorf("bandwidth ingress hard limit reached. Please upgrade your plan")
	}

	if newBalance < 0 {
		s.logger.Warn("bandwidth ingress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}

	return nil
}

func (s *PlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	meter, err := s.GetCurrentLimits(ctx, companyID)
	if err != nil {
		return fmt.Errorf("decrement egress: %w", err)
	}
	if meter == nil {
		return fmt.Errorf("no active subscription for company %s", companyID)
	}

	tierLimits := s.subConfig.GetTierLimits(string(meter.SubscriptionTier))
	if tierLimits != nil && tierLimits.OverageAllowed {
		s.writeUsageToOutbox(ctx, companyID, "bandwidth_egress", bytes, meter.BillingCycleStart)
		return nil
	}

	key := balanceKey(companyID, "egress")
	newBalance, allowed, err := s.decrementUsageWithOutbox(ctx, key, bytes, -999999999999, companyID, "bandwidth_egress", meter.BillingCycleStart)
	if err != nil {
		s.logger.Error("failed to decrement egress in valkey (fail-open)", zap.Error(err))
		return nil
	}

	if allowed == -1 {
		s.logger.Info("egress balance key missing in valkey, seeding from DB", zap.String("company_id", companyID))
		dbMeter, dbErr := s.planRepo.GetCurrentUsageMeter(ctx, companyID)
		if dbErr == nil && dbMeter != nil {
			s.valkeyClient.Set(ctx, key, strconv.FormatInt(dbMeter.BandwidthEgressBalance, 10), 0)
			newBalance, _, err = s.decrementUsageWithOutbox(ctx, key, bytes, -999999999999, companyID, "bandwidth_egress", meter.BillingCycleStart)
			if err != nil {
				s.logger.Error("failed egress decrement retry in valkey (fail-open)", zap.Error(err))
				return nil
			}
		} else {
			s.logger.Error("failed to seed missing egress balance key from database (fail-open)",
				zap.String("company_id", companyID),
				zap.Error(dbErr),
			)
			return nil
		}
	}

	if newBalance < 0 {
		s.logger.Warn("bandwidth egress overage",
			zap.String("company_id", companyID),
			zap.Int64("balance", newBalance),
		)
	}
	return nil
}

func (s *PlanService) decrementUsageWithOutbox(
	ctx context.Context,
	balanceKey string,
	amount int64,
	floor int64,
	companyID, meter string,
	billingCycleStart time.Time,
) (int64, int64, error) {
	eventID := uuid.NewString()

	newBalance, allowed, _, err := s.luaScripts.DecrementUsageWithOutbox(
		ctx,
		balanceKey,
		database.UsageOutboxStreamKey,
		amount,
		floor,
		eventID,
		companyID,
		meter,
		billingCycleStart,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("decrement usage with outbox: %w", err)
	}

	return newBalance, allowed, nil
}

func (s *PlanService) RenewSubscriptionFromBillingEnd(ctx context.Context, companyID string, tier billing.PlanTier, billingCycle billing.BillingCycle, previousBillingEnd time.Time, externalCustomerID, externalSubscriptionID string) error {
	limits := s.subConfig.GetTierLimits(string(tier))
	if limits == nil {
		return fmt.Errorf("unknown subscription tier: %s", tier)
	}

	newStart := previousBillingEnd
	billingEnd, err := computeBillingEnd(newStart, billingCycle)
	if err != nil {
		return err
	}

	meter := newMeterFromLimits(companyID, tier, newStart, billingEnd, limits)

	renewParams := postgres.RenewUsageMeterParams{
		UpdatePlanParams: postgres.UpdatePlanParams{
			CompanyID:      companyID,
			Tier:           tier,
			BillingCycle:   billingCycle,
			ExternalCustID: externalCustomerID,
			ExternalSubID:  externalSubscriptionID,
			BillingStart:   newStart,
			BillingEnd:     billingEnd,
		},
		Meter: meter,
	}

	if err := s.planRepo.RenewUsageMeter(ctx, renewParams); err != nil {
		return fmt.Errorf("renew subscription: %w", err)
	}

	if err := s.seedBalanceKeys(ctx, companyID, meter); err != nil {
		return fmt.Errorf("renew subscription - seed balances: %w", err)
	}
	s.invalidateCache(ctx, companyID)

	s.logger.Info("renewed subscription",
		zap.String("company_id", companyID),
		zap.String("tier", string(tier)),
		zap.String("billing_cycle", string(billingCycle)),
		zap.Time("new_start", newStart),
		zap.Time("billing_end", billingEnd),
	)

	return nil
}

func computeBillingEnd(start time.Time, cycle billing.BillingCycle) (time.Time, error) {
	switch cycle {
	case billing.BillingCycleMonthly:
		return start.AddDate(0, 1, 0), nil
	case billing.BillingCycleYearly:
		return start.AddDate(1, 0, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported billing cycle: %s", cycle)
	}
}

func newMeterFromLimits(companyID string, tier billing.PlanTier, start, billingEnd time.Time, limits *config.TierLimits) *billing.UsageMeter {
	return &billing.UsageMeter{
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

func (s *PlanService) setCacheEntry(ctx context.Context, companyID string, meter *billing.UsageMeter) {
	data, err := json.Marshal(meter)
	if err != nil {
		s.logger.Warn("failed to marshal usage meter for cache", zap.Error(err))
		return
	}
	jitterRange := int64(s.cacheTTL / 5)
	jitter := time.Duration(rand.Int63n(jitterRange)) - (time.Duration(jitterRange) / 2)
	finalTTL := s.cacheTTL + jitter

	if !meter.BillingEnd.IsZero() {
		timeToExpiry := time.Until(meter.BillingEnd)
		if timeToExpiry < finalTTL {
			if timeToExpiry <= 0 {
				return
			}
			finalTTL = timeToExpiry
		}
	}

	if setErr := s.valkeyClient.Set(ctx, database.PlanCachePrefix+companyID, string(data), finalTTL); setErr != nil {
		s.logger.Warn("failed to set plan cache in valkey", zap.Error(setErr))
	}
}

func (s *PlanService) invalidateCache(ctx context.Context, companyID string) {
	if delErr := s.valkeyClient.Delete(ctx, database.PlanCachePrefix+companyID); delErr != nil {
		s.logger.Warn("failed to invalidate plan cache in valkey", zap.Error(delErr))
	}
}
