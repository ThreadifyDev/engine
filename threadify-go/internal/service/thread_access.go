package service

import (
	"context"
	"fmt"
	"time"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/valkey"
)

// ThreadAccessService handles three-tier permission and role management
// This centralizes all permission/role operations with in-memory caching (mutex-protected)
// before hitting Valkey, following the same pattern as contract graph caching.
//
// Permissions are resolved from runtime_role using RBAC loader (no storage needed)
type ThreadAccessService struct {
	accessRepo   *valkey.AccessRepository
	cacheManager interfaces.CacheManager
	luaScripts   *valkey.LuaScriptManager
	rbacLoader   *rbac.Loader
}

// NewThreadAccessService creates a new thread access service
func NewThreadAccessService(accessRepo *valkey.AccessRepository, cacheManager interfaces.CacheManager, luaScripts *valkey.LuaScriptManager, rbacLoader *rbac.Loader) *ThreadAccessService {
	return &ThreadAccessService{
		accessRepo:   accessRepo,
		cacheManager: cacheManager,
		luaScripts:   luaScripts,
		rbacLoader:   rbacLoader,
	}
}

// GetUserPermissions retrieves user permissions by resolving from runtime_role:
// 1. Get user's runtime_role from Valkey/PostgreSQL
// 2. Check global runtime_role permission cache
// 3. If cache miss, resolve from RBAC loader and cache by runtime_role
func (s *ThreadAccessService) GetUserPermissions(threadID, userID string) ([]string, error) {
	start := time.Now()

	// Tier 1: Get user access (runtime_role) from Valkey/PostgreSQL
	valkeyStart := time.Now()
	access, err := s.accessRepo.GetUserAccess(context.Background(), threadID, userID)
	fmt.Printf("[PERF] GetUserPermissions.valkeyLookup: %v\n", time.Since(valkeyStart))
	if err != nil {
		return nil, err
	}

	// Tier 2: Check global runtime_role permission cache
	cacheCheckStart := time.Now()
	if perms, exists := s.cacheManager.GetRuntimeRolePermissions(access.RuntimeRole); exists {
		fmt.Printf("[PERF] GetUserPermissions.cacheHit: %v (runtime_role=%s)\n", time.Since(cacheCheckStart), access.RuntimeRole)
		fmt.Printf("[PERF] GetUserPermissions total: %v (cache hit)\n", time.Since(start))
		return perms, nil
	}
	fmt.Printf("[PERF] GetUserPermissions.cacheMiss: %v\n", time.Since(cacheCheckStart))

	// Tier 3: Resolve permissions from RBAC loader
	if s.rbacLoader == nil {
		return nil, fmt.Errorf("RBAC loader not initialized - cannot resolve permissions for runtime_role: %s", access.RuntimeRole)
	}
	rbacStart := time.Now()
	perms := s.rbacLoader.GetPermissionsForRoles([]string{access.RuntimeRole}, "runtime_level")
	fmt.Printf("[PERF] GetUserPermissions.rbacResolve: %v (runtime_role=%s)\n", time.Since(rbacStart), access.RuntimeRole)

	// Cache by runtime_role (global, not per-user)
	cacheSetStart := time.Now()
	s.cacheManager.SetRuntimeRolePermissions(access.RuntimeRole, perms)
	fmt.Printf("[PERF] GetUserPermissions.cacheSet: %v\n", time.Since(cacheSetStart))
	fmt.Printf("[PERF] GetUserPermissions total: %v (rbac resolution)\n", time.Since(start))
	return perms, nil
}

// GrantOrUpdateAccess grants or updates user access to a thread
// Note: runtime_role is resolved from roles and stored for permission checks
func (s *ThreadAccessService) GrantOrUpdateAccess(
	threadID, userID string,
	roles []string,
	runtimeRole string,
	invitedBy string,
) error {
	// For now, use first role as the primary role
	// Multi-role support will be added in future iterations
	role := roles[0]

	// Resolve permissions from runtime_role using RBAC loader
	var permissions []string
	if s.rbacLoader != nil {
		permissions = s.rbacLoader.GetPermissionsForRoles([]string{runtimeRole}, "runtime_level")
		// Cache permissions globally (for permission checks)
		s.cacheManager.SetRuntimeRolePermissions(runtimeRole, permissions)
	} else {
		// If RBAC loader not available, use empty permissions
		permissions = []string{}
	}

	// Direct write to Valkey
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.accessRepo.GrantOrUpdateAccess(
		ctx,
		threadID, userID,
		role,
		runtimeRole,
		permissions,
		invitedBy,
		s.luaScripts,
		nil, // threadData
		nil, // threadTTL
	)
	if err != nil {
		return fmt.Errorf("failed to grant/update access: %w", err)
	}

	// Update in-memory cache
	s.cacheManager.SetUserRole(threadID, userID, role)
	return nil
}

