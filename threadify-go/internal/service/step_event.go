package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"github.com/threadify/engine/internal/config"
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
	config        *config.Config
}

// NewStepEventService creates a new step event service
func NewStepEventService(valkeyRepo interfaces.ValkeyClient, threadRepo *valkey.ThreadRepository, natsPublisher *natsrepo.ArchivalPublisher, cfg *config.Config, workers int, batchSize int, batchTimeout time.Duration) *StepEventService {
	return &StepEventService{
		valkeyRepo:    valkeyRepo,
		threadRepo:    threadRepo,
		natsPublisher: natsPublisher,
		config:        cfg,
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
	// Track overall step event latency
	startTime := time.Now()
	stepEventID := fmt.Sprintf("%s:%s:%s", event.ThreadID, event.StepName, event.IdempotencyKey)
	log.Printf("[PERF] StepEvent START: %s | thread=%s | step=%s", stepEventID, event.ThreadID, event.StepName)

	defer func() {
		duration := time.Since(startTime)
		log.Printf("[PERF] StepEvent COMPLETE: %s | duration=%v", stepEventID, duration)
	}()

	// 1. Validate the step event
	validationStart := time.Now()
	if err := ses.validateStepEvent(event); err != nil {
		// User input validation errors are safe to expose with context
		log.Printf("[PERF] StepEvent VALIDATION FAILED: %s | duration=%v | error=%v", stepEventID, time.Since(validationStart), err)
		return fmt.Errorf("invalid step data: %w", err)
	}
	log.Printf("[PERF] StepEvent VALIDATION: %s | duration=%v", stepEventID, time.Since(validationStart))

	// 2. Execute atomic hash generation via Lua script
	hashStart := time.Now()
	hashResult, err := ses.executeAtomicHashScript(event, ownerID, serviceName)
	hashDuration := time.Since(hashStart)
	log.Printf("[PERF] StepEvent HASH_GENERATION: %s | duration=%v | success=%t", stepEventID, hashDuration, err == nil)

	if err != nil {
		return err // Already sanitized by executeAtomicHashScript
	}

	// 3. Send activity event to NATS for archival (SYNCHRONOUS - critical for audit trail)
	// Skip if NATS publisher is not available (graceful degradation)
	if ses.natsPublisher != nil {
		natsStart := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		activityEvent := ses.createActivityEvent(hashResult, event, ownerID, serviceName)

		if err := ses.natsPublisher.PublishActivityLog(ctx, activityEvent); err != nil {
			natsDuration := time.Since(natsStart)
			log.Printf("[PERF] StepEvent NATS_PUBLISH FAILED: %s | duration=%v | error=%v", stepEventID, natsDuration, err)
			logInternalErrorWithDetails("PublishActivityLog", fmt.Sprintf("stepId=%s", event.StepID), err)
		} else {
			log.Printf("[PERF] StepEvent NATS_PUBLISH: %s | duration=%v", stepEventID, time.Since(natsStart))
		}
	} else {
		log.Printf("[PERF] StepEvent NATS_PUBLISH: %s | SKIPPED (no publisher)", stepEventID)
	}

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
	stepEventID := fmt.Sprintf("%s:%s:%s", event.ThreadID, event.StepName, event.IdempotencyKey)

	// Step 1: Get current hash atomically
	getHashStart := time.Now()
	threadKey := fmt.Sprintf("thread:%s", event.ThreadID)

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

	result, err := ses.valkeyRepo.Eval(context.Background(), luaScript, []string{threadKey})
	getHashDuration := time.Since(getHashStart)
	log.Printf("[PERF] StepEvent LUA_GET_HASH: %s | duration=%v | success=%t", stepEventID, getHashDuration, err == nil)

	if err != nil {
		log.Printf("[PERF] StepEvent LUA_GET_HASH FAILED: %s | duration=%v | error=%v", stepEventID, getHashDuration, err)
		logInternalErrorWithDetails("executeAtomicHashScript (get old hash)", fmt.Sprintf("threadId=%s", event.ThreadID), err)
		return nil, sanitizeError(err)
	}

	// Parse result to get oldHash
	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) < 1 {
		log.Printf("[PERF] StepEvent LUA_PARSE_FAILED: %s | unexpected result format: %v", stepEventID, result)
		logInternalError("executeAtomicHashScript", fmt.Errorf("unexpected result format: %v", result))
		return nil, fmt.Errorf("failed to process step event")
	}

	oldHash, _ := resultSlice[0].(string)

	// Step 2: Calculate new hash using HMAC-SHA256 with secret key
	calcHashStart := time.Now()
	// Format: hmac-sha256-{version}:{hash}
	version := ses.config.Security.HashChainCurrentVersion
	if version == "" {
		log.Printf("[PERF] StepEvent HASH_CALC_FAILED: %s | version not configured", stepEventID)
		logInternalError("executeAtomicHashScript", fmt.Errorf("hash_chain_current_version not configured"))
		return nil, fmt.Errorf("failed to process step event")
	}

	secret := ses.config.Security.HashChainSecrets[version]
	if secret == "" {
		log.Printf("[PERF] StepEvent HASH_CALC_FAILED: %s | secret version %s not configured", stepEventID, version)
		logInternalError("executeAtomicHashScript", fmt.Errorf("hash secret version %s not configured", version))
		return nil, fmt.Errorf("failed to process step event")
	}

	h := hmac.New(sha256.New, []byte(secret))
	hashData := fmt.Sprintf("%s:%s:%s:%s:%s", oldHash, event.ThreadID, event.StepID, event.IdempotencyKey, event.Timestamp.Format(time.RFC3339))
	h.Write([]byte(hashData))
	newHash := fmt.Sprintf("hmac-sha256-%s:%x", version, h.Sum(nil))
	log.Printf("[PERF] StepEvent HASH_CALC: %s | duration=%v", stepEventID, time.Since(calcHashStart))

	// Step 3: Update thread metadata atomically with new hash
	updateStart := time.Now()
	// Note: Thread TTL is managed by validate_and_update_step_state.lua
	// which extends TTL on all thread keys to prevent partial expiration
	updateScript := `
		-- Update thread metadata with new hash
		local threadKey = KEYS[1]
		local newHash = ARGV[1]
		local currentVersion = ARGV[2]
		
		-- Get current thread data
		local threadData = redis.call('GET', threadKey)
		if not threadData then
			return {err = "Thread not found"}
		end
		
		-- Parse and update
		local threadObj = cjson.decode(threadData)
		threadObj.lastHash = newHash
		threadObj.hashVersion = currentVersion
		
		-- Save back atomically
		redis.call('SET', threadKey, cjson.encode(threadObj))
		
		return {ok = "updated"}
	`

	_, err = ses.valkeyRepo.Eval(context.Background(), updateScript, []string{threadKey}, newHash, version)
	updateDuration := time.Since(updateStart)
	log.Printf("[PERF] StepEvent LUA_UPDATE_HASH: %s | duration=%v | success=%t", stepEventID, updateDuration, err == nil)

	if err != nil {
		log.Printf("[PERF] StepEvent LUA_UPDATE_HASH FAILED: %s | duration=%v | error=%v", stepEventID, updateDuration, err)
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
