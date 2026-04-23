package valkey

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

// AccessRepository handles role and permission management in Valkey
type AccessRepository struct {
	valkey        interfaces.ValkeyClient
	postgresRepo  PostgresAccessRepository // For hot/cold fallback
	rbacLoader    interfaces.RBACLoader    // For dynamic permission-to-role mapping
	ttl           int                      // TTL in seconds for access keys
	writeBackPool *workerpool.Pool         // For async cache write-backs
	logger        *zap.Logger
}

// PostgresAccessRepository defines the interface for PostgreSQL access operations
type PostgresAccessRepository interface {
	GetUserAccess(ctx context.Context, threadID, userID string) (*interfaces.UserAccess, error)
	GetAllAccess(ctx context.Context, threadID string) (map[string]*interfaces.UserAccess, error)
	GetUsersByRuntimeRoles(ctx context.Context, threadID string, runtimeRoles []string) ([]models.UserRoleInfo, error)
	GetUsersByPermissions(ctx context.Context, threadID string, requiredPermissions []string) ([]models.UserPermissionInfo, error)
}

// NewAccessRepository creates a new access repository
func NewAccessRepository(valkey interfaces.ValkeyClient, ttl int, logger *zap.Logger) *AccessRepository {
	return &AccessRepository{
		valkey: valkey,
		ttl:    ttl,
		logger: logger,
	}
}

// NewAccessRepositoryWithPostgres creates a new access repository with PostgreSQL fallback
func NewAccessRepositoryWithPostgres(valkey interfaces.ValkeyClient, postgresRepo PostgresAccessRepository, ttl int, logger *zap.Logger) *AccessRepository {
	return &AccessRepository{
		valkey:       valkey,
		postgresRepo: postgresRepo,
		ttl:          ttl,
		logger:       logger,
	}
}

// SetRBACLoader sets the RBAC loader for dynamic permission-to-role mapping
func (r *AccessRepository) SetRBACLoader(loader interfaces.RBACLoader) {
	r.rbacLoader = loader
}

// SetWriteBackPool sets the worker pool for async cache write-backs
func (r *AccessRepository) SetWriteBackPool(pool *workerpool.Pool) {
	r.writeBackPool = pool
}

// GrantOrUpdateAccess grants or updates user access using unified Lua script with role_index
// Handles all scenarios: thread creator, invitation join, and direct join
// - invitedBy = "self" → Thread creator (creates new access)
// - invitedBy != "self" && user exists → Add role only
// - invitedBy != "self" && user new → Create access with role
// Uses exponential backoff retry to handle concurrent access creation race conditions
// Returns the updated UserAccess object so service layer can pass it to ActivityRepository
// Optional threadData parameter enables atomic thread creation to prevent orphaned threads
// Note: runtime_role is stored in both Valkey (for fast permission checks) and PostgreSQL (via ActivityRepository)
func (r *AccessRepository) GrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	role string,
	runtimeRole string,
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
			ctx, threadID, userID, role, runtimeRole, permissions, invitedBy, luaScripts, threadData, threadTTL,
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
// Note: runtime_role must be passed in and will be stored in Valkey for fast permission checks
func (r *AccessRepository) attemptGrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	role string,
	runtimeRole string,
	permissions []string,
	invitedBy string,
	luaScripts interfaces.LuaScriptManager,
	threadData *string,
	threadTTL *int,
) (*interfaces.UserAccess, error) {
	roleIndexKey := r.getRoleIndexKey(threadID)
	accessKey := r.getAccessKey(threadID)
	threadKey := r.getThreadKey(threadID)

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

	// Serialize permissions to JSON for Lua script
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal permissions: %w", err)
	}

	result, err := r.valkey.EvalSHA(
		ctx,
		scriptHash,
		[]string{roleIndexKey, accessKey, threadKey}, // KEYS[1], KEYS[2], KEYS[3]
		invitedBy,               // ARGV[1]
		userID,                  // ARGV[2]
		role,                    // ARGV[3]
		runtimeRole,             // ARGV[4] - runtime_role for permission checks
		string(permissionsJSON), // ARGV[5] - resolved permissions as JSON array
		timestamp,               // ARGV[6]
		"active",                // ARGV[7]
		threadJSON,              // ARGV[8] - optional thread data
		ttl,                     // ARGV[9] - optional thread TTL
		r.ttl,                   // ARGV[10] - TTL for extending all thread keys
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

	r.logger.Info("Cache miss for user access, querying PostgreSQL",
		zap.String("user_id", userID),
		zap.String("thread_id", threadID))

	access, err := r.postgresRepo.GetUserAccess(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}

	r.logger.Info("Retrieved user access from PostgreSQL", zap.String("user_id", userID))

	// Async write-back via worker pool - don't block the read operation
	if shouldWriteBack && r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeBackCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			r.logger.Debug("Starting async cache write-back for user access",
				zap.String("user_id", userID),
				zap.String("thread_id", threadID))
			if err := r.writeAccessToValkey(writeBackCtx, threadID, userID, access); err != nil {
				r.logger.Warn("Failed to cache user access", zap.String("user_id", userID), zap.Error(err))
			} else {
				r.logger.Info("User access cached successfully", zap.String("user_id", userID))
			}
		})
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

	r.logger.Info("Cache miss for thread access, querying PostgreSQL", zap.String("thread_id", threadID))

	result, err := r.postgresRepo.GetAllAccess(ctx, threadID)
	if err != nil {
		return nil, err
	}

	r.logger.Info("Retrieved thread access from PostgreSQL", zap.String("thread_id", threadID))

	// Async write-back for all users via worker pool - don't block the read operation
	if shouldWriteBack && len(result) > 0 && r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeBackCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			r.logger.Debug("Starting async cache write-back for thread access",
				zap.String("thread_id", threadID),
				zap.Int("user_count", len(result)))
			successCount := 0
			for userID, access := range result {
				if err := r.writeAccessToValkey(writeBackCtx, threadID, userID, access); err == nil {
					successCount++
				}
			}
			r.logger.Info("Thread access cached",
				zap.String("thread_id", threadID),
				zap.Int("success_count", successCount),
				zap.Int("total_count", len(result)))
		})
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

