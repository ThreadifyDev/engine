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
	valkey            interfaces.ValkeyClient
	ttl               int // TTL in seconds
	postgresRepo      *postgres.ThreadRepository
	stepStatePostgres *postgres.StepStateRepository // For step state queries
	cacheManager      interfaces.CacheManager       // For duplicate detection via LRU cache
}

// NewThreadRepository creates a new thread repository with PostgreSQL fallback
// PostgreSQL fallback is always required for production hot/cold architecture
func NewThreadRepository(valkey interfaces.ValkeyClient, ttl int, postgresRepo *postgres.ThreadRepository, stepStatePostgres *postgres.StepStateRepository, cacheManager interfaces.CacheManager) *ThreadRepository {
	return &ThreadRepository{
		valkey:            valkey,
		ttl:               ttl,
		postgresRepo:      postgresRepo,
		stepStatePostgres: stepStatePostgres,
		cacheManager:      cacheManager,
	}
}

// GetPostgresRepo returns the underlying postgres repository for direct queries
func (r *ThreadRepository) GetPostgresRepo() *postgres.ThreadRepository {
	return r.postgresRepo
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

// Get retrieves a thread from Valkey (hot) or PostgreSQL (cold) with optional write-back
// writeBack: if true, caches PostgreSQL data back to Valkey (defaults to false)
func (r *ThreadRepository) Get(ctx context.Context, threadID string, writeBack ...bool) (*models.Thread, error) {
	log.Printf("🔍 [REPO-DEBUG] Get called for thread: %s", threadID)

	// Default writeBack to false
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

	// Try Valkey first (hot data)
	log.Printf("🔄 [REPO-DEBUG] Trying Valkey first...")
	thread, err := r.getFromValkey(ctx, threadID)
	log.Printf("📊 [REPO-DEBUG] getFromValkey result: thread=%v, err=%v", thread != nil, err)

	if err == nil && thread != nil {
		log.Printf("✅ [HOT] Thread %s from Valkey", threadID)
		return thread, nil
	}

	// Fallback to PostgreSQL (cold data)
	log.Printf("🔄 [REPO-DEBUG] Checking if postgresRepo is nil: %v", r.postgresRepo == nil)
	if r.postgresRepo == nil {
		log.Printf("❌ [REPO-DEBUG] postgresRepo is nil, cannot fallback!")
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}

	log.Printf("⚠️ [COLD] Thread %s not in Valkey, checking PostgreSQL", threadID)

	// REUSE: GetWithRefs already exists from GraphQL implementation
	thread, err = r.postgresRepo.GetWithRefs(ctx, threadID)
	if err != nil {
		return nil, apperrors.NewNotFoundError(apperrors.MsgThreadNotFound, err)
	}

	log.Printf("✅ [COLD] Thread %s from PostgreSQL", threadID)

	// Async write-back using Save() for format consistency
	// Don't block the read operation - write-back happens in background
	if shouldWriteBack {
		go func(threadID string, thread *models.Thread) {
			// Create new context with timeout for write-back operation
			writeBackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			log.Printf("🔄 [WRITE-BACK] Async caching thread %s to Valkey", threadID)
			if err := r.Save(writeBackCtx, thread); err != nil {
				log.Printf("⚠️ Async write-back failed for thread %s: %v", threadID, err)
			} else {
				log.Printf("✅ [WRITE-BACK] Thread %s cached successfully", threadID)
				// Extend TTL on all related keys
				if err := r.extendAllThreadTTLs(writeBackCtx, threadID); err != nil {
					log.Printf("⚠️ TTL extension failed for thread %s: %v", threadID, err)
				}
			}
		}(threadID, thread)
	}

	return thread, nil
}

// getFromValkey retrieves thread from Valkey only (internal helper)
func (r *ThreadRepository) getFromValkey(ctx context.Context, threadID string) (*models.Thread, error) {
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

// GetThreadWithCache is deprecated - use Get(ctx, threadID, false) instead
// Kept for backward compatibility with GraphQL resolvers
func (r *ThreadRepository) GetThreadWithCache(ctx context.Context, threadID string) (*models.Thread, error) {
	return r.Get(ctx, threadID, false)
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

// GetThreadWithPermissionCheck retrieves a thread with company-wide access verification
// MVP: Company-wide access - users can view all threads from their company
// Hot path: Valkey first, then verify company_id matches
// Cold path: PostgreSQL with SQL-level company filtering
func (r *ThreadRepository) GetThreadWithPermissionCheck(
	ctx context.Context,
	threadID string,
	companyID string,
) (*models.Thread, error) {
	// Try Valkey first (hot path)
	thread, err := r.getFromValkey(ctx, threadID)
	if err == nil && thread != nil {
		// Verify company-wide access
		if thread.CompanyID != companyID {
			return nil, fmt.Errorf("access denied: thread belongs to a different company")
		}
		return thread, nil
	}

	// Fallback to PostgreSQL (cold path)
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}

	// PostgreSQL query with company-level filtering
	thread, err = r.postgresRepo.GetThreadWithPermissionCheck(ctx, threadID, companyID)
	if err != nil {
		return nil, err
	}

	// Async write-back to Valkey
	go func(threadID string, thread *models.Thread) {
		writeBackCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		r.Save(writeBackCtx, thread)
	}(threadID, thread)

	return thread, nil
}
