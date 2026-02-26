package utils

import (
	"context"
	"time"

	"github.com/threadify/engine/internal/config"
)

// ContextWithDefaultTimeout creates a context with the default operation timeout from config.
// Parent context is propagated to maintain the context chain.
func ContextWithDefaultTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.DefaultOperationSeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}

// ContextWithValidationTimeout creates a context with validation-specific timeout.
func ContextWithValidationTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.ValidationSeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}

// ContextWithDatabaseTimeout creates a context with database query timeout.
func ContextWithDatabaseTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.DatabaseQuerySeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}

// ContextWithRedisTimeout creates a context with Redis operation timeout.
func ContextWithRedisTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.RedisOperationSeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}

// ContextWithNATSTimeout creates a context with NATS publish timeout.
func ContextWithNATSTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.NatsPublishSeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}

// ContextWithArchivalTimeout creates a context with archival operation timeout.
func ContextWithArchivalTimeout(parent context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.ArchivalOperationSeconds) * time.Second
	return context.WithTimeout(parent, timeout)
}
