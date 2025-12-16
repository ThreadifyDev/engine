package service

import (
	"fmt"
	"sync"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// CacheService implements the CacheManager interface with in-memory caching
type CacheService struct {
	contractCache map[string]*models.ContractGraph
	threadCache   map[string]*models.Thread
	mu            sync.RWMutex // Thread safety for concurrent access
}

// NewCacheService creates a new cache service
func NewCacheService() interfaces.CacheManager {
	return &CacheService{
		contractCache: make(map[string]*models.ContractGraph),
		threadCache:   make(map[string]*models.Thread),
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
func (c *CacheService) GetContractGraph(contractID string, version int) (*models.ContractGraph, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cacheKey := fmt.Sprintf("%s:v%d", contractID, version)
	graph, exists := c.contractCache[cacheKey]
	return graph, exists
}

// SetContractGraph stores a contract graph in cache
func (c *CacheService) SetContractGraph(contractID string, version int, graph *models.ContractGraph) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheKey := fmt.Sprintf("%s:v%d", contractID, version)
	c.contractCache[cacheKey] = graph
}

// ClearThreadCache removes a thread from cache
func (c *CacheService) ClearThreadCache(threadID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.threadCache, threadID)
}

// ClearContractCache removes a contract graph from cache
func (c *CacheService) ClearContractCache(contractID string, version int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cacheKey := fmt.Sprintf("%s:v%d", contractID, version)
	delete(c.contractCache, cacheKey)
}