// GetUserIDsByRuntimeRoles retrieves user IDs for runtime_roles
// Returns all user IDs across all roles
// Handles both single and multiple roles efficiently
// Hot/Cold pattern: Valkey first, PostgreSQL fallback with async write-back
func (r *AccessRepository) GetUserIDsByRuntimeRoles(ctx context.Context, threadID string, runtimeRoles []string) ([]string, error) {
	if len(runtimeRoles) == 0 {
		return []string{}, nil
	}

	result := make([]string, 0)

	// HOT PATH: Get members from each role SET in Valkey
	for _, role := range runtimeRoles {
		key := r.getUsersByRoleKey(threadID, role)
		userIDs, err := r.valkey.SMembers(ctx, key)
		if err != nil {
			// Ignore key not found errors, just skip
			continue
		}
		result = append(result, userIDs...)
	}

	// If Valkey has data, use it (hot path)
	if len(result) > 0 {
		return result, nil
	}

	// COLD PATH: Fall back to PostgreSQL if Valkey is empty
	users, err := r.postgresRepo.GetUsersByRuntimeRoles(ctx, threadID, runtimeRoles)
	if err != nil {
		return nil, fmt.Errorf("failed to get users from postgres: %w", err)
	}

	// Single pass: extract user IDs and group by role
	userIDs := make([]string, 0, len(users))
	roleToUsers := make(map[string][]string)

	for _, user := range users {
		userIDs = append(userIDs, user.UserID)
		roleToUsers[user.RuntimeRole] = append(roleToUsers[user.RuntimeRole], user.UserID)
	}

	// Write back to Valkey SETs via worker pool (async, don't block on this)
	if r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			// Use timeout context for async write (5s should be plenty for Redis operations)
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			for runtimeRole, ids := range roleToUsers {
				key := r.getUsersByRoleKey(threadID, runtimeRole)
				// SAdd accepts variadic interface{}, convert slice
				members := make([]interface{}, len(ids))
				for i, id := range ids {
					members[i] = id
				}
				// Add members to SET
				if err := r.valkey.SAdd(writeCtx, key, members...); err != nil {
					// Log error but don't fail (async write)
					continue
				}
				// Set TTL
				r.valkey.Expire(writeCtx, key, time.Duration(r.ttl)*time.Second)
			}
		})
	}

	return userIDs, nil
}

// AddUserToRoleSet adds a user to a runtime_role SET
// Used when populating from PostgreSQL or when Lua script data has expired
func (r *AccessRepository) AddUserToRoleSet(ctx context.Context, threadID, runtimeRole, userID string) error {
	key := r.getUsersByRoleKey(threadID, runtimeRole)
	if err := r.valkey.SAdd(ctx, key, userID); err != nil {
		return err
	}
	return r.valkey.Expire(ctx, key, time.Duration(r.ttl)*time.Second)
}

// RemoveUserFromRoleSet removes a user from a runtime_role SET
func (r *AccessRepository) RemoveUserFromRoleSet(ctx context.Context, threadID, runtimeRole, userID string) error {
	key := r.getUsersByRoleKey(threadID, runtimeRole)
	return r.valkey.SRem(ctx, key, userID)
}

