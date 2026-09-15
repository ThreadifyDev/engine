package valkey

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"time"

	"github.com/threadify/engine/internal/domain"
)

// Embed Lua scripts at compile time
//
//go:embed lua/grant_or_update_access.lua
var grantOrUpdateAccessScript string

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
