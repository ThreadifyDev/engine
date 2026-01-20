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
//go:embed lua/update_thread_status.lua
var updateThreadStatusScript string

//go:embed lua/grant_or_update_access.lua
var grantOrUpdateAccessScript string

//go:embed lua/check_user_rate_limit.lua
var checkUserRateLimitScript string

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
		"update_thread_status":   updateThreadStatusScript,
		"grant_or_update_access": grantOrUpdateAccessScript,
		"check_user_rate_limit":  checkUserRateLimitScript,
	}

	for name, script := range scripts {
		sha, err := m.valkeyClient.ScriptLoad(ctx, script)
		if err != nil {
			return fmt.Errorf("failed to load script %s: %w", name, err)
		}
		m.scriptHashes[name] = sha
		fmt.Printf("Loaded Lua script '%s' with SHA: %s\n", name, sha)
	}

	return nil
}

// GetScriptHash returns the SHA hash for a loaded script
func (m *LuaScriptManager) GetScriptHash(name string) (string, bool) {
	hash, exists := m.scriptHashes[name]
	return hash, exists
}

// UpdateThreadStatus executes the update_thread_status Lua script atomically
// Returns the final thread status after the update
func (m *LuaScriptManager) UpdateThreadStatus(
	ctx context.Context,
	threadID string,
	stepID string,
	newStatus string,
	stepStatus string,
	violationJSON string,
	timestamp string,
) (string, error) {
	scriptHash, exists := m.scriptHashes["update_thread_status"]
	if !exists {
		return "", fmt.Errorf("script update_thread_status not loaded")
	}

	keys := []string{
		"thread:" + threadID + ":meta",            // KEYS[1] - thread metadata hash
		"thread:" + threadID + ":violations",      // KEYS[2]
		"thread:" + threadID + ":steps:" + stepID, // KEYS[3]
	}

	args := []interface{}{
		newStatus,     // ARGV[1]
		stepID,        // ARGV[2]
		violationJSON, // ARGV[3]
		timestamp,     // ARGV[4]
		stepStatus,    // ARGV[5]
	}

	result, err := m.valkeyClient.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return "", fmt.Errorf("failed to execute update_thread_status: %w", err)
	}

	// Result should be the final thread status
	if status, ok := result.(string); ok {
		return status, nil
	}

	return "", fmt.Errorf("unexpected result type from script: %T", result)
}

// CheckUserRateLimit executes the check_user_rate_limit Lua script atomically
// Returns true if request is allowed, false if rate limit exceeded
func (m *LuaScriptManager) CheckUserRateLimit(
	ctx context.Context,
	userID string,
	requestsPerMinute int,
	windowSeconds int,
) (bool, error) {
	scriptHash, exists := m.scriptHashes["check_user_rate_limit"]
	if !exists {
		return false, fmt.Errorf("script check_user_rate_limit not loaded")
	}

	key := fmt.Sprintf("ratelimit:user:%s", userID)
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
		return false, fmt.Errorf("failed to execute check_user_rate_limit: %w", err)
	}

	// Result is 1 (allowed) or 0 (denied)
	if allowed, ok := result.(int64); ok {
		return allowed == 1, nil
	}

	return false, fmt.Errorf("unexpected result type from script: %T", result)
}
