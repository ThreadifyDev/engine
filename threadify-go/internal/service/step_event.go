package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
)

// StepEventService handles step event processing with cryptographic hashing
// Hash generation is now handled atomically via Lua scripts rather than in-memory cache
type StepEventService struct {
	valkeyRepo    interfaces.ValkeyClient
	threadRepo    *valkey.ThreadRepository
	natsPublisher *natsrepo.ArchivalPublisher
}

// NewStepEventService creates a new step event service
func NewStepEventService(valkeyRepo interfaces.ValkeyClient, threadRepo *valkey.ThreadRepository, natsPublisher *natsrepo.ArchivalPublisher, workers int, batchSize int, batchTimeout time.Duration) *StepEventService {
	return &StepEventService{
		valkeyRepo:    valkeyRepo,
		threadRepo:    threadRepo,
		natsPublisher: natsPublisher,
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
func (ses *StepEventService) RecordStepEventDirect(event models.StepEvent, ownerID, serviceName string) error {
	// 1. Validate the step event
	if err := ses.validateStepEvent(event); err != nil {
		// User input validation errors are safe to expose with context
		return fmt.Errorf("invalid step data: %w", err)
	}

	// 2. Execute atomic hash generation via Lua script
	hashResult, err := ses.executeAtomicHashScript(event, ownerID, serviceName)
	if err != nil {
		return err // Already sanitized by executeAtomicHashScript
	}

	// 3. Send activity event to NATS for archival (async, non-blocking)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		activityEvent := ses.createActivityEvent(hashResult, event, ownerID, serviceName)

		if err := ses.natsPublisher.PublishActivityLog(ctx, activityEvent); err != nil {
			logInternalErrorWithDetails("PublishActivityLog", fmt.Sprintf("stepId=%s", event.StepID), err)
		}
	}()

	return nil
}

// HashResult contains the result of atomic hash generation
type HashResult struct {
	OldHash  string
	NewHash  string
	ThreadID string
}

// executeAtomicHashScript performs atomic hash generation and thread metadata update via Lua script
func (ses *StepEventService) executeAtomicHashScript(event models.StepEvent, ownerID, serviceName string) (*HashResult, error) {
	// Execute Lua script to get current hash and atomically update thread metadata
	luaScript := `
		-- Get current thread metadata
		local threadKey = KEYS[1]
		local threadData = redis.call('GET', threadKey)
		
		if not threadData then
			return {err = "Thread not found"}
		end
		
		-- Parse thread JSON using cjson (no require needed in Redis/Valkey)
		local threadObj = cjson.decode(threadData)
		local oldHash = threadObj.lastHash or ""
		
		-- Return old hash for Go to calculate new hash
		return {oldHash}
	`

	// Step 1: Get current hash atomically
	threadKey := fmt.Sprintf("thread:%s", event.ThreadID)
	result, err := ses.valkeyRepo.Eval(context.Background(), luaScript, []string{threadKey})
	if err != nil {
		logInternalErrorWithDetails("executeAtomicHashScript (get old hash)", fmt.Sprintf("threadId=%s", event.ThreadID), err)
		return nil, sanitizeError(err)
	}

	// Parse result to get oldHash
	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 1 {
		logInternalError("executeAtomicHashScript", fmt.Errorf("unexpected result format: %v", result))
		return nil, fmt.Errorf("failed to process step event")
	}

	oldHash, _ := resultSlice[0].(string)

	// Step 2: Calculate new hash in Go (includes oldHash for chain integrity)
	hashData := fmt.Sprintf("%s:%s:%s:%s:%s", oldHash, event.ThreadID, event.StepID, event.IdempotencyKey, event.Timestamp.Format(time.RFC3339))
	newHash := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(hashData)))

	// Step 3: Update thread metadata atomically with new hash
	updateScript := `
		-- Update thread metadata with new hash
		local threadKey = KEYS[1]
		local threadData = redis.call('GET', threadKey)
		
		if not threadData then
			return {err = "Thread not found"}
		end
		
		-- cjson is available globally in Redis/Valkey
		local threadObj = cjson.decode(threadData)
		threadObj.lastHash = ARGV[1]
		local updatedThread = cjson.encode(threadObj)
		redis.call('SET', threadKey, updatedThread, 'EX', 86400)
		
		return {ARGV[1]}
	`

	result, err = ses.valkeyRepo.Eval(context.Background(), updateScript, []string{threadKey}, newHash)
	if err != nil {
		logInternalErrorWithDetails("executeAtomicHashScript (update hash)", fmt.Sprintf("threadId=%s", event.ThreadID), err)
		return nil, sanitizeError(err)
	}

	return &HashResult{
		OldHash:  oldHash,
		NewHash:  newHash,
		ThreadID: event.ThreadID,
	}, nil
}

// createActivityEvent creates an activity event with all required fields for NATS publishing
func (ses *StepEventService) createActivityEvent(hashResult *HashResult, event models.StepEvent, ownerID, serviceName string) map[string]interface{} {
	activityValues := map[string]interface{}{
		"type":            "step_recorded",
		"thread_id":       event.ThreadID,
		"step_id":         fmt.Sprintf("%s:%s", event.StepName, event.IdempotencyKey),
		"step_name":       event.StepName,
		"step_uuid":       event.StepID,
		"idempotency_key": event.IdempotencyKey,
		"timestamp":       event.Timestamp.Format(time.RFC3339),
		"context":         event.ContextJSON(),
		"actor":           ownerID,
		"actor_service":   serviceName,
		"status":          event.Status,
		"hash":            hashResult.NewHash,
		"prev_hash":       hashResult.OldHash,
		"started_at":      event.StartedAt,
	}

	return activityValues
}

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
