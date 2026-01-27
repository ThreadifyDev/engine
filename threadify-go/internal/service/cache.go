package service

import (
	"fmt"
	"log"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// CacheService implements the CacheManager interface with LRU in-memory caching
type CacheService struct {
	contractCache              *lru.Cache[string, *models.ContractGraph]
	threadCache                *lru.Cache[string, *models.Thread]
	runtimeRolePermissionCache *lru.Cache[string, []string] // "runtime_role" -> permissions (global)
	roleCache                  *lru.Cache[string, string]   // "threadID:userID" -> role
}

// NewCacheService creates a new cache service with LRU eviction
func NewCacheService() interfaces.CacheManager {
	// Create LRU caches with reasonable size limits
	contractCache, err := lru.New[string, *models.ContractGraph](1000) // 1000 contract versions
	if err != nil {
		log.Fatalf("Failed to create contract cache: %v", err)
	}

	threadCache, err := lru.New[string, *models.Thread](10000) // 10k threads
	if err != nil {
		log.Fatalf("Failed to create thread cache: %v", err)
	}

	// Runtime role permission cache - only ~5 entries (owner, participant, observer, external, etc.)
	runtimeRolePermissionCache, err := lru.New[string, []string](10)
	if err != nil {
		log.Fatalf("Failed to create runtime role permission cache: %v", err)
	}

	roleCache, err := lru.New[string, string](50000) // 50k role entries
	if err != nil {
		log.Fatalf("Failed to create role cache: %v", err)
	}

	return &CacheService{
		contractCache:              contractCache,
		threadCache:                threadCache,
		runtimeRolePermissionCache: runtimeRolePermissionCache,
		roleCache:                  roleCache,
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
	key := fmt.Sprintf("%s:%s", threadID, userID)
	return c.roleCache.Get(key)
}

// SetUserRole stores role in in-memory cache
func (c *CacheService) SetUserRole(threadID, userID, role string) {
	key := fmt.Sprintf("%s:%s", threadID, userID)
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
