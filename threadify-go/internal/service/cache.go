package service

import (
	"fmt"
	"sync"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// CacheService implements the CacheManager interface with in-memory caching
type CacheService struct {
	contractCache   map[string]*models.ContractGraph
	threadCache     map[string]*models.Thread
	permissionCache map[string][]string // "threadID:userID" -> permissions
	roleCache       map[string]string   // "threadID:userID" -> role
	mu              sync.RWMutex        // Thread safety for concurrent access
}

// NewCacheService creates a new cache service
func NewCacheService() interfaces.CacheManager {
	return &CacheService{
		contractCache:   make(map[string]*models.ContractGraph),
		threadCache:     make(map[string]*models.Thread),
		permissionCache: make(map[string][]string),
		roleCache:       make(map[string]string),
	}
}

// GetThread retrieves a thread from cache
func (c *CacheService) GetThread(threadID string) (*models.Thread, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	thread, exists := c.threadCache[threadID]
	return thread, exists
}

// SetThread stores a thread in cache
func (c *CacheService) SetThread(threadID string, thread *models.Thread) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.threadCache[threadID] = thread
}

// GetContractGraph retrieves a contract graph from cache
func (c *CacheService) GetContractGraph(contractID string, version int, ownerID string) (*models.ContractGraph, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	graph, exists := c.contractCache[cacheKey]
	return graph, exists
}

// SetContractGraph stores a contract graph in cache
func (c *CacheService) SetContractGraph(contractID string, version int, ownerID string, graph *models.ContractGraph) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	c.contractCache[cacheKey] = graph
}

// ClearContractCache removes a contract from cache
func (c *CacheService) ClearContractCache(contractID string, version int, ownerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheKey := fmt.Sprintf("%s:%s:v%d", ownerID, contractID, version)
	delete(c.contractCache, cacheKey)
}

// ClearThreadCache removes a thread from cache
func (c *CacheService) ClearThreadCache(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.threadCache, threadID)
}

// GetUserPermissions retrieves permissions from in-memory cache
func (c *CacheService) GetUserPermissions(threadID, userID string) ([]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", threadID, userID)
	perms, exists := c.permissionCache[key]
	return perms, exists
}

// SetUserPermissions stores permissions in in-memory cache
func (c *CacheService) SetUserPermissions(threadID, userID string, permissions []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%s", threadID, userID)
	c.permissionCache[key] = permissions
}

// GetUserRole retrieves role from in-memory cache
func (c *CacheService) GetUserRole(threadID, userID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", threadID, userID)
	role, exists := c.roleCache[key]
	return role, exists
}

// SetUserRole stores role in in-memory cache
func (c *CacheService) SetUserRole(threadID, userID, role string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%s", threadID, userID)
	c.roleCache[key] = role
}

// ClearThreadPermissions removes all permissions and roles for a thread
func (c *CacheService) ClearThreadPermissions(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove all entries starting with threadID
	prefix := threadID + ":"
	for key := range c.permissionCache {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.permissionCache, key)
		}
	}
	for key := range c.roleCache {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.roleCache, key)
		}
	}
}
