package valkey

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
)

// Embed Lua scripts at compile time
//
//go:embed lua/grant_or_update_access.lua
var grantOrUpdateAccessScript string

//go:embed lua/check_company_rate_limit.lua
var checkCompanyRateLimitScript string

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
		"grant_or_update_access":   grantOrUpdateAccessScript,
		"check_company_rate_limit": checkCompanyRateLimitScript,
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
	nowUnix := time.Now().Unix()
	windowStart := nowUnix - int64(windowSeconds)
	ttl := windowSeconds * 2 // TTL = 2x window for cleanup

	keys := []string{key}
	args := []interface{}{
		nowUnix,
		windowStart,
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
