package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"threadify-go/shared/billing"
	"threadify-go/shared/database"
	serror "threadify-go/shared/errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

var ErrInsufficientCredit = fmt.Errorf("insufficient credit balance")

var ErrNoAccount = fmt.Errorf("no active credit account")

const (
	defaultCacheTTLMs = 10_000 * time.Millisecond

	minBalanceFraction           = 5
	rateLimitWindowSeconds       = 1
	debitDenied            int64 = 0
	debitTopupHit          int64 = 1

	rolloverLockTTL = 15 * time.Minute

	cacheTTLJitterFraction   = 5
	cacheTTLJitterMultiplier = 2

	minDebitFloorMillicents int64 = 1
)

type creditSeed struct {
	BalanceMillicents        int64
	MinBalanceMillicents     int64
	AutoTopupMillicents      int64
	MonthlyChargedMillicents int64
	RateLimitTPS             int64
	PayloadLimitBytes        int64
}

type CachedPlan struct {
	Account *billing.CreditAccount `json:"account"`
}

func newCreditAccount(companyID string, start time.Time, creditCfg *config.CreditConfig, seed *creditSeed) *billing.CreditAccount {
	account := &billing.CreditAccount{
		ID:                uuid.New().String(),
		CompanyID:         companyID,
		BillingCycleStart: start,
	}

	if seed != nil {
		account.CreditBalanceMillicents = seed.BalanceMillicents
		account.CreditMinBalanceMillicents = seed.MinBalanceMillicents
		account.CreditAutoTopupMillicents = seed.AutoTopupMillicents
		account.CreditMonthlyChargedMillicents = seed.MonthlyChargedMillicents
		account.RateLimitTPS = seed.RateLimitTPS
		account.PayloadLimitBytes = seed.PayloadLimitBytes
	}

	return account
}

func nextBillingCycle(cycleStart, now time.Time) (time.Time, bool) {
	next := cycleStart.AddDate(0, 1, 0)
	if now.Before(next) {
		return time.Time{}, false
	}
	for !now.Before(next.AddDate(0, 1, 0)) {
		next = next.AddDate(0, 1, 0)
	}
	return next, true
}

type PlanService struct {
	planRepo     interfaces.PlanRepository
	contractRepo interfaces.ContractRepository
	actorRepo    interfaces.ActorRepository
	subConfig    *config.SubscriptionConfig
	valkeyClient interfaces.ValkeyClient
	luaScripts   interfaces.LuaScriptManager
	logger       *zap.Logger
	cacheTTL     time.Duration
	sfGroup      singleflight.Group
}

