package utils

import (
	"context"
	"time"

	"github.com/threadify/engine/internal/config"
)

// ContextWithDefaultTimeout creates a context with the default operation timeout from config
func ContextWithDefaultTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.DefaultOperationSeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}

// ContextWithValidationTimeout creates a context with validation-specific timeout
func ContextWithValidationTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.ValidationSeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}

// ContextWithDatabaseTimeout creates a context with database query timeout
func ContextWithDatabaseTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.DatabaseQuerySeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}

// ContextWithRedisTimeout creates a context with Redis operation timeout
func ContextWithRedisTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.RedisOperationSeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}

// ContextWithNATSTimeout creates a context with NATS publish timeout
func ContextWithNATSTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.NatsPublishSeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}

// ContextWithArchivalTimeout creates a context with archival operation timeout
func ContextWithArchivalTimeout(cfg *config.Config) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cfg.Timeouts.ArchivalOperationSeconds) * time.Second
	return context.WithTimeout(context.Background(), timeout)
}
