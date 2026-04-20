package valkey

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
	apperrors "github.com/threadify/engine/internal/utils/errors"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

// ThreadRepository handles thread storage in Valkey (Redis)
type ThreadRepository struct {
	valkey            interfaces.ValkeyClient
	ttl               int // TTL in seconds
	postgresRepo      *postgres.ThreadRepository
	stepStatePostgres *postgres.StepStateRepository // For step state queries
	cacheManager      interfaces.CacheManager       // For duplicate detection via LRU cache
	writeBackPool     *workerpool.Pool              // For async cache write-backs
	luaScripts        interfaces.LuaScriptManager   // For atomic Lua script operations
	logger            *zap.Logger
}

// NewThreadRepository creates a new thread repository with PostgreSQL fallback
// PostgreSQL fallback is always required for production hot/cold architecture
func NewThreadRepository(valkey interfaces.ValkeyClient, ttl int, postgresRepo *postgres.ThreadRepository, stepStatePostgres *postgres.StepStateRepository, cacheManager interfaces.CacheManager, luaScripts interfaces.LuaScriptManager, logger *zap.Logger) *ThreadRepository {
	return &ThreadRepository{
		valkey:            valkey,
		ttl:               ttl,
		postgresRepo:      postgresRepo,
		stepStatePostgres: stepStatePostgres,
		cacheManager:      cacheManager,
		luaScripts:        luaScripts,
		logger:            logger,
	}
}

// GetPostgresRepo returns the underlying postgres repository for direct queries
func (r *ThreadRepository) GetPostgresRepo() *postgres.ThreadRepository {
	return r.postgresRepo
}

// SetWriteBackPool sets the worker pool for async cache write-backs
func (r *ThreadRepository) SetWriteBackPool(pool *workerpool.Pool) {
	r.writeBackPool = pool
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
	r.logger.Debug("Get thread called", zap.String("thread_id", threadID))

	// Default writeBack to false
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

	// Try Valkey first (hot data)
	thread, err := r.getFromValkey(ctx, threadID)

	if err == nil && thread != nil {
		r.logger.Info("Thread retrieved from cache", zap.String("thread_id", threadID), zap.String("source", "valkey"))
		return thread, nil
	}

	// Fallback to PostgreSQL (cold data)
	if r.postgresRepo == nil {
		r.logger.Error("PostgreSQL repository not configured, cannot fallback", zap.String("thread_id", threadID))
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}

	r.logger.Info("Cache miss, querying PostgreSQL", zap.String("thread_id", threadID))

	// REUSE: GetWithRefs already exists from GraphQL implementation
	thread, err = r.postgresRepo.GetWithRefs(ctx, threadID)
	if err != nil {
		return nil, apperrors.NewNotFoundError(apperrors.MsgThreadNotFound, err)
	}

	r.logger.Info("Thread retrieved from PostgreSQL", zap.String("thread_id", threadID))

	// Async write-back via worker pool using Save() for format consistency
	// Don't block the read operation - write-back happens in background
	if shouldWriteBack && r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			// Create new context with timeout for write-back operation
			writeBackCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			r.logger.Debug("Starting async cache write-back", zap.String("thread_id", threadID))
			if err := r.Save(writeBackCtx, thread); err != nil {
				r.logger.Warn("Async write-back failed", zap.String("thread_id", threadID), zap.Error(err))
			} else {
				r.logger.Info("Thread cached successfully", zap.String("thread_id", threadID))
			}
		})
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

// UpdateThreadStatus atomically updates thread status in Valkey cache
// Returns nil if already terminal (not an error, just skipped)
// Uses Lua script to prevent race conditions with double-ending
func (r *ThreadRepository) UpdateThreadStatus(ctx context.Context, threadID string, status string, timestamp time.Time) error {
	key := r.getThreadKey(threadID)
	metaKey := r.getThreadMetaKey(threadID)

	// Check if Lua script manager is available
	if r.luaScripts == nil {
		return fmt.Errorf("luaScripts not initialized - cannot update thread status atomically")
	}

	// Get script hash
	scriptHash, exists := r.luaScripts.GetScriptHash("check_and_update_thread_status")
	if !exists {
		return fmt.Errorf("check_and_update_thread_status script not loaded")
	}

	// Execute atomic check-and-update via Lua script
	result, err := r.valkey.EvalSHA(ctx, scriptHash, []string{metaKey, key}, status, timestamp.Format(time.RFC3339), r.ttl)
	if err != nil {
		return fmt.Errorf("lua script failed: %w", err)
	}

	// Parse result: {success, status}
	arr, ok := result.([]interface{})
	if !ok || len(arr) < 2 {
		return fmt.Errorf("unexpected result format from lua script")
	}

	success, ok := arr[0].(int64)
	if !ok {
		return fmt.Errorf("unexpected success value type from lua script")
	}

	if success == 0 {
		// Already terminal - not an error, just skip
		currentStatus, _ := arr[1].(string)
		r.logger.Debug("thread already terminal, skipping update",
			zap.String("thread_id", threadID),
			zap.String("current_status", currentStatus),
			zap.String("attempted_status", status))
		return nil
	}

	r.logger.Debug("thread status updated atomically",
		zap.String("thread_id", threadID),
		zap.String("new_status", status))
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

	// Async write-back to Valkey via worker pool
	if r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeBackCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			r.Save(writeBackCtx, thread)
		})
	}

	return thread, nil
}