// PopulateRoleSetsFromPostgres rebuilds role SETs from PostgreSQL data
// Called when Redis SETs are missing (cache miss) to warm the cache
// Only fetches and caches the specified runtime_roles for efficiency
// Returns the user IDs that were fetched (no need to read from Redis again)
func (r *AccessRepository) PopulateRoleSetsFromPostgres(
	ctx context.Context,
	threadID string,
	runtimeRoles []string,
) ([]string, error) {
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("postgres repository not configured")
	}

	// Use efficient query to get only users with specified roles
	usersInterface, err := r.postgresRepo.GetUsersByRuntimeRoles(ctx, threadID, runtimeRoles)
	if err != nil {
		return nil, fmt.Errorf("failed to get users from postgres: %w", err)
	}

	// Convert interface{} to []models.UserRoleInfo
	var users []models.UserRoleInfo
	jsonData, err := json.Marshal(usersInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal users: %w", err)
	}
	if err := json.Unmarshal(jsonData, &users); err != nil {
		return nil, fmt.Errorf("failed to unmarshal users: %w", err)
	}

	// Single pass: extract user IDs and group by role
	userIDs := make([]string, 0, len(users))
	roleToUsers := make(map[string][]string)

	for _, user := range users {
		userIDs = append(userIDs, user.UserID)
		roleToUsers[user.RuntimeRole] = append(roleToUsers[user.RuntimeRole], user.UserID)
	}

	// Write to Redis SETs via worker pool (async, don't block on this)
	if r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			// Use timeout context for async write (5s should be plenty for Redis operations)
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			for runtimeRole, ids := range roleToUsers {
				key := r.getUsersByRoleKey(threadID, runtimeRole)
				// SAdd accepts variadic interface{}, convert slice
				members := make([]interface{}, len(ids))
				for i, id := range ids {
					members[i] = id
				}
				// Add members to SET
				if err := r.valkey.SAdd(writeCtx, key, members...); err != nil {
					// Log error but don't fail (async write)
					continue
				}
				// Set TTL
				r.valkey.Expire(writeCtx, key, time.Duration(r.ttl)*time.Second)
			}
		})
	}

	return userIDs, nil
}

// GetUsersByPermissions retrieves users who have ANY of the required permissions
// Returns user_id AND permissions array for .own filtering in application layer
// Uses role-based lookup for efficiency (queries role sets, not all users)
func (r *AccessRepository) GetUsersByPermissions(
	ctx context.Context,
	threadID string,
	requiredPermissions []string,
) ([]models.UserPermissionInfo, error) {
	// Step 1: Map permissions to runtime roles
	// This determines which roles could have any of the required permissions
	runtimeRoles := r.getRuntimeRolesForPermissions(requiredPermissions)

	if len(runtimeRoles) == 0 {
		// No roles grant these permissions
		return []models.UserPermissionInfo{}, nil
	}

	// Step 2: Get user IDs from role sets (efficient O(1) lookup per role)
	userIDs, err := r.GetUserIDsByRuntimeRoles(ctx, threadID, runtimeRoles)
	if err != nil {
		return nil, err
	}

	if len(userIDs) == 0 {
		return []models.UserPermissionInfo{}, nil
	}

	// Step 3: Get full access data for these users only
	accessKey := r.getAccessKey(threadID)
	var users []models.UserPermissionInfo

	for _, userID := range userIDs {
		accessJSON, err := r.valkey.HGet(ctx, accessKey, userID)
		if err != nil {
			continue // Skip if user not found
		}

		var access interfaces.UserAccess
		if err := json.Unmarshal([]byte(accessJSON), &access); err != nil {
			continue // Skip invalid entries
		}

		// Verify user has the required permissions (wildcard matching)
		if hasAnyPermission(access.Permissions, requiredPermissions) {
			users = append(users, models.UserPermissionInfo{
				UserID:      userID,
				Permissions: access.Permissions,
			})
		}
	}

	return users, nil
}

// getRuntimeRolesForPermissions maps required permissions to runtime roles
// Returns which runtime roles could grant ANY of the required permissions
// Uses RBAC loader to dynamically determine mapping from roles.json
func (r *AccessRepository) getRuntimeRolesForPermissions(requiredPermissions []string) []string {
	// Fallback to all roles if RBAC loader not available (should not happen in production)
	if r.rbacLoader == nil {
		return []string{"owner", "participant", "observer", "external"}
	}

	allRuntimeRoles := []string{"owner", "participant", "observer", "external"}
	matchedRoles := make([]string, 0, len(allRuntimeRoles))

	// For each runtime role, check if it has ANY of the required permissions
	for _, role := range allRuntimeRoles {
		if r.roleHasAnyPermission(role, requiredPermissions) {
			matchedRoles = append(matchedRoles, role)
		}
	}

	return matchedRoles
}

