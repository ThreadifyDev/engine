package valkey

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/threadify/engine/internal/domain"
)

// Embed Lua scripts at compile time
//
//go:embed lua/grant_or_update_access.lua
var grantOrUpdateAccessScript string

//go:embed lua/check_company_rate_limit.lua
var checkCompanyRateLimitScript string

//go:embed lua/decrement_credit_with_autotopup.lua
var decrementCreditWithAutoTopupScript string

//go:embed lua/get_and_reset_charged.lua
var getAndResetChargedScript string

//go:embed lua/check_and_update_thread_status.lua
var checkAndUpdateThreadStatusScript string

// LuaScriptManager manages Lua script loading and execution
type LuaScriptManager struct {
	valkeyClient domain.ValkeyScriptClient
	scriptHashes map[string]string
}

// NewLuaScriptManager creates a new Lua script manager
func NewLuaScriptManager(valkeyClient domain.ValkeyScriptClient) *LuaScriptManager {
	return &LuaScriptManager{
		valkeyClient: valkeyClient,
		scriptHashes: make(map[string]string),
	}
}

// LoadScripts loads all Lua scripts into Valkey and stores their SHA hashes.
// Must be called once during application startup before any script is invoked.
func (m *LuaScriptManager) LoadScripts(ctx context.Context) error {
	scripts := map[string]string{
		"grant_or_update_access":          grantOrUpdateAccessScript,
		"check_company_rate_limit":        checkCompanyRateLimitScript,
		"decrement_credit_with_autotopup": decrementCreditWithAutoTopupScript,
		"get_and_reset_charged":           getAndResetChargedScript,
		"check_and_update_thread_status":  checkAndUpdateThreadStatusScript,
	}

	for name, script := range scripts {
		sha, err := m.valkeyClient.ScriptLoad(ctx, script)
		if err != nil {
			return fmt.Errorf("failed to load script %s: %w", name, err)
		}
		m.scriptHashes[name] = sha
	}

	return nil
}

// GetScriptHash returns the SHA hash for a loaded script.
func (m *LuaScriptManager) GetScriptHash(name string) (string, bool) {
	hash, exists := m.scriptHashes[name]
	return hash, exists
}

// CheckCompanyRateLimit returns true if the request is allowed, false if rate limited.
func (m *LuaScriptManager) CheckCompanyRateLimit(
	ctx context.Context,
	companyID string,
	requestsPerWindow int,
	windowSeconds int,
) (bool, error) {
	return m.checkRateLimit(ctx, fmt.Sprintf("ratelimit:company:%s", companyID), requestsPerWindow, windowSeconds)
}

// CheckIPRateLimit returns true if the request is allowed, false if rate limited.
func (m *LuaScriptManager) CheckIPRateLimit(
	ctx context.Context,
	ip string,
	requestsPerWindow int,
	windowSeconds int,
) (bool, error) {
	return m.checkRateLimit(ctx, fmt.Sprintf("ratelimit:ip:%s", ip), requestsPerWindow, windowSeconds)
}

// checkRateLimit is the shared implementation for company and IP rate limiting.
// Both use the same sliding-window Lua script; only the key prefix differs.
func (m *LuaScriptManager) checkRateLimit(
	ctx context.Context,
	key string,
	requestsPerWindow int,
	windowSeconds int,
) (bool, error) {
	scriptHash, exists := m.scriptHashes["check_company_rate_limit"]
	if !exists {
		return false, fmt.Errorf("script check_company_rate_limit not loaded")
	}

	nowNano := time.Now().UnixNano()
	windowStartNano := nowNano - int64(windowSeconds)*int64(time.Second)
	ttl := windowSeconds * 2 // TTL = 2× window so Valkey auto-cleans stale entries

	// Create a unique request ID to avoid sorted set member collisions.
	// Under burst, multiple requests can share the same nanosecond; using
	// timestamp alone as both score and member would silently merge them.
	requestID := fmt.Sprintf("%d:%d", nowNano, rand.Int63())

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash,
		[]string{key},
		nowNano,
		windowStartNano,
		requestsPerWindow,
		ttl,
		requestID,
	)
	if err != nil {
		return false, fmt.Errorf("failed to execute rate limit script: %w", err)
	}

	allowed, ok := result.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected result type from rate limit script: %T", result)
	}
	return allowed == 1, nil
}

