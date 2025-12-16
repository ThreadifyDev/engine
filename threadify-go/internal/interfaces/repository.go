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
