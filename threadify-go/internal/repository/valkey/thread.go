package valkey

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ThreadRepository handles thread storage in Valkey (Redis)
type ThreadRepository struct {
	valkey interfaces.ValkeyClient
	ttl    int // TTL in seconds
}

// NewThreadRepository creates a new thread repository
func NewThreadRepository(valkey interfaces.ValkeyClient, ttl int) *ThreadRepository {
	return &ThreadRepository{
		valkey: valkey,
		ttl:    ttl,
	}
}

// Save stores a thread in Valkey
func (r *ThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	key := r.getThreadKey(thread.ID)

	// Serialize thread to JSON
	data, err := thread.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize thread: %w", err)
	}

	// Store in Valkey with TTL
	err = r.valkey.Set(ctx, key, string(data), time.Duration(r.ttl)*time.Second)
	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	return nil
}

// Get retrieves a thread from Valkey
func (r *ThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	key := r.getThreadKey(threadID)

	// Get from Valkey
	data, err := r.valkey.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread: %w", err)
	}

	if data == "" {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}

	// Deserialize thread
	thread, err := models.FromJSON([]byte(data))
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize thread: %w", err)
	}

	return thread, nil
}

// Delete removes a thread from Valkey
func (r *ThreadRepository) Delete(ctx context.Context, threadID string) error {
	key := r.getThreadKey(threadID)

	err := r.valkey.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete thread: %w", err)
	}

	return nil
}

// Exists checks if a thread exists in Valkey
func (r *ThreadRepository) Exists(ctx context.Context, threadID string) (bool, error) {
	key := r.getThreadKey(threadID)

	exists, err := r.valkey.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to check thread existence: %w", err)
	}

	return exists, nil
}

// GetByOwner retrieves all thread IDs for a given owner
func (r *ThreadRepository) GetByOwner(ctx context.Context, ownerID string) ([]string, error) {
	pattern := r.getOwnerThreadPattern(ownerID)

	keys, err := r.valkey.Keys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to get threads by owner: %w", err)
	}

	// Extract thread IDs from keys
	threadIDs := make([]string, 0, len(keys))
	prefix := "thread:"
	for _, key := range keys {
		if len(key) > len(prefix) {
			threadID := key[len(prefix):]
			threadIDs = append(threadIDs, threadID)
		}
	}

	return threadIDs, nil
}

// ExtendTTL extends the TTL of a thread
func (r *ThreadRepository) ExtendTTL(ctx context.Context, threadID string) error {
	key := r.getThreadKey(threadID)

	err := r.valkey.Expire(ctx, key, time.Duration(r.ttl)*time.Second)
	if err != nil {
		return fmt.Errorf("failed to extend thread TTL: %w", err)
	}

	return nil
}

// getThreadKey generates the Redis key for a thread
func (r *ThreadRepository) getThreadKey(threadID string) string {
	return fmt.Sprintf("thread:%s", threadID)
}

// getOwnerThreadPattern generates the Redis key pattern for owner's threads
func (r *ThreadRepository) getOwnerThreadPattern(ownerID string) string {
	return fmt.Sprintf("thread:*")
}
