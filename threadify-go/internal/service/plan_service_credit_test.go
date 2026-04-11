package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"threadify-go/shared/billing"
	sharedrepo "threadify-go/shared/repository"

	"github.com/stretchr/testify/require"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

type stubPlanRepo struct {
	account *billing.CreditAccount
}

func (r *stubPlanRepo) GetCreditAccount(ctx context.Context, companyID string) (*billing.CreditAccount, error) {
	return r.account, nil
}

func (r *stubPlanRepo) GetExternalCustomerID(ctx context.Context, companyID string) (string, error) {
	return "", nil
}
func (r *stubPlanRepo) SetExternalCustomerID(ctx context.Context, companyID, externalCustomerID string) error {
	return nil
}
func (r *stubPlanRepo) FindCompanyByExternalCustomerID(ctx context.Context, externalCustomerID string) (string, error) {
	return "", nil
}
func (r *stubPlanRepo) ListCompaniesForRollover(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (r *stubPlanRepo) CreateCreditAccount(ctx context.Context, account *billing.CreditAccount) error {
	return nil
}
func (r *stubPlanRepo) UpdateTopupSettings(ctx context.Context, companyID string, autoTopupAmount, minBalance int64) error {
	return nil
}
func (r *stubPlanRepo) UpdateCumulativeMonthlyCharge(ctx context.Context, id string, amount int64) error {
	return nil
}
func (r *stubPlanRepo) UpdateMaxMonthlyCharge(ctx context.Context, companyID string, maxMonthlyMillicents int64) error {
	return nil
}

// stubLuaScripts satisfies the interface but is unused in these tests.
type stubLuaScripts struct{}

func (*stubLuaScripts) GetScriptHash(name string) (string, bool) { return "", false }
func (*stubLuaScripts) CheckCompanyRateLimit(ctx context.Context, companyID string, requestsPerMinute int, windowSeconds int) (bool, error) {
	return true, nil
}
func (*stubLuaScripts) CheckIPRateLimit(ctx context.Context, ip string, requestsPerWindow int, windowSeconds int) (bool, error) {
	return true, nil
}
func (*stubLuaScripts) DecrementCreditWithAutoTopup(ctx context.Context, params *interfaces.DebitParams) (interfaces.DebitResult, error) {
	return interfaces.DebitResult{}, nil
}
func (*stubLuaScripts) GetAndResetCharged(ctx context.Context, balanceKey, chargedKey string) (int64, int64, error) {
	return 0, 0, nil
}

func newPlanServiceForTest(repo sharedrepo.PlanRepository, valkey interfaces.ValkeyClient, subCfg *config.SubscriptionConfig) *PlanService {
	if subCfg == nil {
		subCfg = &config.SubscriptionConfig{Credit: config.CreditConfig{}}
	}
	logger, _ := zap.NewDevelopment()
	return NewPlanService(repo, nil, nil, subCfg, valkey, &stubLuaScripts{}, logger, 0)
}

func TestCheckBalancePositive_AllowsZeroBalanceWithAutoTopup(t *testing.T) {
	companyID := "c-1"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          0,
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        100,
		CreditMaxMonthlyChargeMillicents: 500,
	}

	valkey := newMockValkey()
	keys := billing.KeysFor(companyID)
	_ = valkey.Set(context.Background(), keys.Balance, "0", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)
	_ = valkey.Set(context.Background(), keys.Pending, "0", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckBalancePositive_FailsWhenTopupDisabledAndBalanceZero(t *testing.T) {
	companyID := "c-2"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          0,
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	valkey := newMockValkey()
	keys := billing.KeysFor(companyID)
	_ = valkey.Set(context.Background(), keys.Balance, "0", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_FallbacksToDBOnMissingValkey(t *testing.T) {
	companyID := "c-3"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          0,
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	valkey := newMockValkey() // no keys set -> fallback to DB values

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_AllowsPendingTopup(t *testing.T) {
	companyID := "c-4"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          0,
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        100,
		CreditMaxMonthlyChargeMillicents: 500,
	}

	keys := billing.KeysFor(companyID)
	valkey := newMockValkey()
	_ = valkey.Set(context.Background(), keys.Balance, "0", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)
	_ = valkey.Set(context.Background(), keys.Pending, "50", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckBalancePositive_FailsWhenLowBalanceAndTopupDisabled(t *testing.T) {
	companyID := "c-low-1"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          500, // Below 1000 floor
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	valkey := newMockValkey()
	keys := billing.KeysFor(companyID)
	_ = valkey.Set(context.Background(), keys.Balance, "500", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.ErrorIs(t, err, ErrInsufficientCredit)
	require.Nil(t, got)
}

func TestCheckBalancePositive_SucceedsWhenLowBalanceAndTopupEnabled(t *testing.T) {
	companyID := "c-low-2"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          500, // Below 1000 floor
		CreditMonthlyChargedMillicents:   0,
		CreditAutoTopupMillicents:        5000,
		CreditMaxMonthlyChargeMillicents: 10000,
	}

	valkey := newMockValkey()
	keys := billing.KeysFor(companyID)
	_ = valkey.Set(context.Background(), keys.Balance, "500", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, nil)

	got, err := svc.CheckBalancePositive(context.Background(), companyID)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCheckCreditAvailable_HandlesSpecificCost(t *testing.T) {
	companyID := "c-avail-1"
	account := &billing.CreditAccount{
		CompanyID:                        companyID,
		CreditBalanceMillicents:          5000,
		CreditAutoTopupMillicents:        0,
		CreditMaxMonthlyChargeMillicents: 0,
	}

	subCfg := &config.SubscriptionConfig{
		Credit: config.CreditConfig{
			ContractCostMillicents: 10000,
		},
	}

	valkey := newMockValkey()
	keys := billing.KeysFor(companyID)
	_ = valkey.Set(context.Background(), keys.Balance, "5000", 0)
	_ = valkey.Set(context.Background(), keys.Charged, "0", 0)

	svc := newPlanServiceForTest(&stubPlanRepo{account: account}, valkey, subCfg)

	// Cost of 1 contract is 10000. Balance is 5000. Topup disabled.
	// Cost of 1 contract is 10000. Balance is 5000. Topup disabled.
	err := svc.CheckCreditAvailable(context.Background(), companyID, MeterContractCreate, 1)
	require.ErrorIs(t, err, ErrInsufficientCredit)

	// Now enable topup
	account.CreditAutoTopupMillicents = 10000
	account.CreditMaxMonthlyChargeMillicents = 50000
	svc.InvalidatePlanCache(context.Background(), companyID)

	err = svc.CheckCreditAvailable(context.Background(), companyID, MeterContractCreate, 1)
	require.NoError(t, err)
}

// Local mock ValkeyClient sufficient for these tests.
type mockValkey struct {
	data map[string]string
}

func newMockValkey() *mockValkey {
	return &mockValkey{data: make(map[string]string)}
}

func (m *mockValkey) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *mockValkey) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if _, exists := m.data[key]; exists {
		return false, nil
	}
	m.data[key] = value.(string)
	return true, nil
}

func (m *mockValkey) Get(ctx context.Context, key string) (string, error) {
	return m.data[key], nil
}

func (m *mockValkey) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockValkey) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockValkey) Keys(ctx context.Context, pattern string) ([]string, error) { return nil, nil }

func (m *mockValkey) MGet(ctx context.Context, keys ...string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = m.data[k]
	}
	return out, nil
}

func (m *mockValkey) Expire(ctx context.Context, key string, ttl time.Duration) error { return nil }
func (m *mockValkey) Del(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}
func (m *mockValkey) TTL(ctx context.Context, key string) (time.Duration, error) { return 0, nil }

func (m *mockValkey) HSet(ctx context.Context, key string, values ...interface{}) error { return nil }
func (m *mockValkey) HGet(ctx context.Context, key, field string) (string, error)       { return "", nil }
func (m *mockValkey) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *mockValkey) HDel(ctx context.Context, key string, fields ...string) error { return nil }

func (m *mockValkey) LPush(ctx context.Context, key string, values ...interface{}) error { return nil }
func (m *mockValkey) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return nil, nil
}

