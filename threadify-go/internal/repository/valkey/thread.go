package valkey

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	apperrors "github.com/threadify/engine/internal/utils/errors"
)

// ThreadRepository handles thread storage in Valkey (Redis)
type ThreadRepository struct {
	valkey       interfaces.ValkeyClient
	ttl          int // TTL in seconds
	postgresRepo *postgres.ThreadRepository
}

// NewThreadRepository creates a new thread repository
func NewThreadRepository(valkey interfaces.ValkeyClient, ttl int) *ThreadRepository {
	return &ThreadRepository{
		valkey: valkey,
		ttl:    ttl,
	}
}

// NewThreadRepositoryWithPostgres creates a new thread repository with PostgreSQL fallback
func NewThreadRepositoryWithPostgres(valkey interfaces.ValkeyClient, ttl int, postgresRepo *postgres.ThreadRepository) *ThreadRepository {
	return &ThreadRepository{
		valkey:       valkey,
		ttl:          ttl,
		postgresRepo: postgresRepo,
	}
}

// Save stores a thread in Valkey
func (r *ThreadRepository) Save(ctx context.Context, thread *models.Thread) error {
	key := r.getThreadKey(thread.ID)
	metaKey := r.getThreadMetaKey(thread.ID)

	// Serialize thread to JSON
	data, err := thread.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to serialize thread: %w", err)
	}

	// Use pipeline for atomic write
	pipe := r.valkey.Pipeline()

	// Store base data as JSON
	pipe.Set(ctx, key, string(data), time.Duration(r.ttl)*time.Second)

	// Store metadata in hash for atomic Lua updates
	metadata := map[string]interface{}{
		"status": string(thread.Status),
	}
	if thread.CompletedAt != nil {
		metadata["completedAt"] = thread.CompletedAt.Format(time.RFC3339)
	}
	pipe.HSet(ctx, metaKey, metadata)
	pipe.Expire(ctx, metaKey, time.Duration(r.ttl)*time.Second)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to save thread: %w", err)
	}

	return nil
}

// Get retrieves a thread from Valkey
func (r *ThreadRepository) Get(ctx context.Context, threadID string) (*models.Thread, error) {
	key := r.getThreadKey(threadID)
	metaKey := r.getThreadMetaKey(threadID)

	// Get base data from JSON
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

	// Get metadata from hash and overlay it (hash is source of truth)
	meta, err := r.valkey.HGetAll(ctx, metaKey)
	if err == nil && len(meta) > 0 {
		// Initialize refs map if needed
		if thread.Refs == nil {
			thread.Refs = make(map[string]string)
		}

		// Extract and overlay metadata fields
		for key, value := range meta {
			switch key {
			case "status":
				thread.Status = models.ThreadStatus(value)
			case "completedAt":
				if value != "" {
					if completedAt, err := time.Parse(time.RFC3339, value); err == nil {
						thread.CompletedAt = &completedAt
					}
				}
			default:
				// Extract refs (keys prefixed with "refs:")
				if strings.HasPrefix(key, "refs:") {
					refKey := strings.TrimPrefix(key, "refs:")
					thread.Refs[refKey] = value
				}
			}
		}
	}

	return thread, nil
}

// GetThreadWithCache retrieves a thread using read-only cache pattern:
// 1. Try Redis first (async validator's live data)
// 2. Fallback to PostgreSQL if cache miss (inactive/archived threads)
// 3. NO write-back to prevent data conflicts with async validator
func (r *ThreadRepository) GetThreadWithCache(ctx context.Context, threadID string) (*models.Thread, error) {
	// Add timeout context for production robustness
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try Redis first (active thread data from async validator)
	if cached, err := r.Get(ctx, threadID); err == nil && cached != nil {
		log.Printf(" Cache HIT for thread %s from Redis (active thread)", threadID)
		return cached, nil
	}

	log.Printf(" Cache MISS for thread %s - falling back to PostgreSQL (inactive thread)", threadID)

	// Fallback to PostgreSQL if available (inactive/archived threads)
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("thread not found in Redis and no PostgreSQL repository configured")
	}

	thread, err := r.postgresRepo.Get(ctx, threadID)
	if err != nil {
		// Return user-friendly error without exposing database details
		return nil, apperrors.NewNotFoundError(apperrors.MsgThreadNotFound, err)
	}

	log.Printf(" Retrieved thread %s from PostgreSQL (inactive thread)", threadID)

	// NO write-back - let async validator own Redis writes to prevent data conflicts
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

// AddRefs adds references to thread metadata atomically
func (r *ThreadRepository) AddRefs(ctx context.Context, threadID string, refs map[string]string) error {
	if len(refs) == 0 {
		return nil
	}

	metaKey := r.getThreadMetaKey(threadID)
	pipe := r.valkey.Pipeline()

	// Atomic ref updates - each HSET is atomic per field
	for key, value := range refs {
		pipe.HSet(ctx, metaKey, "refs:"+key, value)
	}
	pipe.Expire(ctx, metaKey, time.Duration(r.ttl)*time.Second)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to add refs: %w", err)
	}

	return nil
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
	return fmt.Sprintf("thread:*:owner:%s", ownerID)
}

func (r *ThreadRepository) getThreadMetaKey(threadID string) string {
	return fmt.Sprintf("thread:%s:meta", threadID)
}
