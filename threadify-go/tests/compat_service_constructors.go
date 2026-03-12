package tests

import (
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/service"
	"go.uber.org/zap"
)

func NewCacheService(logger *zap.Logger) interfaces.CacheManager {
	return service.NewCacheService(logger)
}

func NewGraphBuilder() *service.GraphBuilder {
	return service.NewGraphBuilder()
}
