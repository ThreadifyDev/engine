package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/threadify/engine/internal/interfaces"
)

// AccessRepository handles role and permission management in Valkey
type AccessRepository struct {
	valkey       interfaces.ValkeyClient
	postgresRepo PostgresAccessRepository // For hot/cold fallback
	ttl          int                      // TTL in seconds for access keys
}

// PostgresAccessRepository defines the interface for PostgreSQL access operations
type PostgresAccessRepository interface {
	GetUserAccess(ctx context.Context, threadID, userID string) (*interfaces.UserAccess, error)
	GetAllAccess(ctx context.Context, threadID string) (map[string]*interfaces.UserAccess, error)
}

// NewAccessRepository creates a new access repository
func NewAccessRepository(valkey interfaces.ValkeyClient, ttl int) *AccessRepository {
	return &AccessRepository{
		valkey: valkey,
		ttl:    ttl,
	}
}

// NewAccessRepositoryWithPostgres creates a new access repository with PostgreSQL fallback
func NewAccessRepositoryWithPostgres(valkey interfaces.ValkeyClient, postgresRepo PostgresAccessRepository, ttl int) *AccessRepository {
	return &AccessRepository{
		valkey:       valkey,
		postgresRepo: postgresRepo,
		ttl:          ttl,
	}
}

// GrantOrUpdateAccess grants or updates user access using unified Lua script with role_index
// Handles all scenarios: thread creator, invitation join, and direct join
// - invitedBy = "self" → Thread creator (creates new access)
// - invitedBy != "self" && user exists → Add role only
// - invitedBy != "self" && user new → Create access with role + permissions
// Uses exponential backoff retry to handle concurrent access creation race conditions
// Returns the updated UserAccess object so service layer can pass it to ActivityRepository
// Optional threadData parameter enables atomic thread creation to prevent orphaned threads
func (r *AccessRepository) GrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	role string,
	permissions []string,
	invitedBy string,
	luaScripts interfaces.LuaScriptManager,
	threadData *string,
	threadTTL *int,
) (*interfaces.UserAccess, error) {
	var result *interfaces.UserAccess

	// Use existing ExecuteWithBackoff for retry logic (10ms initial, 100ms max, 500ms total)
	err := r.valkey.ExecuteWithBackoff(ctx, func() error {
		access, err := r.attemptGrantOrUpdateAccess(
			ctx, threadID, userID, role, permissions, invitedBy, luaScripts, threadData, threadTTL,
		)
		if err != nil {
			// Check if error is retryable (concurrent creation detected)
			// Note: Lua script atomicity prevents races during role addition to existing users
			if strings.Contains(err.Error(), "CONCURRENT_ACCESS_CREATION") {
				return err // Retryable - backoff will retry
			}
			// Role already assigned to another user - non-retryable
			if strings.Contains(err.Error(), "ROLE_ALREADY_ASSIGNED") {
				return backoff.Permanent(fmt.Errorf("role already assigned to another user: %w", err))
			}
			// Non-retryable error (script not found, JSON error, etc.)
			return backoff.Permanent(err)
		}
		result = access
		return nil
	})

	return result, err
}