// roleHasAnyPermission checks if a runtime role has any of the required permissions
func (r *AccessRepository) roleHasAnyPermission(role string, requiredPermissions []string) bool {
	// Get permissions for this role from roles.json via RBAC loader
	rolePerms := r.rbacLoader.GetPermissionsForRoles([]string{role}, "runtime_level")

	// Check if this role has any of the required permissions
	for _, rolePerm := range rolePerms {
		for _, reqPerm := range requiredPermissions {
			if matchesPermission(rolePerm, reqPerm) {
				return true // Found a match
			}
		}
	}

	return false // No matches found
}

// matchesPermission checks if a role permission matches a required permission
// Handles wildcard permissions (e.g., "notification.execution.*" matches "notification.execution.success.*")
func matchesPermission(rolePerm, reqPerm string) bool {
	// Exact match
	if rolePerm == reqPerm {
		return true
	}

	// Wildcard match: role has "notification.execution.*", req is "notification.execution.success.*"
	if strings.HasSuffix(rolePerm, ".*") {
		prefix := strings.TrimSuffix(rolePerm, ".*")
		if strings.HasPrefix(reqPerm, prefix) {
			return true
		}
	}

	return false
}

// hasAnyPermission checks if user has any of the required permissions
// Handles wildcard permissions (e.g., "notification.execution.*")
func hasAnyPermission(userPerms []string, requiredPerms []string) bool {
	for _, userPerm := range userPerms {
		for _, reqPerm := range requiredPerms {
			// Exact match
			if userPerm == reqPerm {
				return true
			}

			// Wildcard match: user has "notification.execution.*", req is "notification.execution.success.*"
			if strings.HasSuffix(userPerm, ".*") {
				prefix := strings.TrimSuffix(userPerm, ".*")
				if strings.HasPrefix(reqPerm, prefix) {
					return true
				}
			}
		}
	}
	return false
}

func (r *AccessRepository) getUsersByRoleKey(threadID, runtimeRole string) string {
	return fmt.Sprintf("thread:%s:users:%s", threadID, runtimeRole)
}

// PermissionCheckResult contains the result of a permission check
type PermissionCheckResult struct {
	HasAccess   bool     // User has some form of read access
	HasFullRead bool     // User has thread.read.* (can see all data)
	HasOwnRead  bool     // User has thread.read.own (can only see own data)
	Permissions []string // User's full permissions array
	RuntimeRole string   // User's runtime role
}

// CheckUserReadPermission checks if a user has read permission for a thread
// Returns detailed permission info for filtering decisions
// Hot path: Valkey first, PostgreSQL fallback
func (r *AccessRepository) CheckUserReadPermission(ctx context.Context, threadID, userID string) (*PermissionCheckResult, error) {
	key := r.getAccessKey(threadID)

	// Try Valkey first (hot path)
	accessJSON, err := r.valkey.HGet(ctx, key, userID)
	if err == nil && accessJSON != "" {
		var access interfaces.UserAccess
		if err := json.Unmarshal([]byte(accessJSON), &access); err == nil {
			return r.evaluateReadPermissions(&access), nil
		}
	}

	// Fallback to PostgreSQL
	if r.postgresRepo == nil {
		return &PermissionCheckResult{HasAccess: false}, nil
	}

	r.logger.Info("Cache miss for permission check, querying PostgreSQL",
		zap.String("user_id", userID),
		zap.String("thread_id", threadID))

	access, err := r.postgresRepo.GetUserAccess(ctx, threadID, userID)
	if err != nil {
		return &PermissionCheckResult{HasAccess: false}, nil
	}

	// Async write-back via worker pool
	if r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeBackCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			r.writeAccessToValkey(writeBackCtx, threadID, userID, access)
		})
	}

	return r.evaluateReadPermissions(access), nil
}

// evaluateReadPermissions evaluates read permissions from UserAccess
func (r *AccessRepository) evaluateReadPermissions(access *interfaces.UserAccess) *PermissionCheckResult {
	result := &PermissionCheckResult{
		HasAccess:   false,
		HasFullRead: false,
		HasOwnRead:  false,
		Permissions: access.Permissions,
		RuntimeRole: access.RuntimeRole,
	}

	// Check if user has active status
	if access.Status != "active" {
		return result
	}

	// Check permissions array for read access
	for _, perm := range access.Permissions {
		if perm == "thread.read.*" {
			result.HasAccess = true
			result.HasFullRead = true
			return result // Full access, no need to check further
		}
		if perm == "thread.read.own" {
			result.HasAccess = true
			result.HasOwnRead = true
			// Continue checking in case they also have full read
		}
	}

	return result
}
