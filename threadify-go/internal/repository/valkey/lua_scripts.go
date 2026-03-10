package valkey

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"github.com/threadify/engine/internal/interfaces"
)

// Embed Lua scripts at compile time
//
//go:embed lua/grant_or_update_access.lua
var grantOrUpdateAccessScript string

//go:embed lua/check_company_rate_limit.lua
var checkCompanyRateLimitScript string

//go:embed lua/decrement_usage.lua
var decrementUsageScript string

//go:embed lua/decrement_usage_with_outbox.lua
var decrementUsageWithOutboxScript string

//go:embed lua/check_and_incr_quota.lua
var checkAndIncrQuotaScript string

// LuaScriptManager manages Lua script loading and execution
type LuaScriptManager struct {
	valkeyClient interfaces.ValkeyClient
	scriptHashes map[string]string
}

// NewLuaScriptManager creates a new Lua script manager
func NewLuaScriptManager(valkeyClient interfaces.ValkeyClient) *LuaScriptManager {
	return &LuaScriptManager{
		valkeyClient: valkeyClient,
		scriptHashes: make(map[string]string),
	}
}

// LoadScripts loads all Lua scripts into Valkey and stores their SHA hashes
// Should be called once during application startup
func (m *LuaScriptManager) LoadScripts(ctx context.Context) error {
	scripts := map[string]string{
		"grant_or_update_access":      grantOrUpdateAccessScript,
		"check_company_rate_limit":    checkCompanyRateLimitScript,
		"decrement_usage":             decrementUsageScript,
		"decrement_usage_with_outbox": decrementUsageWithOutboxScript,
		"check_and_incr_quota":        checkAndIncrQuotaScript,
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

// GetScriptHash returns the SHA hash for a loaded script
func (m *LuaScriptManager) GetScriptHash(name string) (string, bool) {
	hash, exists := m.scriptHashes[name]
	return hash, exists
}

// Returns true if request is allowed, false if rate limit exceeded
func (m *LuaScriptManager) CheckCompanyRateLimit(
	ctx context.Context,
	companyID string,
	requestsPerMinute int,
	windowSeconds int,
) (bool, error) {
	scriptHash, exists := m.scriptHashes["check_company_rate_limit"]
	if !exists {
		return false, fmt.Errorf("script check_company_rate_limit not loaded")
	}

	key := fmt.Sprintf("ratelimit:company:%s", companyID)
	nowNano := time.Now().UnixNano()
	windowStartNano := nowNano - (int64(windowSeconds) * time.Second.Nanoseconds())
	ttl := windowSeconds * 2 // TTL = 2x window for cleanup (EXPIRE takes seconds)

	keys := []string{key}
	args := []interface{}{
		nowNano,
		windowStartNano,
		requestsPerMinute,
		ttl,
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return false, fmt.Errorf("failed to execute check_company_rate_limit: %w", err)
	}

	// Result is 1 (allowed) or 0 (denied)
	if allowed, ok := result.(int64); ok {
		return allowed == 1, nil
	}

	return false, fmt.Errorf("unexpected result type from script: %T", result)
}

// CheckIPRateLimit returns true if request is allowed, false if rate limit exceeded
func (m *LuaScriptManager) CheckIPRateLimit(
	ctx context.Context,
	ip string,
	requestsPerWindow int,
	windowSeconds int,
) (bool, error) {
	scriptHash, exists := m.scriptHashes["check_company_rate_limit"] // Use same logic script
	if !exists {
		return false, fmt.Errorf("script check_company_rate_limit not loaded")
	}

	key := fmt.Sprintf("ratelimit:ip:%s", ip)
	nowNano := time.Now().UnixNano()
	windowStartNano := nowNano - (int64(windowSeconds) * time.Second.Nanoseconds())
	ttl := windowSeconds * 2 // TTL = 2x window for cleanup

	keys := []string{key}
	args := []interface{}{
		nowNano,
		windowStartNano,
		requestsPerWindow,
		ttl,
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return false, fmt.Errorf("failed to execute check_ip_rate_limit: %w", err)
	}

	// Result is 1 (allowed) or 0 (denied)
	if allowed, ok := result.(int64); ok {
		return allowed == 1, nil
	}

	return false, fmt.Errorf("unexpected result type from script: %T", result)
}

// DecrementUsage decrements a balance key with a floor check.
// Returns: newBalance, allowed (1 if allowed, 0 if denied, -1 if key missing), error
func (m *LuaScriptManager) DecrementUsage(ctx context.Context, key string, amount int64, floor int64) (int64, int64, error) {
	scriptHash, exists := m.scriptHashes["decrement_usage"]
	if !exists {
		return 0, 0, fmt.Errorf("script decrement_usage not loaded")
	}

	keys := []string{key}
	args := []interface{}{amount, floor}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute decrement_usage: %w", err)
	}

	if res, ok := result.([]interface{}); ok && len(res) == 2 {
		return res[0].(int64), res[1].(int64), nil
	}

	return 0, 0, fmt.Errorf("unexpected result type from decrement_usage: %T", result)
}

// DecrementUsageWithOutbox decrements a balance key with a floor check and appends a usage outbox stream event.
// Returns: newBalance, allowed (1 allowed, 0 denied, -1 key missing, -2 invalid value), streamID, error
func (m *LuaScriptManager) DecrementUsageWithOutbox(
	ctx context.Context,
	balanceKey string,
	streamKey string,
	amount int64,
	floor int64,
	eventID string,
	companyID string,
	meter string,
	billingCycleStart time.Time,
	occurredAt time.Time,
) (int64, int64, string, error) {
	scriptHash, exists := m.scriptHashes["decrement_usage_with_outbox"]
	if !exists {
		return 0, 0, "", fmt.Errorf("script decrement_usage_with_outbox not loaded")
	}

	keys := []string{balanceKey, streamKey}
	args := []interface{}{
		amount,
		floor,
		eventID,
		companyID,
		meter,
		amount,
		billingCycleStart.UTC().Format(time.RFC3339Nano),
		occurredAt.UTC().Format(time.RFC3339Nano),
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to execute decrement_usage_with_outbox: %w", err)
	}

	res, ok := result.([]interface{})
	if !ok || len(res) < 3 {
		return 0, 0, "", fmt.Errorf("unexpected result type from decrement_usage_with_outbox: %T", result)
	}

	newBalance, err := parseLuaInt64Value(res[0])
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse new balance: %w", err)
	}
	allowed, err := parseLuaInt64Value(res[1])
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse allowed flag: %w", err)
	}
	streamID, err := parseLuaStringValue(res[2])
	if err != nil {
		return 0, 0, "", fmt.Errorf("parse stream id: %w", err)
	}

	return newBalance, allowed, streamID, nil
}