func NewPlanService(
	planRepo interfaces.PlanRepository,
	contractRepo interfaces.ContractRepository,
	actorRepo interfaces.ActorRepository,
	subConfig *config.SubscriptionConfig,
	valkeyClient interfaces.ValkeyClient,
	luaScripts interfaces.LuaScriptManager,
	logger *zap.Logger,
	cacheTTLMs int,
) *PlanService {
	ttl := defaultCacheTTLMs
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

func (s *PlanService) ProvisionSubscription(
	ctx context.Context,
	companyID, externalCustomerID string,
	initialAmount int64,
) error {
	s.logger.Debug("Provisioning subscription",
		zap.String("company_id", companyID),
		zap.String("external_customer_id", externalCustomerID),
		zap.Int64("initial_amount", initialAmount),
	)

	existingAccount, err := s.planRepo.GetCreditAccount(ctx, companyID)
	if err != nil && !errors.Is(err, ErrNoAccount) {
		return fmt.Errorf("provision subscription - get credit account: %w", err)
	}

	if existingAccount != nil {
		if err := s.planRepo.SetExternalCustomerID(ctx, companyID, externalCustomerID); err != nil {
			s.logger.Warn("failed to update external customer id on topup", zap.Error(err))
		}

		keys := billing.KeysFor(companyID)

		_, getErr := s.valkeyClient.Get(ctx, keys.Balance)
		if getErr != nil {
			if !errors.Is(getErr, redis.Nil) {
				return fmt.Errorf("manual topup - checking valkey balance: %w", getErr)
			}
			s.logger.Warn("valkey cache miss during manual topup, seeding from db",
				zap.String("company_id", companyID),
				zap.Int64("db_balance", existingAccount.CreditBalanceMillicents),
			)
			if seedErr := s.seedCreditKeys(ctx, companyID, existingAccount); seedErr != nil {
				return fmt.Errorf("manual topup - seed valkey: %w", seedErr)
			}
		}

		newBalance, err := s.valkeyClient.ApplyCreditTopupAtomic(ctx, keys.Balance, keys.Pending, initialAmount)
		if err != nil {
			return fmt.Errorf("manual topup - apply credit atomic: %w", err)
		}

		s.writeUsageToOutbox(ctx, companyID, MeterCreditTopup, initialAmount, existingAccount.BillingCycleStart)

		minBalance := newBalance / minBalanceFraction

		// Manual topup should NOT enable auto-topup - only update min_balance
		// Auto-topup should only be enabled via explicit user configuration
		if err := s.planRepo.UpdateTopupSettings(ctx, companyID, 0, minBalance); err != nil {
			s.logger.Warn("failed to update min balance on manual topup", zap.Error(err))
		}

		s.logger.Info("processed manual topup for existing subscription",
			zap.String("company_id", companyID),
			zap.Int64("added_amount", initialAmount),
			zap.Int64("new_balance", newBalance),
			zap.Int64("new_min_balance", minBalance),
			zap.Int64("max_monthly", existingAccount.CreditMaxMonthlyChargeMillicents),
		)

		s.InvalidatePlanCache(ctx, companyID)
		return nil
	}

	now := time.Now().UTC()

	if err := s.planRepo.SetExternalCustomerID(ctx, companyID, externalCustomerID); err != nil {
		return fmt.Errorf("provision subscription - set external customer id: %w", err)
	}

	creditCfg := &s.subConfig.Credit
	minBalance := initialAmount / minBalanceFraction

	seed := &creditSeed{
		BalanceMillicents:    initialAmount,
		AutoTopupMillicents:  0, // Manual topup should NOT enable auto-topup
		MinBalanceMillicents: minBalance,
		RateLimitTPS:         creditCfg.RateLimitTPS,
		PayloadLimitBytes:    creditCfg.PayloadLimitBytes,
	}
	account := newCreditAccount(companyID, now, creditCfg, seed)

	if err := s.planRepo.CreateCreditAccount(ctx, account); err != nil {
		return fmt.Errorf("provision subscription - create credit account: %w", err)
	}

	if err := s.seedCreditKeys(ctx, companyID, account); err != nil {
		s.logger.Warn("failed to seed valkey keys on provision (will lazy-seed on first debit)",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
	}

	s.logger.Info("provisioned subscription",
		zap.String("company_id", companyID),
		zap.Time("billing_start", now),
	)

	return nil
}

func (s *PlanService) seedCreditKeys(ctx context.Context, companyID string, account *billing.CreditAccount) error {
	keys := billing.KeysFor(companyID)

	s.logger.Debug("seeding credit keys in valkey",
		zap.String("company_id", companyID),
		zap.Int64("balance", account.CreditBalanceMillicents),
		zap.Int64("charged", account.CreditMonthlyChargedMillicents),
	)

	pipe := s.valkeyClient.Pipeline()
	pipe.Set(ctx, keys.Balance, strconv.FormatInt(account.CreditBalanceMillicents, 10), 0)
	pipe.Set(ctx, keys.Charged, strconv.FormatInt(account.CreditMonthlyChargedMillicents, 10), 0)
	pipe.Del(ctx, keys.Pending)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("seed credit keys: %w", err)
	}
	return nil
}

func (s *PlanService) GetExternalCustomerID(ctx context.Context, companyID string) (string, error) {
	return s.planRepo.GetExternalCustomerID(ctx, companyID)
}

func (s *PlanService) getCurrentAccount(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	v, err, _ := s.sfGroup.Do(companyID, func() (interface{}, error) {
		if cached := s.getCacheEntry(ctx, companyID); cached != nil {
			return cached.Account, nil
		}

		account, err := s.planRepo.GetCreditAccount(ctx, companyID)
		if err != nil {
			return nil, fmt.Errorf("get credit account: %w", err)
		}
		if account == nil {
			return nil, ErrNoAccount
		}

		s.setCacheEntry(ctx, companyID, &CachedPlan{Account: account})
		return account, nil
	})

	if err != nil {
		return nil, err
	}
	return v.(*billing.CreditAccount), nil
}

func (s *PlanService) GetCurrentLimits(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	return s.getCurrentAccount(ctx, companyID)
}

func (s *PlanService) CheckBalancePositive(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	account, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return nil, err
	}

	state, err := s.readCreditState(ctx, companyID)
	if err != nil {
		s.logger.Warn("credit check: failed to read valkey state, using db values",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
	}
	if !state.Populated {
		s.logger.Warn("credit check: valkey state missing, using db values",
			zap.String("company_id", companyID),
		)
		state.Balance = account.CreditBalanceMillicents
		state.Charged = account.CreditMonthlyChargedMillicents
		state.Pending = 0
	}

	minOperatingCost := s.getMinOperatingCost()
	if err := s.evaluateCreditAvailability(account, state.Balance, state.Charged, state.Pending, minOperatingCost); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *PlanService) evaluateCreditAvailability(account *billing.CreditAccount, balance, charged, pending, cost int64) error {
	if balance >= cost {
		return nil
	}

	if !account.IsTopupEnabled() {
		return ErrInsufficientCredit
	}

	cushion := int64(0)
	if pending > billing.CreditDisabled {
		cushion = pending
	} else {
		topup := account.CreditAutoTopupMillicents
		max := account.CreditMaxMonthlyChargeMillicents
		underMonthlyCap := (max == billing.CreditDisabled) || (charged+topup <= max)
		if topup > billing.CreditDisabled && underMonthlyCap {
			cushion = topup
		}
	}

	if balance-cost+cushion >= 0 {
		return nil
	}

	s.logger.Warn("credit check failed: deficit exceeds available cushion",
		zap.String("company_id", account.CompanyID),
		zap.Int64("balance", balance),
		zap.Int64("cost", cost),
		zap.Int64("cushion", cushion),
		zap.Int64("max_monthly", account.CreditMaxMonthlyChargeMillicents),
	)

	return ErrInsufficientCredit
}

func (s *PlanService) getMinOperatingCost() int64 {
	cfg := s.subConfig.Credit
	costs := []int64{
		cfg.ContractCostMillicents,
		cfg.IngressCostMillicents,
		cfg.EgressCostMillicents,
	}
	min := int64(math.MaxInt64)
	for _, c := range costs {
		if c > 0 && c < min {
			min = c
		}
	}
	if min == math.MaxInt64 {
		return minDebitFloorMillicents
	}
	return min
}

func (s *PlanService) CheckPayloadSize(ctx context.Context, account *billing.CreditAccount, payloadBytes int64) error {
	limit := account.PayloadLimitBytes
	if limit > 0 && payloadBytes > limit {
		return fmt.Errorf("payload too large: %d bytes exceeds limit of %d bytes", payloadBytes, limit)
	}
	return nil
}

func (s *PlanService) CheckRateLimit(ctx context.Context, account *billing.CreditAccount) (bool, error) {
	tps := account.RateLimitTPS
	if tps <= 0 {
		return true, nil
	}

	allowed, err := s.luaScripts.CheckCompanyRateLimit(ctx, account.CompanyID, int(tps), rateLimitWindowSeconds)
	if err != nil {
		s.logger.Error("rate limit script failure",
			zap.Error(err),
			zap.String("company_id", account.CompanyID),
		)
		return true, fmt.Errorf("check rate limit: %w", err)
	}

	return allowed, nil
}

func (s *PlanService) calculateCost(meter string, amount int64) int64 {
	cfg := s.subConfig.Credit
	switch meter {
	case MeterBandwidthIngress, MeterBandwidthEgress:
		costPerMB := cfg.IngressCostMillicents
		if meter == MeterBandwidthEgress {
			costPerMB = cfg.EgressCostMillicents
		}
		if costPerMB <= 0 {
			return 0
		}

		millicents := (amount * costPerMB) / (1024 * 1024)
		if millicents == 0 && amount > 0 {
			return 1 // Minimum 1 millicent floor for any usage
		}
		return millicents

	case MeterContractCreate, MeterContractVersionCreate:
		return amount * cfg.ContractCostMillicents
	case MeterSeatCreate:
		return amount * cfg.SeatCostMillicents
	case MeterLLMTokenUsage:
		return amount * cfg.LLMTokenCostMillicents
	default:
		return 0
	}
}

func (s *PlanService) invokeDebit(ctx context.Context, params *interfaces.DebitParams) (interfaces.DebitResult, error) {
	return s.luaScripts.DecrementCreditWithAutoTopup(ctx, params)
}

func (s *PlanService) chargeWithAccount(ctx context.Context, account *billing.CreditAccount, meter string, amount int64, allowTopup bool) error {
	cost := s.calculateCost(meter, amount)
	s.logger.Info("cost decrementing", zap.Any("meter", meter), zap.Any("cost", cost), zap.Any("amount", amount))
	if cost <= 0 {
		return nil
	}

	if err := s.debitCredits(ctx, account.CompanyID, cost, account, allowTopup); err != nil {
		return err
	}

	return nil
}

func (s *PlanService) debitCredits(ctx context.Context, companyID string, costMillicents int64, account *billing.CreditAccount, allowTopup bool) error {
	if costMillicents <= 0 {
		return nil
	}

	keys := billing.KeysFor(companyID)

	spendEventID := uuid.NewString()
	topupEventID := uuid.NewString()

	minBalance := account.CreditMinBalanceMillicents
	topupAmount := account.CreditAutoTopupMillicents
	maxMonthly := account.CreditMaxMonthlyChargeMillicents
	now := time.Now().UTC()

	params := &interfaces.DebitParams{
		BalanceKey:        keys.Balance,
		ChargedKey:        keys.Charged,
		PendingKey:        keys.Pending,
		StreamKey:         database.UsageOutboxStreamKey,
		Cost:              costMillicents,
		MinBalance:        minBalance,
		TopupAmount:       topupAmount,
		MaxMonthly:        maxMonthly,
		AllowTopup:        allowTopup,
		SpendEventID:      spendEventID,
		TopupEventID:      topupEventID,
		CompanyID:         companyID,
		BillingCycleStart: account.BillingCycleStart,
		OccurredAt:        now,
		SeedBalance:       account.CreditBalanceMillicents,
		SeedCharged:       account.CreditMonthlyChargedMillicents,
	}

	result, err := s.invokeDebit(ctx, params)
	if err != nil {
		return fmt.Errorf("invoke debit: %w", err)
	}

	if result.Allowed == debitDenied {
		return ErrInsufficientCredit
	}

	if result.TopupApplied == debitTopupHit {
		s.logger.Info("auto-topup triggered and applied",
			zap.String("company_id", companyID),
			zap.String("event_id", result.TopupStreamID))
	}

	if result.NewBalance < 0 {
		s.logger.Warn("credit balance negative after debit",
			zap.String("company_id", companyID),
			zap.Int64("balance_millicents", result.NewBalance),
		)
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
		fieldBillingCycleStart: billingCycleStart.Format(time.RFC3339Nano),
		fieldTimestamp:         time.Now().UTC().Format(time.RFC3339Nano),
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

type creditState struct {
	Balance   int64
	Charged   int64
	Pending   int64
	Populated bool
}

func (s *PlanService) readCreditState(ctx context.Context, companyID string) (creditState, error) {
	keys := billing.KeysFor(companyID)
	vals, err := s.valkeyClient.MGet(ctx, keys.Balance, keys.Charged, keys.Pending)
	if err != nil {
		return creditState{}, err
	}

	balanceRaw, ok := vals[keys.Balance]
	if !ok || balanceRaw == "" {
		return creditState{}, nil
	}
	chargedRaw, ok := vals[keys.Charged]
	if !ok || chargedRaw == "" {
		return creditState{}, nil
	}

	balance, err := strconv.ParseInt(balanceRaw, 10, 64)
	if err != nil {
		return creditState{}, err
	}
	charged, err := strconv.ParseInt(chargedRaw, 10, 64)
	if err != nil {
		return creditState{}, err
	}

	pending := int64(0)
	if pendingRaw, ok := vals[keys.Pending]; ok && pendingRaw != "" {
		if parsed, parseErr := strconv.ParseInt(pendingRaw, 10, 64); parseErr == nil {
			pending = parsed
		} else {
			return creditState{}, parseErr
		}
	}

	return creditState{
		Balance:   balance,
		Charged:   charged,
		Pending:   pending,
		Populated: true,
	}, nil
}

func (s *PlanService) CheckCreditAvailable(ctx context.Context, companyID, meter string, amount int64) error {
	account, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}

	cost := s.calculateCost(meter, amount)

	state, err := s.readCreditState(ctx, companyID)
	if err != nil {
		s.logger.Warn("failed to read credit state from valkey, falling back to DB values",
			zap.String("company_id", companyID),
			zap.Error(err),
		)
	}
	if !state.Populated {
		state.Balance = account.CreditBalanceMillicents
		state.Charged = account.CreditMonthlyChargedMillicents
		state.Pending = 0
	}

	return s.evaluateCreditAvailability(account, state.Balance, state.Charged, state.Pending, cost)
}

func (s *PlanService) DecrementIngress(ctx context.Context, companyID string, count int64) error {
	acc, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}
	allowTopup := acc.IsTopupEnabled()
	return s.chargeWithAccount(ctx, acc, MeterBandwidthIngress, count, allowTopup)
}

func (s *PlanService) DecrementEgress(ctx context.Context, companyID string, bytes int64) error {
	acc, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}
	allowTopup := acc.IsTopupEnabled()
	return s.chargeWithAccount(ctx, acc, MeterBandwidthEgress, bytes, allowTopup)
}

