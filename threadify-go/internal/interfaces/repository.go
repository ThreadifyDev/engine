package interfaces

import (
	"context"
	"time"

	"github.com/threadify/engine/internal/models"
)

// ValkeyClient defines the interface for Valkey operations
type ValkeyClient interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	// Hash operations for role and permission management
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	// List operations for event queues
	LPush(ctx context.Context, key string, values ...interface{}) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// Stream operations for archiver
	XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error)
	XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]map[string]interface{}, error)
	XAck(ctx context.Context, stream, group string, ids []string) error
	XGroupCreate(ctx context.Context, stream, group, start string) error
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	// Pipeline operations
	Pipeline() ValkeyPipeline
}

// ValkeyPipeline defines the interface for Redis pipeline operations
type ValkeyPipeline interface {
	HSet(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	HDel(ctx context.Context, key string, fields ...string) ValkeyPipeline
	LPush(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	XAdd(ctx context.Context, stream string, values map[string]interface{}) ValkeyPipeline
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) ValkeyPipeline
	Del(ctx context.Context, keys ...string) ValkeyPipeline
	Expire(ctx context.Context, key string, expiration time.Duration) ValkeyPipeline
	Exec(ctx context.Context) ([]interface{}, error)
}

// ThreadRepository defines the interface for thread storage operations
type ThreadRepository interface {
	Save(ctx context.Context, thread *models.Thread) error
	Get(ctx context.Context, threadID string) (*models.Thread, error)
	Delete(ctx context.Context, threadID string) error
	Exists(ctx context.Context, threadID string) (bool, error)
	GetByOwner(ctx context.Context, ownerID string) ([]string, error)
	ExtendTTL(ctx context.Context, threadID string) error
}

// ContractGraphRepository defines the interface for contract graph operations
type ContractGraphRepository interface {
	Get(ctx context.Context, contractID string, version int) (*models.ContractGraph, error)
	Save(ctx context.Context, contractID string, version int, graph *models.ContractGraph) error
	Delete(ctx context.Context, contractID string, version int) error
	Exists(ctx context.Context, contractID string, version int) (bool, error)
}

// Note: ValkeyClient is already properly defined in internal/repository/valkey/thread.go
// We should reuse that instead of redefining it here