// CheckAndIncrQuota checks if a counter is under the limit and increments it.
// Returns: newCount, allowed (1 if allowed, 0 if denied, -1 if key missing), error
func (m *LuaScriptManager) CheckAndIncrQuota(ctx context.Context, key string, limit int, amount int) (int64, int64, error) {
	scriptHash, exists := m.scriptHashes["check_and_incr_quota"]
	if !exists {
		return 0, 0, fmt.Errorf("script check_and_incr_quota not loaded")
	}

	keys := []string{key}
	args := []interface{}{limit, amount}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to execute check_and_incr_quota: %w", err)
	}

	if res, ok := result.([]interface{}); ok && len(res) == 2 {
		return res[0].(int64), res[1].(int64), nil
	}

	return 0, 0, fmt.Errorf("unexpected result type from check_and_incr_quota: %T", result)
}

func parseLuaInt64Value(v interface{}) (int64, error) {
	switch t := v.(type) {
	case int64:
		return t, nil
	case int:
		return int64(t), nil
	case float64:
		return int64(t), nil
	case string:
		parsed, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	case []byte:
		parsed, err := strconv.ParseInt(string(t), 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported lua numeric type: %T", v)
	}
}

func parseLuaStringValue(v interface{}) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case []byte:
		return string(t), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("unsupported lua string type: %T", v)
	}
}