// attemptGrantOrUpdateAccess performs a single attempt to grant/update access
// Extracted from GrantOrUpdateAccess to enable retry logic
// Supports atomic thread creation via optional threadData and threadTTL parameters
func (r *AccessRepository) attemptGrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	role string,
	permissions []string,
	invitedBy string,
	luaScripts interfaces.LuaScriptManager,
	threadData *string,
	threadTTL *int,
) (*interfaces.UserAccess, error) {
	roleIndexKey := r.getRoleIndexKey(threadID)
	accessKey := r.getAccessKey(threadID)
	threadKey := r.getThreadKey(threadID)

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

	// Prepare optional thread creation parameters
	threadJSON := ""
	ttl := "0"
	if threadData != nil && *threadData != "" {
		threadJSON = *threadData
	}
	if threadTTL != nil && *threadTTL > 0 {
		ttl = fmt.Sprintf("%d", *threadTTL)
	}

	result, err := r.valkey.EvalSHA(
		ctx,
		scriptHash,
		[]string{roleIndexKey, accessKey, threadKey}, // KEYS[1], KEYS[2], KEYS[3]
		invitedBy,               // ARGV[1]
		userID,                  // ARGV[2]
		role,                    // ARGV[3]
		string(permissionsJSON), // ARGV[4]
		timestamp,               // ARGV[5]
		"active",                // ARGV[6]
		threadJSON,              // ARGV[7] - optional thread data
		ttl,                     // ARGV[8] - optional thread TTL
		r.ttl,                   // ARGV[9] - TTL for extending all thread keys
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

// GetUserAccess retrieves access for a user in a thread with optional PostgreSQL fallback
func (r *AccessRepository) GetUserAccess(ctx context.Context, threadID, userID string, writeBack ...bool) (*interfaces.UserAccess, error) {
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

	key := r.getAccessKey(threadID)

	// Try Valkey first
	accessJSON, err := r.valkey.HGet(ctx, key, userID)
	if err == nil && accessJSON != "" {
		var access interfaces.UserAccess
		if err := json.Unmarshal([]byte(accessJSON), &access); err == nil {
			return &access, nil
		}
	}

	// Fallback to PostgreSQL
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("user does not have access")
	}

	log.Printf("⚠️ [COLD] Access for user %s in thread %s not in Valkey, checking PostgreSQL", userID, threadID)

	access, err := r.postgresRepo.GetUserAccess(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}

	log.Printf("✅ [COLD] Access for user %s retrieved from PostgreSQL", userID)

	// Async write-back - don't block the read operation
	if shouldWriteBack {
		go func(threadID, userID string, access *interfaces.UserAccess) {
			writeBackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			log.Printf("🔄 [WRITE-BACK] Async caching access for user %s in thread %s", userID, threadID)
			if err := r.writeAccessToValkey(writeBackCtx, threadID, userID, access); err != nil {
				log.Printf("⚠️ Async write-back failed for access: %v", err)
			} else {
				log.Printf("✅ [WRITE-BACK] Access for user %s cached successfully", userID)
			}
		}(threadID, userID, access)
	}

	return access, nil
}

// GetAllAccess gets all access grants for a thread with optional PostgreSQL fallback
func (r *AccessRepository) GetAllAccess(ctx context.Context, threadID string, writeBack ...bool) (map[string]*interfaces.UserAccess, error) {
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

	key := r.getAccessKey(threadID)

	// Try Valkey first
	accessMap, err := r.valkey.HGetAll(ctx, key)
	if err == nil && len(accessMap) > 0 {
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

	// Fallback to PostgreSQL
	if r.postgresRepo == nil {
		return make(map[string]*interfaces.UserAccess), nil
	}

	log.Printf("⚠️ [COLD] Access for thread %s not in Valkey, checking PostgreSQL", threadID)

	result, err := r.postgresRepo.GetAllAccess(ctx, threadID)
	if err != nil {
		return nil, err
	}

	log.Printf("✅ [COLD] Access for thread %s retrieved from PostgreSQL", threadID)

	// Async write-back for all users - don't block the read operation
	if shouldWriteBack && len(result) > 0 {
		go func(threadID string, result map[string]*interfaces.UserAccess) {
			writeBackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			log.Printf("🔄 [WRITE-BACK] Async caching access for %d users in thread %s", len(result), threadID)
			successCount := 0
			for userID, access := range result {
				if err := r.writeAccessToValkey(writeBackCtx, threadID, userID, access); err != nil {
					log.Printf("⚠️ Async write-back failed for user %s access: %v", userID, err)
				} else {
					successCount++
				}
			}
			log.Printf("✅ [WRITE-BACK] Cached %d/%d user access records successfully", successCount, len(result))
		}(threadID, result)
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

func (r *AccessRepository) getThreadKey(threadID string) string {
	return fmt.Sprintf("thread:%s", threadID)
}

// writeAccessToValkey writes access back to Valkey in the same format as GrantOrUpdateAccess
func (r *AccessRepository) writeAccessToValkey(ctx context.Context, threadID, userID string, access *interfaces.UserAccess) error {
	key := r.getAccessKey(threadID)

	// Serialize access to JSON (same format as GrantOrUpdateAccess)
	accessJSON, err := json.Marshal(access)
	if err != nil {
		return fmt.Errorf("failed to marshal access: %w", err)
	}

	// Write to Valkey with TTL extension
	pipe := r.valkey.Pipeline()
	pipe.HSet(ctx, key, userID, string(accessJSON))
	pipe.Expire(ctx, key, time.Duration(r.ttl)*time.Second)

	// Extend TTL on all related thread keys to prevent partial expiration
	pipe.Expire(ctx, fmt.Sprintf("thread:%s", threadID), time.Duration(r.ttl)*time.Second)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:meta", threadID), time.Duration(r.ttl)*time.Second)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:role_index", threadID), time.Duration(r.ttl)*time.Second)

	_, err = pipe.Exec(ctx)
	return err
}