// DecrementCreditWithAutoTopup atomically debits credits, optionally triggers a
// topup request, and appends outbox events — all in a single Lua call.
func (m *LuaScriptManager) DecrementCreditWithAutoTopup(
	ctx context.Context,
	params *domain.DebitParams,
) (domain.DebitResult, error) {
	scriptHash, exists := m.scriptHashes["decrement_credit_with_autotopup"]
	if !exists {
		return domain.DebitResult{}, fmt.Errorf("script decrement_credit_with_autotopup not loaded")
	}

	keys := []string{
		params.BalanceKey,
		params.ChargedKey,
		params.StreamKey,
		params.PendingKey,
	}
	args := []interface{}{
		params.Cost,
		params.MinBalance,
		params.TopupAmount,
		params.MaxMonthly,
		params.SpendEventID,
		params.TopupEventID,
		params.CompanyID,
		params.BillingCycleStart.UTC().Format(time.RFC3339Nano),
		params.OccurredAt.UTC().Format(time.RFC3339Nano),
		boolToIntString(params.AllowTopup),
		params.SeedBalance,
		params.SeedCharged,
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("failed to execute decrement_credit_with_autotopup: %w", err)
	}

	res, ok := result.([]interface{})
	if !ok || len(res) < 5 {
		return domain.DebitResult{}, fmt.Errorf("unexpected result shape from decrement_credit_with_autotopup: got %T len=%d", result, func() int {
			if ok {
				return len(res)
			}
			return -1
		}())
	}

	newBalance, err := parseLuaInt64(res[0])
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("parse new_balance: %w", err)
	}
	allowed, err := parseLuaInt64(res[1])
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("parse allowed: %w", err)
	}
	spendStreamID, err := parseLuaString(res[2])
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("parse spend_stream_id: %w", err)
	}
	topupStreamID, err := parseLuaString(res[3])
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("parse topup_stream_id: %w", err)
	}
	topupApplied, err := parseLuaInt64(res[4])
	if err != nil {
		return domain.DebitResult{}, fmt.Errorf("parse topup_applied: %w", err)
	}

	return domain.DebitResult{
		NewBalance:    newBalance,
		Allowed:       allowed,
		SpendStreamID: spendStreamID,
		TopupStreamID: topupStreamID,
		TopupApplied:  topupApplied,
	}, nil
}

func (m *LuaScriptManager) GetAndResetCharged(ctx context.Context, balanceKey, chargedKey string) (balance int64, charged int64, err error) {
	scriptHash, exists := m.scriptHashes["get_and_reset_charged"]
	if !exists {
		return 0, 0, fmt.Errorf("script get_and_reset_charged not loaded")
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, []string{balanceKey, chargedKey})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute get_and_reset_charged: %w", err)
	}

	res, ok := result.([]interface{})
	if !ok || len(res) < 2 {
		return 0, 0, fmt.Errorf("unexpected result shape from get_and_reset_charged: %T", result)
	}

	balance, err = parseLuaInt64(res[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse balance: %w", err)
	}
	charged, err = parseLuaInt64(res[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse charged: %w", err)
	}

	return balance, charged, nil
}

func parseLuaInt64(v interface{}) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case float64:
		return int64(t), nil
	case string:
		return strconv.ParseInt(t, 10, 64)
	case []byte:
		return strconv.ParseInt(string(t), 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type for int64: %T", v)
	}
}

func parseLuaString(v interface{}) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("unsupported type for string: %T", v)
	}
}

func boolToIntString(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
