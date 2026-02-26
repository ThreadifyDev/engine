package service

import (
	"fmt"
	"strings"

	"go.uber.org/zap"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// CacheService implements the CacheManager interface with LRU in-memory caching.
type CacheService struct {
	contractCache              *lru.Cache[string, *models.ContractGraph]
	threadCache                *lru.Cache[string, *models.Thread]
	runtimeRolePermissionCache *lru.Cache[string, []string] // runtime_role -> permissions (global)
	roleCache                  *lru.Cache[string, string]   // "threadID:userID" -> role
	stepStatusCache            *lru.Cache[string, string]   // "threadID:stepName:idempotencyKey" -> status
	logger                     *zap.Logger
}

// NewCacheService creates a new cache service with LRU eviction.
// Cache sizes are configurable via config.yaml under cache.lru.
func NewCacheService(logger *zap.Logger) interfaces.CacheManager {
	viper.SetDefault("cache.lru.contract_cache_size", 1000)
	viper.SetDefault("cache.lru.thread_cache_size", 10000)
	viper.SetDefault("cache.lru.role_cache_size", 50000)
	viper.SetDefault("cache.lru.step_status_cache_size", 100000)

	contractCacheSize := viper.GetInt("cache.lru.contract_cache_size")
	threadCacheSize := viper.GetInt("cache.lru.thread_cache_size")
	roleCacheSize := viper.GetInt("cache.lru.role_cache_size")
	stepStatusCacheSize := viper.GetInt("cache.lru.step_status_cache_size")

	contractCache := mustNewLRU[string, *models.ContractGraph](contractCacheSize, "contract", logger)
	threadCache := mustNewLRU[string, *models.Thread](threadCacheSize, "thread", logger)
	// Runtime role permission cache — only ~5 entries (owner, participant, observer, external, etc.)
	runtimeRolePermissionCache := mustNewLRU[string, []string](10, "runtime_role_permission", logger)
	roleCache := mustNewLRU[string, string](roleCacheSize, "role", logger)
	stepStatusCache := mustNewLRU[string, string](stepStatusCacheSize, "step_status", logger)

	logger.Info("initialized LRU caches",
		zap.Int("contracts", contractCacheSize),
		zap.Int("threads", threadCacheSize),
		zap.Int("roles", roleCacheSize),
		zap.Int("step_status", stepStatusCacheSize),
	)

	return &CacheService{
		contractCache:              contractCache,
		threadCache:                threadCache,
		runtimeRolePermissionCache: runtimeRolePermissionCache,
		roleCache:                  roleCache,
		stepStatusCache:            stepStatusCache,
		logger:                     logger,
	}
}

// mustNewLRU creates an LRU cache, fatally logging if construction fails.
func mustNewLRU[K comparable, V any](size int, name string, logger *zap.Logger) *lru.Cache[K, V] {
	cache, err := lru.New[K, V](size)
	if err != nil {
		logger.Fatal("failed to create LRU cache", zap.String("cache", name), zap.Error(err))
	}
	return cache
}

// contractCacheKey returns the canonical cache key for a contract graph.
func contractCacheKey(ownerID, contractID string, version int) string {
	return fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
}

// GetThread retrieves a thread from cache.
func (c *CacheService) GetThread(threadID string) (*models.Thread, bool) {
	return c.threadCache.Get(threadID)
}

// SetThread stores a thread in cache.
func (c *CacheService) SetThread(threadID string, thread *models.Thread) {
	c.threadCache.Add(threadID, thread)
}

// GetContractGraph retrieves a contract graph from cache.
func (c *CacheService) GetContractGraph(contractID string, version int, ownerID string) (*models.ContractGraph, bool) {
	return c.contractCache.Get(contractCacheKey(ownerID, contractID, version))
}

// SetContractGraph stores a contract graph in cache.
func (c *CacheService) SetContractGraph(contractID string, version int, ownerID string, graph *models.ContractGraph) {
	c.contractCache.Add(contractCacheKey(ownerID, contractID, version), graph)
}

// ClearContractCache removes a contract graph from cache.
func (c *CacheService) ClearContractCache(contractID string, version int, ownerID string) {
	c.contractCache.Remove(contractCacheKey(ownerID, contractID, version))
}

// ClearThreadCache removes a thread from cache.
func (c *CacheService) ClearThreadCache(threadID string) {
	c.threadCache.Remove(threadID)
}

// GetRuntimeRolePermissions retrieves permissions for a runtime_role from cache.
func (c *CacheService) GetRuntimeRolePermissions(runtimeRole string) ([]string, bool) {
	return c.runtimeRolePermissionCache.Get(runtimeRole)
}

// SetRuntimeRolePermissions stores permissions for a runtime_role in cache.
func (c *CacheService) SetRuntimeRolePermissions(runtimeRole string, permissions []string) {
	c.runtimeRolePermissionCache.Add(runtimeRole, permissions)
}

// GetUserRole retrieves a user's role for a thread from cache.
func (c *CacheService) GetUserRole(threadID, userID string) (string, bool) {
	return c.roleCache.Get(threadID + ":" + userID)
}

// SetUserRole stores a user's role for a thread in cache.
func (c *CacheService) SetUserRole(threadID, userID, role string) {
	c.roleCache.Add(threadID+":"+userID, role)
}

// ClearThreadRoles removes all cached roles for a given thread.
func (c *CacheService) ClearThreadRoles(threadID string) {
	prefix := threadID + ":"
	for _, key := range c.roleCache.Keys() {
		if strings.HasPrefix(key, prefix) {
			c.roleCache.Remove(key)
		}
	}
}

// GetStepStatus retrieves a step status from cache.
func (c *CacheService) GetStepStatus(stepHashKey string) (string, bool) {
	return c.stepStatusCache.Get(stepHashKey)
}

// SetStepStatus stores a step status in cache for duplicate detection.
func (c *CacheService) SetStepStatus(stepHashKey, status string) {
	c.stepStatusCache.Add(stepHashKey, status)
}

// ClearStepStatus removes a step from cache.
func (c *CacheService) ClearStepStatus(stepHashKey string) {
	c.stepStatusCache.Remove(stepHashKey)
}