// GrantAccessWithThreadCreation atomically creates a thread and grants creator access
// This is used during thread creation to ensure thread and access are created together
// Resolves permissions from runtime_role before storing
func (s *ThreadAccessService) GrantAccessWithThreadCreation(
	ctx context.Context,
	threadID, userID string,
	role string,
	runtimeRole string,
	threadData *string,
	threadTTL *int,
) (*interfaces.UserAccess, error) {
	// Resolve permissions from runtime_role using RBAC loader
	rbacStart := time.Now()
	var permissions []string
	if s.rbacLoader != nil {
		permissions = s.rbacLoader.GetPermissionsForRoles([]string{runtimeRole}, "runtime_level")
		// Cache permissions globally (for permission checks)
		s.cacheManager.SetRuntimeRolePermissions(runtimeRole, permissions)
	} else {
		// If RBAC loader not available, use empty permissions
		permissions = []string{}
	}
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "rbac_permissions").Observe(time.Since(rbacStart).Seconds())

	// Call repository with resolved permissions
	repoStart := time.Now()
	access, err := s.accessRepo.GrantOrUpdateAccess(
		ctx,
		threadID, userID,
		role,
		runtimeRole,
		permissions,
		"self", // invitedBy for creator
		s.luaScripts,
		threadData,
		threadTTL,
	)
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "lua_script_exec").Observe(time.Since(repoStart).Seconds())
	if err != nil {
		return nil, fmt.Errorf("failed to grant access with thread creation: %w", err)
	}

	// Update in-memory cache
	cacheStart := time.Now()
	s.cacheManager.SetUserRole(threadID, userID, role)
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "cache_update").Observe(time.Since(cacheStart).Seconds())

	return access, nil
}

// GetUserRole retrieves role with three-tier caching
// Tier 1: In-memory cache (mutex-protected, ~0.1ms)
// Tier 2: Valkey (Redis, ~2-5ms)
func (s *ThreadAccessService) GetUserRole(threadID, userID string) (string, error) {
	// Tier 1: Check in-memory cache
	if role, exists := s.cacheManager.GetUserRole(threadID, userID); exists {
		return role, nil
	}

	// Tier 2: Check Valkey
	access, err := s.accessRepo.GetUserAccess(context.Background(), threadID, userID)
	if err != nil {
		return "", err
	}
	role := ""
	if len(access.Roles) > 0 {
		role = access.Roles[0] // Return first role for backward compatibility
	}

	// Cache in memory for future use (only if role exists)
	if role != "" {
		s.cacheManager.SetUserRole(threadID, userID, role)
	}
	return role, nil
}

// CheckThreadAccess checks if user has required permission for a thread
// Returns true if:
//  1. User is thread owner (implicit full access) - ownerID is derived from API key,
//     so multiple services using the same API key are all considered "owners", OR
//  2. User has the required permission stored in Valkey
//
// Uses three-tier caching for permission lookup
func (s *ThreadAccessService) CheckThreadAccess(threadID, userID, requiredPermission string, thread *models.Thread) (bool, error) {
	start := time.Now()
	defer func() {
		fmt.Printf("[PERF] CheckThreadAccess total: %v (thread=%s, user=%s)\n", time.Since(start), threadID[:8], userID)
	}()

	// Check if user is thread owner (implicit full access)
	ownerCheckStart := time.Now()
	if thread.OwnerID == userID {
		fmt.Printf("[PERF] CheckThreadAccess.ownerCheck: %v (OWNER - fast path)\n", time.Since(ownerCheckStart))
		return true, nil
	}
	fmt.Printf("[PERF] CheckThreadAccess.ownerCheck: %v (not owner)\n", time.Since(ownerCheckStart))

	// Get user permissions (with three-tier caching)
	permCheckStart := time.Now()
	permissions, err := s.GetUserPermissions(threadID, userID)
	fmt.Printf("[PERF] CheckThreadAccess.GetUserPermissions: %v\n", time.Since(permCheckStart))
	if err != nil {
		// No permissions found - access denied
		return false, nil
	}

	// Check if required permission exists
	loopStart := time.Now()
	for _, perm := range permissions {
		if perm == requiredPermission {
			fmt.Printf("[PERF] CheckThreadAccess.permissionLoop: %v (found)\n", time.Since(loopStart))
			return true, nil
		}
	}
	fmt.Printf("[PERF] CheckThreadAccess.permissionLoop: %v (not found)\n", time.Since(loopStart))

	return false, nil
}

