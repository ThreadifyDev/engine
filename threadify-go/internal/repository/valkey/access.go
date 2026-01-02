package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
)

// AccessRepository handles role and permission management in Valkey
type AccessRepository struct {
	valkey interfaces.ValkeyClient
}

// NewAccessRepository creates a new access repository
func NewAccessRepository(valkey interfaces.ValkeyClient) *AccessRepository {
	return &AccessRepository{
		valkey: valkey,
	}
}

// GrantOrUpdateAccess grants or updates user access using unified Lua script with role_index
// Handles all scenarios: thread creator, invitation join, and direct join
// - invitedBy = "self" → Thread creator (creates new access)
// - invitedBy != "self" && user exists → Add role only
// - invitedBy != "self" && user new → Create access with role + permissions
// Enforces role uniqueness: only one user can have a specific role at a time
// Returns the updated UserAccess object so service layer can pass it to ActivityRepository
func (r *AccessRepository) GrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	role string,
	permissions []string,
	invitedBy string,
	luaScripts interfaces.LuaScriptManager,
) (*interfaces.UserAccess, error) {
	roleIndexKey := r.getRoleIndexKey(threadID)
	accessKey := r.getAccessKey(threadID)

	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	timestamp := time.Now().Format(time.RFC3339)

	// Execute unified Lua script with role_index
	scriptHash, exists := luaScripts.GetScriptHash("grant_or_update_access")
	if !exists {
		return nil, fmt.Errorf("script grant_or_update_access not loaded")
	}

	result, err := r.valkey.EvalSHA(
		ctx,
		scriptHash,
		[]string{roleIndexKey, accessKey}, // KEYS[1], KEYS[2]
		invitedBy,                         // ARGV[1]
		userID,                            // ARGV[2]
		role,                              // ARGV[3]
		string(permissionsJSON),           // ARGV[4]
		timestamp,                         // ARGV[5]
		"active",                          // ARGV[6]
	)
	if err != nil {
		return nil, fmt.Errorf("failed to grant/update access: %w", err)
	}

	// Parse result and return UserAccess object
	accessJSON, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from Lua script: %T", result)
	}

	var access interfaces.UserAccess
	if err := json.Unmarshal([]byte(accessJSON), &access); err != nil {
		return nil, fmt.Errorf("failed to parse access result: %w", err)
	}

	return &access, nil
}

// GetUserAccess retrieves access for a user in a thread
func (r *AccessRepository) GetUserAccess(ctx context.Context, threadID, userID string) (*interfaces.UserAccess, error) {
	key := r.getAccessKey(threadID)

	accessJSON, err := r.valkey.HGet(ctx, key, userID)
	if err != nil {
		return nil, err
	}

	if accessJSON == "" {
		return nil, fmt.Errorf("user does not have access")
	}

	var access interfaces.UserAccess
	if err := json.Unmarshal([]byte(accessJSON), &access); err != nil {
		return nil, fmt.Errorf("failed to parse access: %w", err)
	}

	return &access, nil
}

// GetAllAccess gets all access grants for a thread
func (r *AccessRepository) GetAllAccess(ctx context.Context, threadID string) (map[string]*interfaces.UserAccess, error) {
	key := r.getAccessKey(threadID)
	accessMap, err := r.valkey.HGetAll(ctx, key)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*interfaces.UserAccess)
	for userID, accessJSON := range accessMap {
		var access interfaces.UserAccess
		if err := json.Unmarshal([]byte(accessJSON), &access); err != nil {
			continue // Skip invalid entries
		}
		result[userID] = &access
	}

	return result, nil
}

// RevokeAccess removes all access for a user in a thread
func (r *AccessRepository) RevokeAccess(ctx context.Context, threadID, userID string) error {
	key := r.getAccessKey(threadID)
	return r.valkey.HDel(ctx, key, userID)
}

// Helper methods for key generation

func (r *AccessRepository) getAccessKey(threadID string) string {
	return fmt.Sprintf("thread:%s:access", threadID)
}

func (r *AccessRepository) getRoleIndexKey(threadID string) string {
	return fmt.Sprintf("thread:%s:role_index", threadID)
}
