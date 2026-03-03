package tests

import (
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/service"
)

// Legacy constructor aliases kept for tests that still call package-level helpers.
func NewCacheService() interfaces.CacheManager {
	return service.NewCacheService()
}

func NewGraphBuilder() *service.GraphBuilder {
	return service.NewGraphBuilder()
}