// BatchCheckThreadAccess checks access for multiple threads at once
// Returns a map of threadID -> hasAccess
// This is more efficient than calling CheckThreadAccess in a loop
func (s *ThreadAccessService) BatchCheckThreadAccess(threads []*models.Thread, userID, requiredPermission string) (map[string]bool, error) {
	if len(threads) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool, len(threads))

	// First pass: Check ownership (no DB query needed)
	threadsNeedingPermCheck := make([]*models.Thread, 0)
	for _, thread := range threads {
		if thread.OwnerID == userID {
			result[thread.ID] = true // Owner has implicit full access
		} else {
			threadsNeedingPermCheck = append(threadsNeedingPermCheck, thread)
		}
	}

	// If all threads are owned by user, we're done
	if len(threadsNeedingPermCheck) == 0 {
		return result, nil
	}

	// Second pass: Batch check permissions for non-owned threads
	// For now, check individually (can be optimized with batch DB query later)
	for _, thread := range threadsNeedingPermCheck {
		permissions, err := s.GetUserPermissions(thread.ID, userID)
		if err != nil {
			result[thread.ID] = false
			continue
		}

		// Check if required permission exists
		hasPermission := false
		for _, perm := range permissions {
			if perm == requiredPermission {
				hasPermission = true
				break
			}
		}
		result[thread.ID] = hasPermission
	}

	return result, nil
}

// ValidateUserRoleForStep validates if user has required role for a contract step
// This is used for contract-based workflows where steps require specific roles
// (e.g., "buyer" can execute "create_order", "seller" can execute "confirm_shipment")
//
// Uses three-tier caching for role lookup
func (s *ThreadAccessService) ValidateUserRoleForStep(threadID, userID, requiredRole string) (bool, error) {
	// Get user's role (with three-tier caching)
	userRole, err := s.GetUserRole(threadID, userID)
	if err != nil {
		return false, err
	}

	return userRole == requiredRole, nil
}

// ClearThreadCache clears all cached roles for a thread
// This should be called when a thread is completed or deleted
// Note: Permissions are cached globally by runtime_role, not per-thread
func (s *ThreadAccessService) ClearThreadCache(threadID string) {
	s.cacheManager.ClearThreadRoles(threadID)
}

// GetUserIDsByRuntimeRoles retrieves user IDs for specified runtime_roles
// Uses Redis SETs (hot) with PostgreSQL fallback (cold) for efficient notification routing
func (s *ThreadAccessService) GetUserIDsByRuntimeRoles(
	ctx context.Context,
	threadID string,
	runtimeRoles []string,
) ([]string, error) {
	// Try Redis first (single pipelined call)
	userIDs, err := s.accessRepo.GetUserIDsByRuntimeRoles(ctx, threadID, runtimeRoles)
	if err != nil {
		return nil, fmt.Errorf("failed to get users from redis: %w", err)
	}

	// If no users found (complete cache miss), get from PostgreSQL
	if len(userIDs) == 0 {
		return s.accessRepo.PopulateRoleSetsFromPostgres(ctx, threadID, runtimeRoles)
	}

	return userIDs, nil
}

// GetUsersByPermissions retrieves users who have ANY of the required permissions
// Returns user info with permissions array for .own filtering in notification service
// Follows proper architecture: Redis → PostgreSQL → write-back to Redis
func (s *ThreadAccessService) GetUsersByPermissions(
	ctx context.Context,
	threadID string,
	requiredPermissions []string,
) ([]models.UserPermissionInfo, error) {
	// For now, we query PostgreSQL directly since we don't cache by permissions in Redis
	// The permissions are stored in PostgreSQL and queried via GIN index (efficient)
	// Future optimization: Could cache permission → userIDs mapping in Redis

	// This still respects separation of concerns:
	// - ThreadAccessService orchestrates the data access
	// - Repository handles the actual PostgreSQL query
	// - No direct PostgreSQL calls from NotificationService

	// Get users with permissions from repository
	valkeyUsers, err := s.accessRepo.GetUsersByPermissions(ctx, threadID, requiredPermissions)
	if err != nil {
		return nil, err
	}

	// Return directly - already models.UserPermissionInfo from repository
	return valkeyUsers, nil
}
