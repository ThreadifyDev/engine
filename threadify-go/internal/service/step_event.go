package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/valkey"
)

// StepEventService handles step event processing with cryptographic hashing
type StepEventService struct {
	valkeyRepo interfaces.ValkeyClient
	threadRepo *valkey.ThreadRepository

	// Async processing
	eventQueue chan models.StepEvent
	workers    int
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex // For thread-safe shutdown

	// Thread-specific sequential processing
	threadQueues map[string]chan models.StepEvent // threadID -> queue
	threadMu     sync.RWMutex

	// Global batching for performance
	batch        []models.HashedStepEvent // Global batch (max 20)
	batchMu      sync.RWMutex
	batchSize    int
	batchTimeout time.Duration
	lastHashes   map[string]string // threadID -> lastHash (in memory cache)
	batchTimer   *time.Timer
}

// NewStepEventService creates a new step event service
func NewStepEventService(valkeyRepo interfaces.ValkeyClient, threadRepo *valkey.ThreadRepository, workers int, batchSize int, batchTimeout time.Duration) *StepEventService {
	ctx, cancel := context.WithCancel(context.Background())

	return &StepEventService{
		valkeyRepo: valkeyRepo,
		threadRepo: threadRepo,
		eventQueue: make(chan models.StepEvent, 1000), // Buffer for events
		workers:    workers,
		wg:         sync.WaitGroup{},
		ctx:        ctx,
		cancel:     cancel,
		mu:         sync.Mutex{},

		// Initialize thread-specific queues
		threadQueues: make(map[string]chan models.StepEvent),
		threadMu:     sync.RWMutex{},

		// Initialize batching from config
		batch:        make([]models.HashedStepEvent, 0, batchSize),
		batchMu:      sync.RWMutex{},
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		lastHashes:   make(map[string]string),
	}
}

// Start begins the step event processing workers
func (ses *StepEventService) Start() error {
	for i := 0; i < ses.workers; i++ {
		ses.wg.Add(1)
		go ses.processStepQueue()
	}
	return nil
}

// Stop gracefully shuts down the step event service and flushes remaining batch
func (ses *StepEventService) Stop() error {
	// Use a mutex to prevent race conditions if Stop() is called multiple times
	ses.mu.Lock()
	defer ses.mu.Unlock()

	// Check if already stopped
	select {
	case <-ses.ctx.Done():
		// Already stopped
		return nil
	default:
		// Not stopped yet, proceed with shutdown
	}

	// Flush any remaining batch before shutdown
	ses.writeBatch()

	// Close all thread-specific queues
	ses.threadMu.Lock()
	for threadID, queue := range ses.threadQueues {
		close(queue)
		delete(ses.threadQueues, threadID)
	}
	ses.threadMu.Unlock()

	close(ses.eventQueue)
	ses.cancel()
	ses.wg.Wait()
	return nil
}

// ProcessStepEvent queues a step event for thread-specific sequential processing
func (ses *StepEventService) ProcessStepEvent(event models.StepEvent) error {
	ses.threadMu.Lock()

	// Get or create thread-specific queue
	threadQueue, exists := ses.threadQueues[event.ThreadID]
	if !exists {
		// Create new queue for this thread
		threadQueue = make(chan models.StepEvent, 100)
		ses.threadQueues[event.ThreadID] = threadQueue

		// Start sequential processor for this thread
		ses.wg.Add(1)
		go ses.processThreadQueue(event.ThreadID, threadQueue)
	}
	ses.threadMu.Unlock()

	// Add event to thread-specific queue
	select {
	case threadQueue <- event:
		return nil
	case <-ses.ctx.Done():
		return fmt.Errorf("service is shutting down")
	default:
		return fmt.Errorf("thread queue is full for thread %s", event.ThreadID)
	}
}

// processThreadQueue processes events sequentially for a specific thread
func (ses *StepEventService) processThreadQueue(threadID string, threadQueue chan models.StepEvent) {
	defer ses.wg.Done()

	for {
		select {
		case event, ok := <-threadQueue:
			if !ok {
				return // Queue closed
			}

			// Process event sequentially for this thread
			if err := ses.processStepEvent(event); err != nil {
				fmt.Printf("Error processing step event for thread %s: %v\n", threadID, err)
			}

		case <-ses.ctx.Done():
			return
		}
	}
}

// processStepQueue processes events from the global queue (legacy, not used anymore)
func (ses *StepEventService) processStepQueue() {
	defer ses.wg.Done()

	for {
		select {
		case event, ok := <-ses.eventQueue:
			if !ok {
				return // Queue closed
			}

			// Redirect to thread-specific queue
			ses.ProcessStepEvent(event)

		case <-ses.ctx.Done():
			return
		}
	}
}

