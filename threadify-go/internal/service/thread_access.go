package service

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/valkey"
)

// ThreadAccessService handles three-tier permission and role management
// This centralizes all permission/role operations with in-memory caching (mutex-protected)
// before hitting Valkey, following the same pattern as contract graph caching.
//
// TODO: Add batching support for permission/role writes to Valkey (similar to step event batching)
// This would reduce Valkey write operations by buffering multiple permission changes
// and flushing them in batches (e.g., every 100ms or 50 operations).
type ThreadAccessService struct {
	accessRepo   *valkey.AccessRepository
	cacheManager interfaces.CacheManager
	luaScripts   *valkey.LuaScriptManager
}

// NewThreadAccessService creates a new thread access service
func NewThreadAccessService(accessRepo *valkey.AccessRepository, cacheManager interfaces.CacheManager, luaScripts *valkey.LuaScriptManager) *ThreadAccessService {
	return &ThreadAccessService{
		accessRepo:   accessRepo,
		cacheManager: cacheManager,
		luaScripts:   luaScripts,
	}
}

// GetUserPermissions retrieves permissions with three-tier caching
// Tier 1: In-memory cache (mutex-protected, ~0.1ms)
// Tier 2: Valkey (Redis, ~2-5ms)
func (s *ThreadAccessService) GetUserPermissions(threadID, userID string) ([]string, error) {
	// Tier 1: Check in-memory cache
	if perms, exists := s.cacheManager.GetUserPermissions(threadID, userID); exists {
		return perms, nil
	}

	// Tier 2: Check Valkey
	access, err := s.accessRepo.GetUserAccess(context.Background(), threadID, userID)
	if err != nil {
		return nil, err
	}
	perms := access.Permissions

	// Cache in memory for future use
	s.cacheManager.SetUserPermissions(threadID, userID, perms)
	return perms, nil
}

// GrantOrUpdateAccess grants or updates user access using unified method
// Handles all scenarios: thread creator (invitedBy="self"), invitation join, and direct join
// Validates that thread is not completed before granting access
func (s *ThreadAccessService) GrantOrUpdateAccess(
	threadID, userID string,
	role string,
	permissions []string,
	invitedBy string,
	thread *models.Thread,
) error {
	// Validate thread is not completed
	if thread.Status == "completed" || thread.Status == "failed" || thread.Status == "cancelled" {
		return fmt.Errorf("cannot join thread with status: %s", thread.Status)
	}

	// Write to Valkey using unified Lua script
	// Pass nil for threadData/threadTTL (not creating thread here, only managing access)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.accessRepo.GrantOrUpdateAccess(
		ctx,
		threadID, userID,
		role,
		permissions,
		invitedBy,
		s.luaScripts,
		nil, // threadData - not creating thread
		nil, // threadTTL - not creating thread
	)
	if err != nil {
		return fmt.Errorf("failed to grant/update access: %w", err)
	}

	// Update in-memory cache
	s.cacheManager.SetUserRole(threadID, userID, role)
	s.cacheManager.SetUserPermissions(threadID, userID, permissions)
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
	// Check if user is thread owner (implicit full access)
	if thread.OwnerID == userID {
		return true, nil
	}

	// Get user permissions (with three-tier caching)
	permissions, err := s.GetUserPermissions(threadID, userID)
	if err != nil {
		// No permissions found - access denied
		return false, nil
	}

	// Check if required permission exists
	for _, perm := range permissions {
		if perm == requiredPermission {
			return true, nil
		}
	}

	return false, nil
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

// ClearThreadCache clears all cached permissions and roles for a thread
// This should be called when a thread is completed or deleted
func (s *ThreadAccessService) ClearThreadCache(threadID string) {
	s.cacheManager.ClearThreadPermissions(threadID)
}
