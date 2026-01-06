package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
)

// StepEventService handles step event processing with cryptographic hashing
type StepEventService struct {
	valkeyRepo    interfaces.ValkeyClient
	threadRepo    *valkey.ThreadRepository
	natsPublisher *natsrepo.ArchivalPublisher

	// Hash cache
	lastHashes map[string]string // threadID -> lastHash (in memory cache)
	mu         sync.Mutex        // For thread-safe operations
}

// NewStepEventService creates a new step event service
func NewStepEventService(valkeyRepo interfaces.ValkeyClient, threadRepo *valkey.ThreadRepository, natsPublisher *natsrepo.ArchivalPublisher, workers int, batchSize int, batchTimeout time.Duration) *StepEventService {
	return &StepEventService{
		valkeyRepo:    valkeyRepo,
		threadRepo:    threadRepo,
		natsPublisher: natsPublisher,
		lastHashes:    make(map[string]string),
	}
}

// Start begins the step event service (no-op for direct write mode)
func (ses *StepEventService) Start() error {
	return nil
}

// Stop stops the step event service (no-op for direct write mode)
func (ses *StepEventService) Stop() error {
	return nil
}

// RecordStepEventDirect records a step event immediately without batching
func (ses *StepEventService) RecordStepEventDirect(event models.StepEvent, ownerID string) error {
	// 1. Validate the step event
	if err := ses.validateStepEvent(event); err != nil {
		return fmt.Errorf("invalid step event: %w", err)
	}

	// 2. Get the last hash for this thread (from memory or Valkey)
	previousHash, err := ses.getLastStepHash(event.ThreadID)
	if err != nil {
		return fmt.Errorf("failed to get last step hash: %w", err)
	}

	// 3. Calculate the new hash
	newHash := ses.calculateStepHash(previousHash, event)

	// 4. Create the hashed step event
	hashedEvent := event.ToHashedStepEvent(newHash, previousHash)

	// 5. Store immediately
	return ses.storeStepEvent(*hashedEvent, ownerID)
}

// validateStepEvent ensures the step event has required fields
func (ses *StepEventService) validateStepEvent(event models.StepEvent) error {
	if event.StepID == "" {
		return fmt.Errorf("step_id is required")
	}
	if event.ThreadID == "" {
		return fmt.Errorf("thread_id is required")
	}
	if event.Context == nil {
		return fmt.Errorf("context is required")
	}
	return nil
}

// getLastStepHash retrieves the last hash for a thread using memory cache with Valkey fallback
func (ses *StepEventService) getLastStepHash(threadID string) (string, error) {
	ses.mu.Lock()

	// 1. Check memory cache first (instant)
	if hash, exists := ses.lastHashes[threadID]; exists {
		ses.mu.Unlock()
		return hash, nil
	}
	ses.mu.Unlock()

	// 2. Fallback to Valkey if not in memory
	threadData, err := ses.threadRepo.Get(context.Background(), threadID)
	if err != nil {
		// If thread doesn't exist, return empty string for genesis hash
		return "", nil
	}

	// 3. Store in memory cache for next time
	ses.mu.Lock()
	ses.lastHashes[threadID] = threadData.LastHash
	ses.mu.Unlock()

	return threadData.LastHash, nil
}

// calculateStepHash computes SHA256 hash of previousHash + stepID + stepName + startedAt + finishedAt + status + context
func (ses *StepEventService) calculateStepHash(previousHash string, event models.StepEvent) string {
	hasher := sha256.New()

	// Add previous hash (empty for genesis)
	hasher.Write([]byte(previousHash))

	// Add step ID
	hasher.Write([]byte(event.StepID))

	// Add step name
	hasher.Write([]byte(event.StepName))

	// Add step lifecycle data
	hasher.Write([]byte(event.StartedAt))
	hasher.Write([]byte(event.FinishedAt))
	hasher.Write([]byte(event.Status))

	// Add context as JSON
	hasher.Write([]byte(event.ContextJSON()))

	return hex.EncodeToString(hasher.Sum(nil))
}

// storeStepEvent persists the hashed step event to Redis and streams
func (ses *StepEventService) storeStepEvent(event models.HashedStepEvent, ownerID string) error {
	startTime := time.Now()

	// Use pipeline for atomic writes
	pipe := ses.valkeyRepo.Pipeline()

	// Prepare activity event data
	activityValues := map[string]interface{}{
		"type":            "step_recorded",
		"thread_id":       event.ThreadID,
		"step_id":         fmt.Sprintf("%s:%s", event.StepName, event.IdempotencyKey), // composite: name:idempKey
		"step_name":       event.StepName,
		"step_uuid":       event.StepID,
		"idempotency_key": event.IdempotencyKey, // User-provided or context hash
		"timestamp":       event.Timestamp.Format(time.RFC3339),
		"context":         event.ContextJSON(),
		"actor":           ownerID,           // user-123
		"actor_service":   event.ServiceName, // merchant-service
		"status":          event.Status,
		"hash":            event.Hash,
		"prev_hash":       event.PreviousHash,
		"started_at":      event.StartedAt,
		"finished_at":     event.FinishedAt,
	}

	// 1. Write to per-thread LIST for fast queries (7-day TTL)
	activityList := fmt.Sprintf("thread:%s:activity", event.ThreadID)
	eventJSON, _ := json.Marshal(activityValues)
	pipe.LPush(context.Background(), activityList, string(eventJSON))
	pipe.Expire(context.Background(), activityList, 7*24*time.Hour)

	// Execute pipeline
	_, err := pipe.Exec(context.Background())
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to write step event to activity list: %v\n", err)
		return err
	}

	// 2. Publish to NATS for archival (async, non-blocking)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ses.natsPublisher.PublishActivityLog(ctx, activityValues); err != nil {
			fmt.Printf("❌ ERROR: Failed to publish activity log to NATS for stepId=%s: %v\n", event.StepID, err)
		} else {
			fmt.Printf("✅ SUCCESS: Activity log published to NATS for stepId=%s\n", event.StepID)
		}
	}()

	duration := time.Since(startTime)
	fmt.Printf("⏱️  [StepEventService] Valkey write (Activity Streams) took %v for stepId=%s\n", duration, event.StepID)

	return err
}

// updateThreadLastHash updates the thread's last hash and refreshes TTL
func (ses *StepEventService) updateThreadLastHash(threadID, newHash string) error {
	// Get current thread data using the same key pattern as ThreadRepository
	threadKey := fmt.Sprintf("thread:%s", threadID)
	threadData, err := ses.valkeyRepo.Get(context.Background(), threadKey)
	if err != nil {
		return fmt.Errorf("failed to get thread data: %w", err)
	}

	// Parse and update thread
	var thread models.Thread
	if err := json.Unmarshal([]byte(threadData), &thread); err != nil {
		return fmt.Errorf("failed to parse thread data: %w", err)
	}

	thread.LastHash = newHash

	// Serialize and store back with refreshed TTL
	updatedData, err := json.Marshal(thread)
	if err != nil {
		return fmt.Errorf("failed to serialize thread: %w", err)
	}

	return ses.valkeyRepo.Set(context.Background(), threadKey, string(updatedData), 24*time.Hour)
}
