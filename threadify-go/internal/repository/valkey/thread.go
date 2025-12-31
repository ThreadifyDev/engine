package valkey

import (
	"context"
	"encoding/json"
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
		// Overlay status from hash
		if status, ok := meta["status"]; ok {
			thread.Status = models.ThreadStatus(status)
		}
		// Overlay completedAt from hash
		if completedAtStr, ok := meta["completedAt"]; ok && completedAtStr != "" {
			if completedAt, err := time.Parse(time.RFC3339, completedAtStr); err == nil {
				thread.CompletedAt = &completedAt
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

// Role management methods

// AssignRole assigns a role to a user in a thread
func (r *ThreadRepository) AssignRole(ctx context.Context, threadID, role, userID string) error {
	key := r.getThreadRolesKey(threadID)
	return r.valkey.HSet(ctx, key, role, userID)
}

// GetUserRole gets the role assigned to a user in a thread
func (r *ThreadRepository) GetUserRole(ctx context.Context, threadID, userID string) (string, error) {
	key := r.getThreadRolesKey(threadID)
	roles, err := r.valkey.HGetAll(ctx, key)
	if err != nil {
		return "", err
	}

	// Find the role assigned to this user
	for role, assignedUser := range roles {
		if assignedUser == userID {
			return role, nil
		}
	}

	return "", nil
}

// GetAllRoles gets all role assignments for a thread
func (r *ThreadRepository) GetAllRoles(ctx context.Context, threadID string) (map[string]string, error) {
	key := r.getThreadRolesKey(threadID)
	return r.valkey.HGetAll(ctx, key)
}

// RemoveRole removes a role assignment in a thread
func (r *ThreadRepository) RemoveRole(ctx context.Context, threadID, role string) error {
	key := r.getThreadRolesKey(threadID)
	return r.valkey.HDel(ctx, key, role)
}

// Permission management methods

// SetUserPermissions sets permissions for a user in a thread
func (r *ThreadRepository) SetUserPermissions(ctx context.Context, threadID, userID string, permissions []string) error {
	key := r.getThreadPermissionsKey(threadID)
	permissionsJSON, err := json.Marshal(permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}
	return r.valkey.HSet(ctx, key, userID, string(permissionsJSON))
}

// GetUserPermissions gets permissions for a user in a thread
func (r *ThreadRepository) GetUserPermissions(ctx context.Context, threadID, userID string) ([]string, error) {
	key := r.getThreadPermissionsKey(threadID)
	permissionsJSON, err := r.valkey.HGet(ctx, key, userID)
	if err != nil {
		return nil, err
	}

	var permissions []string
	err = json.Unmarshal([]byte(permissionsJSON), &permissions)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}

	return permissions, nil
}

// Event queue methods

// AddThreadEvent adds an event to the thread's activity queue
func (r *ThreadRepository) AddThreadEvent(ctx context.Context, threadID string, event models.ThreadEvent) error {
	key := r.getThreadEventsKey(threadID)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	return r.valkey.LPush(ctx, key, string(eventJSON))
}

// GetThreadEvents gets events from the thread's activity queue
func (r *ThreadRepository) GetThreadEvents(ctx context.Context, threadID string, start, stop int64) ([]models.ThreadEvent, error) {
	key := r.getThreadEventsKey(threadID)
	eventJSONs, err := r.valkey.LRange(ctx, key, start, stop)
	if err != nil {
		return nil, err
	}

	events := make([]models.ThreadEvent, 0, len(eventJSONs))
	for _, eventJSON := range eventJSONs {
		var event models.ThreadEvent
		err := json.Unmarshal([]byte(eventJSON), &event)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal event: %w", err)
		}
		events = append(events, event)
	}

	return events, nil
}

// Pipeline operations

// CreateThreadWithSetup creates a thread with initial role and permissions using pipeline
func (r *ThreadRepository) CreateThreadWithSetup(ctx context.Context, thread *models.Thread, creatorID string, creatorRole string, creatorPerms []string) error {
	pipe := r.valkey.Pipeline()

	// 1. Save thread base data as JSON
	threadKey := r.getThreadKey(thread.ID)
	threadJSON, err := thread.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal thread: %w", err)
	}
	pipe.Set(ctx, threadKey, string(threadJSON), time.Duration(r.ttl)*time.Second)

	// 2. Save thread metadata in hash (for atomic Lua updates)
	metaKey := r.getThreadMetaKey(thread.ID)
	metadata := map[string]interface{}{
		"status": string(thread.Status),
	}
	if thread.CompletedAt != nil {
		metadata["completedAt"] = thread.CompletedAt.Format(time.RFC3339)
	}
	pipe.HSet(ctx, metaKey, metadata)
	pipe.Expire(ctx, metaKey, time.Duration(r.ttl)*time.Second)

	// 3. Set initial role
	rolesKey := r.getThreadRolesKey(thread.ID)
	pipe.HSet(ctx, rolesKey, creatorRole, creatorID)
	pipe.Expire(ctx, rolesKey, time.Duration(r.ttl)*time.Second)

	// 4. Set initial permissions
	permsKey := r.getThreadPermissionsKey(thread.ID)
	permsJSON, _ := json.Marshal(creatorPerms)
	pipe.HSet(ctx, permsKey, creatorID, string(permsJSON))
	pipe.Expire(ctx, permsKey, time.Duration(r.ttl)*time.Second)

	// 5. Add creation event
	eventKey := r.getThreadEventsKey(thread.ID)
	eventJSON, _ := json.Marshal(models.ThreadEvent{
		Action:    "thread_created",
		ThreadID:  thread.ID,
		UserID:    creatorID,
		Role:      creatorRole,
		Timestamp: time.Now().UTC(),
	})
	pipe.LPush(ctx, eventKey, string(eventJSON))
	pipe.Expire(ctx, eventKey, time.Duration(r.ttl)*time.Second)

	_, err = pipe.Exec(ctx)
	return err
}

// CompleteThread marks a thread as completed with optional immediate cleanup
func (r *ThreadRepository) CompleteThread(ctx context.Context, threadID string, immediateCleanup bool) error {
	// Get and update thread
	thread, err := r.Get(ctx, threadID)
	if err != nil {
		return err
	}

	thread.Complete()

	if immediateCleanup {
		// Immediate cleanup using pipeline
		pipe := r.valkey.Pipeline()

		// Mark thread as completed
		threadKey := r.getThreadKey(threadID)
		threadJSON, _ := thread.ToJSON()
		pipe.Set(ctx, threadKey, string(threadJSON), time.Duration(r.ttl)*time.Second)

		// Clean up everything immediately
		pipe.Del(ctx, r.getThreadRolesKey(threadID))
		pipe.Del(ctx, r.getThreadPermissionsKey(threadID))
		pipe.Del(ctx, r.getThreadEventsKey(threadID))

		_, err = pipe.Exec(ctx)
		return err
	} else {
		// Just mark as completed, let TTLs handle cleanup
		return r.Save(ctx, thread)
	}
}

// Helper methods for key generation

func (r *ThreadRepository) getThreadRolesKey(threadID string) string {
	return fmt.Sprintf("thread:%s:roles", threadID)
}

func (r *ThreadRepository) getThreadPermissionsKey(threadID string) string {
	return fmt.Sprintf("thread:%s:permissions", threadID)
}

func (r *ThreadRepository) getThreadEventsKey(threadID string) string {
	return fmt.Sprintf("thread:%s:queue", threadID)
}

func (r *ThreadRepository) getThreadMetaKey(threadID string) string {
	return fmt.Sprintf("thread:%s:meta", threadID)
}