// processStepEvent handles a single step event with batching
func (ses *StepEventService) processStepEvent(event models.StepEvent) error {
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

	// 5. Add to batch for bulk processing
	ses.addToBatch(*hashedEvent)

	// 6. Update memory cache immediately (prevents race condition)
	ses.batchMu.Lock()
	ses.lastHashes[event.ThreadID] = newHash
	ses.batchMu.Unlock()

	return nil
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
	ses.batchMu.RLock()

	// 1. Check memory cache first (instant)
	if hash, exists := ses.lastHashes[threadID]; exists {
		ses.batchMu.RUnlock()
		return hash, nil
	}
	ses.batchMu.RUnlock()

	// 2. Fallback to Valkey if not in memory
	threadData, err := ses.threadRepo.Get(ses.ctx, threadID)
	if err != nil {
		// If thread doesn't exist, return empty string for genesis hash
		return "", nil
	}

	// 3. Store in memory cache for next time
	ses.batchMu.Lock()
	ses.lastHashes[threadID] = threadData.LastHash
	ses.batchMu.Unlock()

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

// addToBatch adds an event to the global batch and triggers write if needed
func (ses *StepEventService) addToBatch(event models.HashedStepEvent) {
	ses.batchMu.Lock()
	defer ses.batchMu.Unlock()

	// Add event to batch
	ses.batch = append(ses.batch, event)

	// Check if we should write the batch
	if len(ses.batch) >= ses.batchSize {
		// Batch size reached - write immediately
		go ses.writeBatch()
	} else if len(ses.batch) == 1 {
		// First event in batch - start timer
		ses.batchTimer = time.AfterFunc(ses.batchTimeout, func() {
			ses.writeBatch()
		})
	}
}

// writeBatch performs bulk write of all events in the batch
func (ses *StepEventService) writeBatch() {
	ses.batchMu.Lock()
	if len(ses.batch) == 0 {
		ses.batchMu.Unlock()
		return
	}

	// Copy batch and clear it
	eventsToWrite := make([]models.HashedStepEvent, len(ses.batch))
	copy(eventsToWrite, ses.batch)
	ses.batch = ses.batch[:0] // Clear but keep capacity

	// Stop timer if running
	if ses.batchTimer != nil {
		ses.batchTimer.Stop()
		ses.batchTimer = nil
	}
	ses.batchMu.Unlock()

	// Perform bulk write
	if err := ses.bulkWriteToRedis(eventsToWrite); err != nil {
		fmt.Printf("Error writing batch to Redis: %v\n", err)
	}
}

// bulkWriteToRedis writes multiple events to Redis in a single operation
func (ses *StepEventService) bulkWriteToRedis(events []models.HashedStepEvent) error {
	for _, event := range events {
		// Store individual event
		if err := ses.storeStepEvent(event); err != nil {
			return fmt.Errorf("failed to store event %s: %w", event.StepID, err)
		}

		// Update thread last hash
		if err := ses.updateThreadLastHash(event.ThreadID, event.Hash); err != nil {
			return fmt.Errorf("failed to update thread hash for %s: %w", event.ThreadID, err)
		}
	}
	return nil
}

// storeStepEvent persists the hashed step event to Redis and streams
func (ses *StepEventService) storeStepEvent(event models.HashedStepEvent) error {
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
		"actor":           event.ServiceName,
		"status":          event.Status,
		"hash":            event.Hash,
		"prev_hash":       event.PreviousHash,
		"started_at":      event.StartedAt,
		"finished_at":     event.FinishedAt,
	}

	// 1. Write to per-thread LIST for fast queries (7-day TTL)
	activityList := fmt.Sprintf("thread:%s:activity", event.ThreadID)
	eventJSON, _ := json.Marshal(activityValues)
	pipe.LPush(ses.ctx, activityList, string(eventJSON))
	pipe.Expire(ses.ctx, activityList, 7*24*time.Hour)

	// 2. Write to partitioned STREAM for reliable archival
	partition := ses.getPartitionForThread(event.ThreadID)
	partitionedStream := fmt.Sprintf("streams:activity_log:%d", partition)
	pipe.XAdd(ses.ctx, partitionedStream, activityValues)

	// Execute pipeline
	_, err := pipe.Exec(ses.ctx)

	duration := time.Since(startTime)
	fmt.Printf("⏱️  [StepEventService] Valkey write (Activity Streams) took %v for stepId=%s\n", duration, event.StepID)

	return err
}

// updateThreadLastHash updates the thread's last hash and refreshes TTL
func (ses *StepEventService) updateThreadLastHash(threadID, newHash string) error {
	// Get current thread data using the same key pattern as ThreadRepository
	threadKey := fmt.Sprintf("thread:%s", threadID)
	threadData, err := ses.valkeyRepo.Get(ses.ctx, threadKey)
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

	return ses.valkeyRepo.Set(ses.ctx, threadKey, string(updatedData), 24*time.Hour)
}

// getPartitionForThread calculates the partition number for a thread ID
// Uses FNV hash for consistent distribution across partitions
func (ses *StepEventService) getPartitionForThread(threadID string) int {
	const numPartitions = 10 // Should match archiver config
	h := fnv.New32a()
	h.Write([]byte(threadID))
	return int(h.Sum32() % uint32(numPartitions))
}
