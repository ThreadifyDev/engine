package tests

import (
	"testing"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/valkey"
)

// TestThreadAccessService_SetAndGetPermissions tests permission storage and retrieval
func TestThreadAccessService_SetAndGetPermissions(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	threadID := "thread-123"
	userID := "user-456"
	permissions := []string{"read", "write"}

	// Test: Set permissions
	err := accessService.SetUserPermissions(threadID, userID, permissions)
	if err != nil {
		t.Fatalf("SetUserPermissions failed: %v", err)
	}

	// Test: Get permissions (should hit in-memory cache)
	perms, err := accessService.GetUserPermissions(threadID, userID)
	if err != nil {
		t.Fatalf("GetUserPermissions failed: %v", err)
	}

	if len(perms) != 2 || perms[0] != "read" || perms[1] != "write" {
		t.Errorf("Expected permissions [read, write], got %v", perms)
	}
}

// TestThreadAccessService_SetAndGetRole tests role storage and retrieval
func TestThreadAccessService_SetAndGetRole(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	threadID := "thread-123"
	userID := "user-456"
	role := "buyer"

	// Test: Assign role
	err := accessService.AssignRole(threadID, role, userID)
	if err != nil {
		t.Fatalf("AssignRole failed: %v", err)
	}

	// Test: Get role (should hit in-memory cache)
	retrievedRole, err := accessService.GetUserRole(threadID, userID)
	if err != nil {
		t.Fatalf("GetUserRole failed: %v", err)
	}

	if retrievedRole != role {
		t.Errorf("Expected role %s, got %s", role, retrievedRole)
	}
}

// TestThreadAccessService_CheckThreadAccess_Owner tests owner access
func TestThreadAccessService_CheckThreadAccess_Owner(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	thread := &models.Thread{
		ID:      "thread-123",
		OwnerID: "user-456",
	}

	// Test: Owner should have access without explicit permissions
	hasAccess, err := accessService.CheckThreadAccess(thread.ID, "user-456", "write", thread)
	if err != nil {
		t.Fatalf("CheckThreadAccess failed: %v", err)
	}

	if !hasAccess {
		t.Error("Expected owner to have access")
	}
}

// TestThreadAccessService_CheckThreadAccess_InvitedUser tests invited user access
func TestThreadAccessService_CheckThreadAccess_InvitedUser(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	thread := &models.Thread{
		ID:      "thread-123",
		OwnerID: "user-owner",
	}

	invitedUserID := "user-invited"

	// Set permissions for invited user
	err := accessService.SetUserPermissions(thread.ID, invitedUserID, []string{"read", "write"})
	if err != nil {
		t.Fatalf("SetUserPermissions failed: %v", err)
	}

	// Test: Invited user with write permission should have access
	hasAccess, err := accessService.CheckThreadAccess(thread.ID, invitedUserID, "write", thread)
	if err != nil {
		t.Fatalf("CheckThreadAccess failed: %v", err)
	}

	if !hasAccess {
		t.Error("Expected invited user with write permission to have access")
	}
}

// TestThreadAccessService_CheckThreadAccess_NoPermission tests denied access
func TestThreadAccessService_CheckThreadAccess_NoPermission(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	thread := &models.Thread{
		ID:      "thread-123",
		OwnerID: "user-owner",
	}

	unauthorizedUserID := "user-unauthorized"

	// Test: User without permissions should be denied
	hasAccess, err := accessService.CheckThreadAccess(thread.ID, unauthorizedUserID, "write", thread)
	if err != nil {
		t.Fatalf("CheckThreadAccess failed: %v", err)
	}

	if hasAccess {
		t.Error("Expected unauthorized user to be denied access")
	}
}

// TestThreadAccessService_CheckThreadAccess_ReadOnlyUser tests read-only access
func TestThreadAccessService_CheckThreadAccess_ReadOnlyUser(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	thread := &models.Thread{
		ID:      "thread-123",
		OwnerID: "user-owner",
	}

	readOnlyUserID := "user-readonly"

	// Set read-only permissions
	err := accessService.SetUserPermissions(thread.ID, readOnlyUserID, []string{"read"})
	if err != nil {
		t.Fatalf("SetUserPermissions failed: %v", err)
	}

	// Test: Read-only user should not have write access
	hasAccess, err := accessService.CheckThreadAccess(thread.ID, readOnlyUserID, "write", thread)
	if err != nil {
		t.Fatalf("CheckThreadAccess failed: %v", err)
	}

	if hasAccess {
		t.Error("Expected read-only user to be denied write access")
	}

	// Test: Read-only user should have read access
	hasReadAccess, err := accessService.CheckThreadAccess(thread.ID, readOnlyUserID, "read", thread)
	if err != nil {
		t.Fatalf("CheckThreadAccess failed: %v", err)
	}

	if !hasReadAccess {
		t.Error("Expected read-only user to have read access")
	}
}

// TestThreadAccessService_ValidateUserRoleForStep tests role validation
func TestThreadAccessService_ValidateUserRoleForStep(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	threadID := "thread-123"
	userID := "user-456"
	role := "buyer"

	// Assign role
	err := accessService.AssignRole(threadID, role, userID)
	if err != nil {
		t.Fatalf("AssignRole failed: %v", err)
	}

	// Test: User with correct role should pass validation
	hasRole, err := accessService.ValidateUserRoleForStep(threadID, userID, "buyer")
	if err != nil {
		t.Fatalf("ValidateUserRoleForStep failed: %v", err)
	}

	if !hasRole {
		t.Error("Expected user with buyer role to pass validation")
	}

	// Test: User with wrong role should fail validation
	hasWrongRole, err := accessService.ValidateUserRoleForStep(threadID, userID, "seller")
	if err != nil {
		t.Fatalf("ValidateUserRoleForStep failed: %v", err)
	}

	if hasWrongRole {
		t.Error("Expected user with buyer role to fail seller validation")
	}
}

// TestThreadAccessService_CacheHit tests in-memory cache hit
func TestThreadAccessService_CacheHit(t *testing.T) {
	// Setup
	mockValkey := NewMockValkeyClient()
	threadRepo := valkey.NewThreadRepository(mockValkey, 3600)
	cacheService := NewCacheService()
	accessService := NewThreadAccessService(threadRepo, cacheService)

	threadID := "thread-123"
	userID := "user-456"
	permissions := []string{"read", "write"}

	// Set permissions (writes to Valkey and cache)
	err := accessService.SetUserPermissions(threadID, userID, permissions)
	if err != nil {
		t.Fatalf("SetUserPermissions failed: %v", err)
	}

	// Clear Valkey to test cache hit
	mockValkey.hash = make(map[string]map[string]string)

	// Get permissions (should hit cache, not Valkey)
	perms, err := accessService.GetUserPermissions(threadID, userID)
	if err != nil {
		t.Fatalf("GetUserPermissions failed: %v", err)
	}

	if len(perms) != 2 {
		t.Error("Expected cache hit to return permissions even after Valkey cleared")
	}
}
