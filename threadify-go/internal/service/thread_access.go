package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"threadify-go/shared/rbac"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/metrics"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
	"github.com/threadify/engine/internal/repository/valkey"
	"go.uber.org/zap"
)

// ThreadAccessService centralises permission and role management with three-tier caching:
// in-memory (mutex-protected) → Valkey → PostgreSQL.
// Permissions are resolved from runtime_role via the RBAC loader (no per-user storage needed).
type ThreadAccessService struct {
	accessRepo   *valkey.AccessRepository
	cacheManager interfaces.CacheManager
	luaScripts   *valkey.LuaScriptManager
	rbacLoader   *rbac.Loader
	logger       *zap.Logger
}

// NewThreadAccessService creates a new thread access service.
func NewThreadAccessService(
	accessRepo *valkey.AccessRepository,
	cacheManager interfaces.CacheManager,
	luaScripts *valkey.LuaScriptManager,
	rbacLoader *rbac.Loader,
	logger *zap.Logger,
) *ThreadAccessService {
	return &ThreadAccessService{
		accessRepo:   accessRepo,
		cacheManager: cacheManager,
		luaScripts:   luaScripts,
		rbacLoader:   rbacLoader,
		logger:       logger,
	}
}

// GetUserPermissions resolves permissions for a user in a thread:
//  1. Get user's runtime_role from Valkey
//  2. Check global runtime_role permission cache
//  3. On cache miss, resolve from RBAC loader and cache by runtime_role
func (s *ThreadAccessService) GetUserPermissions(ctx context.Context, threadID, userID string) ([]string, error) {
	start := perf.Now()

	access, err := s.accessRepo.GetUserAccess(ctx, threadID, userID)
	if err != nil {
		return nil, err
	}

	cacheCheckStart := perf.Now()
	if perms, exists := s.cacheManager.GetRuntimeRolePermissions(access.RuntimeRole); exists {
		perf.LogStructured("GetUserPermissions.cacheHit",
			zap.Duration("duration", perf.Since(cacheCheckStart)),
			zap.String("runtime_role", access.RuntimeRole),
		)
		return perms, nil
	}

	perms, err := s.resolveAndCachePermissions(access.RuntimeRole)
	if err != nil {
		return nil, err
	}

	perf.LogStructured("GetUserPermissions total",
		zap.Duration("duration", perf.Since(start)),
		zap.String("source", "rbac"),
	)
	return perms, nil
}

// GrantOrUpdateAccess grants or updates user access to a thread.
// The first entry in roles is used as the primary role; multi-role support is planned.
func (s *ThreadAccessService) GrantOrUpdateAccess(
	ctx context.Context,
	threadID, userID string,
	roles []string,
	runtimeRole, invitedBy string,
) error {
	if len(roles) == 0 {
		return fmt.Errorf("at least one role is required")
	}
	role := roles[0]

	permissions, _ := s.resolveAndCachePermissions(runtimeRole)

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := s.accessRepo.GrantOrUpdateAccess(
		timeoutCtx,
		threadID, userID, role, runtimeRole, permissions, invitedBy,
		s.luaScripts, nil, nil,
	); err != nil {
		return fmt.Errorf("failed to grant/update access: %w", err)
	}

	s.cacheManager.SetUserRole(threadID, userID, role)
	return nil
}

// GrantAccessWithThreadCreation atomically creates a thread and grants creator access.
// Permissions are resolved from runtime_role before storing.
func (s *ThreadAccessService) GrantAccessWithThreadCreation(
	ctx context.Context,
	threadID, userID, role, runtimeRole string,
	threadData *string,
	threadTTL *int,
) (*interfaces.UserAccess, error) {
	rbacStart := perf.Now()
	permissions, _ := s.resolveAndCachePermissions(runtimeRole)
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "rbac_permissions").Observe(perf.Since(rbacStart).Seconds())

	repoStart := perf.Now()
	access, err := s.accessRepo.GrantOrUpdateAccess(
		ctx,
		threadID, userID, role, runtimeRole, permissions,
		"self", // invitedBy for creator
		s.luaScripts, threadData, threadTTL,
	)
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "lua_script_exec").Observe(perf.Since(repoStart).Seconds())
	if err != nil {
		return nil, fmt.Errorf("failed to grant access with thread creation: %w", err)
	}

	cacheStart := perf.Now()
	s.cacheManager.SetUserRole(threadID, userID, role)
	metrics.OperationDuration.WithLabelValues("redis_thread_create", "cache_update").Observe(perf.Since(cacheStart).Seconds())

	return access, nil
}

