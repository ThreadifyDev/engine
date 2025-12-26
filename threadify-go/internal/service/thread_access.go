package service

import (
	"context"
	"fmt"

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
	threadRepo   *valkey.ThreadRepository
	cacheManager interfaces.CacheManager
}

// NewThreadAccessService creates a new thread access service
func NewThreadAccessService(threadRepo *valkey.ThreadRepository, cacheManager interfaces.CacheManager) *ThreadAccessService {
	return &ThreadAccessService{
		threadRepo:   threadRepo,
		cacheManager: cacheManager,
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
	perms, err := s.threadRepo.GetUserPermissions(context.Background(), threadID, userID)
	if err != nil {
		return nil, err
	}

	// Cache in memory for future use
	s.cacheManager.SetUserPermissions(threadID, userID, perms)
	return perms, nil
}

// SetUserPermissions stores permissions with write-through caching
// Writes to Valkey first (durability), then updates in-memory cache (performance)
func (s *ThreadAccessService) SetUserPermissions(threadID, userID string, permissions []string) error {
	// Write to Valkey first (durability)
	err := s.threadRepo.SetUserPermissions(context.Background(), threadID, userID, permissions)
	if err != nil {
		return fmt.Errorf("failed to store permissions in Valkey: %w", err)
	}

	// Update in-memory cache (performance)
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
	role, err := s.threadRepo.GetUserRole(context.Background(), threadID, userID)
	if err != nil {
		return "", err
	}

	// Cache in memory for future use (only if role exists)
	if role != "" {
		s.cacheManager.SetUserRole(threadID, userID, role)
	}
	return role, nil
}

// AssignRole assigns role with write-through caching
// Writes to Valkey first (durability), then updates in-memory cache (performance)
func (s *ThreadAccessService) AssignRole(threadID, role, userID string) error {
	// Write to Valkey first (durability)
	err := s.threadRepo.AssignRole(context.Background(), threadID, role, userID)
	if err != nil {
		return fmt.Errorf("failed to assign role in Valkey: %w", err)
	}

	// Update in-memory cache (performance)
	s.cacheManager.SetUserRole(threadID, userID, role)
	return nil
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
	// Note: ownerID is derived from API key, so all services with same API key are owners
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
