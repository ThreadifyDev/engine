package service

import (
	"fmt"
	"log"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/spf13/viper"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// CacheService implements the CacheManager interface with LRU in-memory caching
type CacheService struct {
	contractCache              *lru.Cache[string, *models.ContractGraph]
	threadCache                *lru.Cache[string, *models.Thread]
	runtimeRolePermissionCache *lru.Cache[string, []string] // "runtime_role" -> permissions (global)
	roleCache                  *lru.Cache[string, string]   // "threadID:userID" -> role
	stepStatusCache            *lru.Cache[string, string]   // "threadID:stepName:idempotencyKey" -> status
}

// NewCacheService creates a new cache service with LRU eviction
// Cache sizes are configurable via config.yaml under cache.lru
func NewCacheService() interfaces.CacheManager {
	// Read cache sizes from config with sensible defaults
	contractCacheSize := viper.GetInt("cache.lru.contract_cache_size")
	if contractCacheSize == 0 {
		contractCacheSize = 1000
	}

	threadCacheSize := viper.GetInt("cache.lru.thread_cache_size")
	if threadCacheSize == 0 {
		threadCacheSize = 10000
	}

	roleCacheSize := viper.GetInt("cache.lru.role_cache_size")
	if roleCacheSize == 0 {
		roleCacheSize = 50000
	}

	stepStatusCacheSize := viper.GetInt("cache.lru.step_status_cache_size")
	if stepStatusCacheSize == 0 {
		stepStatusCacheSize = 100000
	}

	// Create LRU caches with configured size limits
	contractCache, err := lru.New[string, *models.ContractGraph](contractCacheSize)
	if err != nil {
		log.Fatalf("Failed to create contract cache: %v", err)
	}

	threadCache, err := lru.New[string, *models.Thread](threadCacheSize)
	if err != nil {
		log.Fatalf("Failed to create thread cache: %v", err)
	}

	// Runtime role permission cache - only ~5 entries (owner, participant, observer, external, etc.)
	runtimeRolePermissionCache, err := lru.New[string, []string](10)
	if err != nil {
		log.Fatalf("Failed to create runtime role permission cache: %v", err)
	}

	roleCache, err := lru.New[string, string](roleCacheSize)
	if err != nil {
		log.Fatalf("Failed to create role cache: %v", err)
	}

	stepStatusCache, err := lru.New[string, string](stepStatusCacheSize)
	if err != nil {
		log.Fatalf("Failed to create step status cache: %v", err)
	}

	log.Printf("[CACHE] Initialized LRU caches: contracts=%d, threads=%d, roles=%d, stepStatus=%d",
		contractCacheSize, threadCacheSize, roleCacheSize, stepStatusCacheSize)

	return &CacheService{
		contractCache:              contractCache,
		threadCache:                threadCache,
		runtimeRolePermissionCache: runtimeRolePermissionCache,
		roleCache:                  roleCache,
		stepStatusCache:            stepStatusCache,
	}
}

// GetThread retrieves a thread from cache
func (c *CacheService) GetThread(threadID string) (*models.Thread, bool) {
	return c.threadCache.Get(threadID)
}

// SetThread stores a thread in cache
func (c *CacheService) SetThread(threadID string, thread *models.Thread) {
	c.threadCache.Add(threadID, thread)
}

// GetContractGraph retrieves a contract graph from cache
func (c *CacheService) GetContractGraph(contractID string, version int, ownerID string) (*models.ContractGraph, bool) {
	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	return c.contractCache.Get(cacheKey)
}

// SetContractGraph stores a contract graph in cache
func (c *CacheService) SetContractGraph(contractID string, version int, ownerID string, graph *models.ContractGraph) {
	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	c.contractCache.Add(cacheKey, graph)
}

// ClearContractCache removes a contract from cache
func (c *CacheService) ClearContractCache(contractID string, version int, ownerID string) {
	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	c.contractCache.Remove(cacheKey)
}

// ClearThreadCache removes a thread from cache
func (c *CacheService) ClearThreadCache(threadID string) {
	c.threadCache.Remove(threadID)
}

// GetRuntimeRolePermissions retrieves permissions for a runtime_role from cache
func (c *CacheService) GetRuntimeRolePermissions(runtimeRole string) ([]string, bool) {
	return c.runtimeRolePermissionCache.Get(runtimeRole)
}

// SetRuntimeRolePermissions stores permissions for a runtime_role in cache
func (c *CacheService) SetRuntimeRolePermissions(runtimeRole string, permissions []string) {
	c.runtimeRolePermissionCache.Add(runtimeRole, permissions)
}

// GetUserRole retrieves role from in-memory cache
func (c *CacheService) GetUserRole(threadID, userID string) (string, bool) {
	key := threadID + ":" + userID
	return c.roleCache.Get(key)
}

// SetUserRole stores role in in-memory cache
func (c *CacheService) SetUserRole(threadID, userID, role string) {
	key := threadID + ":" + userID
	c.roleCache.Add(key, role)
}

// ClearThreadRoles removes all roles for a thread
func (c *CacheService) ClearThreadRoles(threadID string) {
	// Remove all role entries starting with threadID
	prefix := threadID + ":"

	// LRU cache doesn't support prefix deletion, so we iterate through keys
	for _, key := range c.roleCache.Keys() {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			c.roleCache.Remove(key)
		}
	}
}

// GetStepStatus retrieves a step status from cache
// Returns status and true if found, empty string and false otherwise
func (c *CacheService) GetStepStatus(stepHashKey string) (string, bool) {
	return c.stepStatusCache.Get(stepHashKey)
}

// SetStepStatus stores a step status in cache for duplicate detection
func (c *CacheService) SetStepStatus(stepHashKey, status string) {
	c.stepStatusCache.Add(stepHashKey, status)
}

// ClearStepStatus removes a step from cache
func (c *CacheService) ClearStepStatus(stepHashKey string) {
	c.stepStatusCache.Remove(stepHashKey)
}