// GetUserRole retrieves a user's role with two-tier caching:
// Tier 1: in-memory cache (~0.1ms), Tier 2: Valkey (~2–5ms).
func (s *ThreadAccessService) GetUserRole(ctx context.Context, threadID, userID string) (string, error) {
	if role, exists := s.cacheManager.GetUserRole(threadID, userID); exists {
		return role, nil
	}

	access, err := s.accessRepo.GetUserAccess(ctx, threadID, userID)
	if err != nil {
		return "", err
	}

	role := ""
	if len(access.Roles) > 0 {
		role = access.Roles[0]
	}
	if role != "" {
		s.cacheManager.SetUserRole(threadID, userID, role)
	}
	return role, nil
}

// CheckThreadAccess reports whether a user has the required permission for a thread.
// Thread owners always have implicit full access.
func (s *ThreadAccessService) CheckThreadAccess(ctx context.Context, threadID, userID, requiredPermission string, thread *models.Thread) (bool, error) {
	start := perf.Now()
	defer func() {
		perf.LogStructured("CheckThreadAccess total",
			zap.Duration("duration", perf.Since(start)),
			zap.String("thread_id", threadID),
			zap.String("user_id", userID),
		)
	}()

	if thread.OwnerID == userID {
		return true, nil
	}

	permissions, err := s.GetUserPermissions(ctx, threadID, userID)
	if err != nil {
		return false, nil
	}

	return slices.Contains(permissions, requiredPermission), nil
}

// BatchCheckThreadAccess checks access for multiple threads in a single pass.
// Returns a map of threadID → hasAccess.
func (s *ThreadAccessService) BatchCheckThreadAccess(ctx context.Context, threads []*models.Thread, userID, requiredPermission string) (map[string]bool, error) {
	result := make(map[string]bool, len(threads))
	if len(threads) == 0 {
		return result, nil
	}

	var nonOwned []*models.Thread
	for _, thread := range threads {
		if thread.OwnerID == userID {
			result[thread.ID] = true
		} else {
			nonOwned = append(nonOwned, thread)
		}
	}

	for _, thread := range nonOwned {
		permissions, err := s.GetUserPermissions(ctx, thread.ID, userID)
		if err != nil {
			result[thread.ID] = false
			continue
		}
		result[thread.ID] = slices.Contains(permissions, requiredPermission)
	}

	return result, nil
}

// ValidateUserRoleForStep reports whether a user holds the required role for a contract step.
func (s *ThreadAccessService) ValidateUserRoleForStep(ctx context.Context, threadID, userID, requiredRole string) (bool, error) {
	userRole, err := s.GetUserRole(ctx, threadID, userID)
	if err != nil {
		return false, err
	}
	return userRole == requiredRole, nil
}

// ClearThreadCache removes all cached roles for a thread.
// Should be called when a thread is completed or deleted.
// Note: permissions are cached globally by runtime_role, not per-thread.
func (s *ThreadAccessService) ClearThreadCache(threadID string) {
	s.cacheManager.ClearThreadRoles(threadID)
}

// GetUserIDsByRuntimeRoles retrieves user IDs for the given runtime_roles,
// falling back to PostgreSQL on a complete Valkey cache miss.
func (s *ThreadAccessService) GetUserIDsByRuntimeRoles(
	ctx context.Context,
	threadID string,
	runtimeRoles []string,
) ([]string, error) {
	userIDs, err := s.accessRepo.GetUserIDsByRuntimeRoles(ctx, threadID, runtimeRoles)
	if err != nil {
		return nil, fmt.Errorf("failed to get users from redis: %w", err)
	}
	if len(userIDs) == 0 {
		return s.accessRepo.PopulateRoleSetsFromPostgres(ctx, threadID, runtimeRoles)
	}
	return userIDs, nil
}

// GetUsersByPermissions returns users who hold any of the required permissions,
// queried via PostgreSQL (GIN-indexed). Results are not cached in Valkey since
// the permission→userIDs mapping changes frequently.
func (s *ThreadAccessService) GetUsersByPermissions(
	ctx context.Context,
	threadID string,
	requiredPermissions []string,
) ([]models.UserPermissionInfo, error) {
	return s.accessRepo.GetUsersByPermissions(ctx, threadID, requiredPermissions)
}

// resolveAndCachePermissions resolves permissions for a runtime_role via the RBAC loader
// and stores the result in the global runtime_role permission cache.
// Returns an empty slice (not an error) if the RBAC loader is unavailable.
func (s *ThreadAccessService) resolveAndCachePermissions(runtimeRole string) ([]string, error) {
	if s.rbacLoader == nil {
		return []string{}, nil
	}

	rbacStart := perf.Now()
	perms := s.rbacLoader.GetPermissionsForRoles([]string{runtimeRole}, "runtime_level")
	perf.LogStructured("GetUserPermissions.rbacResolve",
		zap.Duration("duration", perf.Since(rbacStart)),
		zap.String("runtime_role", runtimeRole),
	)

	s.cacheManager.SetRuntimeRolePermissions(runtimeRole, perms)
	return perms, nil
}
