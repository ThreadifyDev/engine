package service

import (
	"context"
	"fmt"
	"time"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/valkey"
)

// ThreadAccessService handles three-tier permission and role management
// This centralizes all permission/role operations with in-memory caching (mutex-protected)
// before hitting Valkey, following the same pattern as contract graph caching.
//
// Access writes are batched to reduce Valkey load (hybrid: 25 items OR 50ms flush)
// Permissions are resolved from runtime_role using RBAC loader (no storage needed)
type ThreadAccessService struct {
	accessRepo   *valkey.AccessRepository
	cacheManager interfaces.CacheManager
	luaScripts   *valkey.LuaScriptManager
	batcher      *AccessBatcher
	rbacLoader   *rbac.Loader
}

// NewThreadAccessService creates a new thread access service
func NewThreadAccessService(accessRepo *valkey.AccessRepository, cacheManager interfaces.CacheManager, luaScripts *valkey.LuaScriptManager, batcher *AccessBatcher, rbacLoader *rbac.Loader) *ThreadAccessService {
	return &ThreadAccessService{
		accessRepo:   accessRepo,
		cacheManager: cacheManager,
		luaScripts:   luaScripts,
		batcher:      batcher,
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

// GrantOrUpdateAccess grants or updates user access using batched writes
// Handles all scenarios: thread creator (invitedBy="self"), invitation join, and direct join
// Validates that thread is not completed before granting access
// Permissions are resolved from runtimeRole using RBAC loader (not stored)
func (s *ThreadAccessService) GrantOrUpdateAccess(
	threadID, userID string,
	role string,
	runtimeRole string,
	invitedBy string,
	thread *models.Thread,
) error {
	// Validate thread is not completed
	if thread.Status == "completed" || thread.Status == "failed" || thread.Status == "cancelled" {
		return fmt.Errorf("cannot join thread with status: %s", thread.Status)
	}

	// Write to Valkey (batched if batcher available, otherwise direct)
	if s.batcher != nil {
		// Queue write to batcher (non-blocking)
		write := &AccessWrite{
			ThreadID:    threadID,
			UserID:      userID,
			Role:        role,
			RuntimeRole: runtimeRole,
			InvitedBy:   invitedBy,
		}

		if err := s.batcher.Write(write); err != nil {
			return fmt.Errorf("failed to queue access write: %w", err)
		}
	} else {
		// Fallback to direct write (for internal services without batcher)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := s.accessRepo.GrantOrUpdateAccess(
			ctx,
			threadID, userID,
			role,
			runtimeRole,
			invitedBy,
			s.luaScripts,
			nil, // threadData
			nil, // threadTTL
		)
		if err != nil {
			return fmt.Errorf("failed to grant/update access: %w", err)
		}
	}

	// Update in-memory cache
	s.cacheManager.SetUserRole(threadID, userID, role)
	// Resolve and cache permissions from runtime_role (global cache)
	// Only if RBAC loader is available (may be nil for internal services)
	if s.rbacLoader != nil {
		perms := s.rbacLoader.GetPermissionsForRoles([]string{runtimeRole}, "runtime_level")
		s.cacheManager.SetRuntimeRolePermissions(runtimeRole, perms)
	}
	return nil
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