func (m *mockValkey) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return nil
}
func (m *mockValkey) ZRem(ctx context.Context, key string, members ...string) error { return nil }
func (m *mockValkey) ZCard(ctx context.Context, key string) (int64, error)          { return 0, nil }
func (m *mockValkey) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return nil, nil
}

func (m *mockValkey) SAdd(ctx context.Context, key string, members ...interface{}) error { return nil }
func (m *mockValkey) SMembers(ctx context.Context, key string) ([]string, error)         { return nil, nil }
func (m *mockValkey) SRem(ctx context.Context, key string, members ...interface{}) error { return nil }

func (m *mockValkey) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return nil, nil
}
func (m *mockValkey) ScriptLoad(ctx context.Context, script string) (string, error) { return "", nil }
func (m *mockValkey) EvalSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error) {
	return nil, nil
}

func (m *mockValkey) DecrBy(ctx context.Context, key string, value int64) (int64, error) {
	return m.IncrBy(ctx, key, -value)
}
func (m *mockValkey) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	current, _ := strconv.ParseInt(m.data[key], 10, 64)
	current += value
	m.data[key] = strconv.FormatInt(current, 10)
	return current, nil
}

func (m *mockValkey) Pipeline() interfaces.ValkeyPipeline { return &mockPipeline{} }

func (m *mockValkey) XAdd(ctx context.Context, stream string, id string, values interface{}) (string, error) {
	return "", nil
}

func (m *mockValkey) ExecuteWithBackoff(ctx context.Context, operation func() error) error {
	return operation()
}

func (m *mockValkey) ApplyCreditTopupAtomic(ctx context.Context, balanceKey, pendingKey string, amount int64) (int64, error) {
	current, _ := strconv.ParseInt(m.data[balanceKey], 10, 64)
	current += amount
	m.data[balanceKey] = strconv.FormatInt(current, 10)
	m.data[pendingKey] = "0"
	return current, nil
}

type mockPipeline struct{}

func (p *mockPipeline) HSet(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	return p
}
func (p *mockPipeline) HDel(ctx context.Context, key string, fields ...string) interfaces.ValkeyPipeline {
	return p
}
func (p *mockPipeline) LPush(ctx context.Context, key string, values ...interface{}) interfaces.ValkeyPipeline {
	return p
}
func (p *mockPipeline) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) interfaces.ValkeyPipeline {
	return p
}
func (p *mockPipeline) Del(ctx context.Context, keys ...string) interfaces.ValkeyPipeline { return p }
func (p *mockPipeline) Expire(ctx context.Context, key string, expiration time.Duration) interfaces.ValkeyPipeline {
	return p
}
func (p *mockPipeline) Exec(ctx context.Context) ([]interface{}, error) { return nil, nil }