func (s *PlanService) ChargeContract(ctx context.Context, companyID string) error {
	acc, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}
	allowTopup := acc.IsTopupEnabled()
	return s.chargeWithAccount(ctx, acc, MeterContractCreate, 1, allowTopup)
}

func (s *PlanService) ChargeContractVersion(ctx context.Context, companyID string) error {
	acc, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}
	allowTopup := acc.IsTopupEnabled()
	return s.chargeWithAccount(ctx, acc, MeterContractVersionCreate, 1, allowTopup)
}

func (s *PlanService) DecrementLLMUsage(ctx context.Context, companyID string, tokens int64) error {
	acc, err := s.getCurrentAccount(ctx, companyID)
	if err != nil {
		return err
	}
	allowTopup := acc.IsTopupEnabled()
	return s.chargeWithAccount(ctx, acc, MeterLLMTokenUsage, tokens, allowTopup)
}

func (s *PlanService) ProcessRollovers(ctx context.Context) error {
	lockKey := database.CreditRolloverLockKey
	lockValue := uuid.NewString()
	lockTTL := rolloverLockTTL
	if s.valkeyClient != nil {
		acquired, err := s.valkeyClient.SetNX(ctx, lockKey, lockValue, lockTTL)
		if err != nil {
			s.logger.Warn("failed to acquire rollover lock; aborting rollover to be safe", zap.Error(err))
			return fmt.Errorf("rollover lock unavailable: %w", err)
		}
		if !acquired {
			s.logger.Info("rollover already in progress; skipping")
			return nil
		}
		defer func() {
			current, err := s.valkeyClient.Get(ctx, lockKey)
			if err != nil {
				return
			}
			if current == lockValue {
				_ = s.valkeyClient.Delete(ctx, lockKey)
			}
		}()
	} else {
		s.logger.Warn("valkey unavailable; rollover lock disabled")
	}

	companies, err := s.planRepo.ListCompaniesForRollover(ctx)
	if err != nil {
		return fmt.Errorf("list companies for rollover: %w", err)
	}

	now := time.Now().UTC()
	for companyID, extCustID := range companies {

		account, err := s.planRepo.GetCreditAccount(ctx, companyID)
		if err != nil {
			s.logger.Error("failed to get account for rollover", zap.String("company_id", companyID), zap.Error(err))
			continue
		}
		if account == nil {
			continue
		}

		nextCycle, due := nextBillingCycle(account.BillingCycleStart, now)
		if !due {
			continue
		}

		s.logger.Info("performing monthly rollover",
			zap.String("company_id", companyID),
			zap.Time("old_cycle_start", account.BillingCycleStart),
			zap.Time("new_cycle_start", nextCycle),
		)

		liveBalance := account.CreditBalanceMillicents
		finalCharged := account.CreditMonthlyChargedMillicents
		keys := billing.KeysFor(companyID)

		if s.valkeyClient != nil && s.luaScripts != nil {
			fetchedBalance, fetchedCharged, err := s.luaScripts.GetAndResetCharged(ctx, keys.Balance, keys.Charged)
			if err != nil {
				s.logger.Error("rollover atomic fetch failed; skipping company to avoid stale rollover",
					zap.String("company_id", companyID),
					zap.Error(err),
				)
				continue
			}
			liveBalance = fetchedBalance
			finalCharged = fetchedCharged
		}

		if err := s.planRepo.UpdateCumulativeMonthlyCharge(ctx, account.ID, finalCharged); err != nil {
			s.logger.Warn("failed to update old account with final charged amount",
				zap.String("company_id", companyID),
				zap.Error(err),
			)
		}

		newAccount := &billing.CreditAccount{
			ID:                               uuid.New().String(),
			CompanyID:                        companyID,
			ExternalCustomerID:               extCustID,
			BillingCycleStart:                nextCycle,
			CreditBalanceMillicents:          liveBalance,
			CreditMinBalanceMillicents:       account.CreditMinBalanceMillicents,
			CreditMaxMonthlyChargeMillicents: account.CreditMaxMonthlyChargeMillicents,
			CreditAutoTopupMillicents:        account.CreditAutoTopupMillicents,
			CreditMonthlyChargedMillicents:   0,
			RateLimitTPS:                     account.RateLimitTPS,
			PayloadLimitBytes:                account.PayloadLimitBytes,
		}

		if err := s.planRepo.CreateCreditAccount(ctx, newAccount); err != nil {
			if errors.Is(err, serror.ErrDuplicateCreditAccount) {
				s.logger.Info("rollover already applied by another instance, skipping",
					zap.String("company_id", companyID),
				)
				continue
			}
			s.logger.Error("failed to renew credit account in repo", zap.String("company_id", companyID), zap.Error(err))
			continue
		}
		s.InvalidatePlanCache(ctx, companyID)
	}

	return nil
}

func (s *PlanService) InvalidatePlanCache(ctx context.Context, companyID string) {
	err := s.valkeyClient.Delete(ctx, database.PlanCachePrefix+companyID)
	if err != nil {
		s.logger.Warn("failed to invalidate plan cache", zap.String("company_id", companyID), zap.Error(err))
	}
}

func (s *PlanService) getCacheEntry(ctx context.Context, companyID string) *CachedPlan {
	cached, err := s.valkeyClient.Get(ctx, database.PlanCachePrefix+companyID)
	if err != nil || cached == "" {
		return nil
	}
	var cp CachedPlan
	if err := json.Unmarshal([]byte(cached), &cp); err != nil {
		return nil
	}
	return &cp
}

func (s *PlanService) setCacheEntry(ctx context.Context, companyID string, cp *CachedPlan) {
	data, err := json.Marshal(cp)
	if err != nil {
		return
	}
	ttl := s.cacheTTL
	if jitterRange := int64(ttl / cacheTTLJitterFraction); jitterRange > 0 {
		ttl += time.Duration(rand.Int63n(jitterRange*cacheTTLJitterMultiplier)) - time.Duration(jitterRange)
	}
	_ = s.valkeyClient.Set(ctx, database.PlanCachePrefix+companyID, string(data), ttl)
}
