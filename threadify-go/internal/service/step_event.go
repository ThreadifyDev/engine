package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
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

// loadLuaScript loads a Lua script from the lua/ directory
func loadLuaScript(filename string) (string, error) {
	// Try multiple possible paths (handles different working directories)
	possiblePaths := []string{
		filepath.Join("internal", "repository", "valkey", "lua", filename),
		filepath.Join("threadify-go", "internal", "repository", "valkey", "lua", filename),
		filepath.Join("/Users/martins2/Downloads/ThreadifyEngine/threadify-go", "internal", "repository", "valkey", "lua", filename),
	}

	for _, path := range possiblePaths {
		if data, err := os.ReadFile(path); err == nil {
			return string(data), nil
		}
	}

	return "", fmt.Errorf("failed to load Lua script: %s", filename)
}

// RecordStepEventDirect records a step event immediately without batching
// Also processes sub-steps if provided
func (ses *StepEventService) RecordStepEventDirect(event models.StepEvent, ownerID, serviceName string, subSteps []models.SubStepRequest) error {
	// Track overall step event latency
	startTime := perf.Now()
	stepEventID := fmt.Sprintf("%s:%s:%s", event.ThreadID, event.StepName, event.IdempotencyKey)
	perf.Log("[PERF] StepEvent START: %s | thread=%s | step=%s", stepEventID, event.ThreadID, event.StepName)

	defer func() {
		duration := perf.Since(startTime)
		perf.Log("[PERF] StepEvent COMPLETE: %s | duration=%v", stepEventID, duration)
	}()

	// 1. Validate the step event
	validationStart := perf.Now()
	if err := ses.validateStepEvent(event); err != nil {
		// User input validation errors are safe to expose with context
		perf.Log("[PERF] StepEvent VALIDATION FAILED: %s | duration=%v | error=%v", stepEventID, perf.Since(validationStart), err)
		return fmt.Errorf("invalid step data: %w", err)
	}
	perf.Log("[PERF] StepEvent VALIDATION: %s | duration=%v", stepEventID, perf.Since(validationStart))

	// 2. Execute atomic hash generation via Lua script
	hashStart := perf.Now()
	hashResult, err := ses.executeAtomicHashScript(event, ownerID, serviceName)
	hashDuration := perf.Since(hashStart)
	perf.Log("[PERF] StepEvent HASH_GENERATION: %s | duration=%v | success=%t", stepEventID, hashDuration, err == nil)

	if err != nil {
		return err // Already sanitized by executeAtomicHashScript
	}

	// 3. Send activity event to NATS for archival (SYNCHRONOUS - critical for audit trail)
	// Skip if NATS publisher is not available (graceful degradation)
	if ses.natsPublisher != nil {
		natsStart := perf.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		activityEvent := ses.createActivityEvent(hashResult, event, ownerID, serviceName)

		if err := ses.natsPublisher.PublishActivityLog(ctx, activityEvent); err != nil {
			natsDuration := perf.Since(natsStart)
			perf.Log("[PERF] StepEvent NATS_PUBLISH FAILED: %s | duration=%v | error=%v", stepEventID, natsDuration, err)
			logInternalErrorWithDetails("PublishActivityLog", fmt.Sprintf("stepId=%s", event.StepID), err)
		} else {
			perf.Log("[PERF] StepEvent NATS_PUBLISH: %s | duration=%v", stepEventID, perf.Since(natsStart))
		}
	} else {
		perf.Log("[PERF] StepEvent NATS_PUBLISH: %s | SKIPPED (no publisher)", stepEventID)
	}

	// 4. Process sub-steps if provided
	if len(subSteps) > 0 && ses.natsPublisher != nil {
		subStepsStart := perf.Now()
		if err := ses.processSubSteps(event.ThreadID, event.StepID, subSteps); err != nil {
			perf.Log("[PERF] StepEvent SUBSTEPS_PROCESS FAILED: %s | duration=%v | error=%v", stepEventID, perf.Since(subStepsStart), err)
			// Don't fail the main step if sub-steps fail
		} else {
			perf.Log("[PERF] StepEvent SUBSTEPS_PROCESS: %s | count=%d | duration=%v", stepEventID, len(subSteps), perf.Since(subStepsStart))
		}
	}

	return nil
}

// processSubSteps publishes sub-steps to NATS for archival
func (ses *StepEventService) processSubSteps(threadID, stepID string, subSteps []models.SubStepRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create sub-step events for NATS
	subStepEvents := make([]map[string]interface{}, 0, len(subSteps))

	for _, subStep := range subSteps {
		// Parse recordedAt timestamp
		recordedAt, err := time.Parse(time.RFC3339, subStep.RecordedAt)
		if err != nil {
			log.Printf("[WARN] Invalid sub-step recordedAt timestamp: %v", err)
			recordedAt = time.Now()
		}

		subStepEvent := map[string]interface{}{
			"thread_id":   threadID,
			"step_id":     stepID,
			"name":        subStep.Name,
			"status":      subStep.Status,
			"payload":     subStep.Payload,
			"recorded_at": recordedAt.Format(time.RFC3339Nano),
		}
		subStepEvents = append(subStepEvents, subStepEvent)
	}

	// Publish all sub-steps as a batch
	batchEvent := map[string]interface{}{
		"type":      "substeps_batch",
		"thread_id": threadID,
		"step_id":   stepID,
		"substeps":  subStepEvents,
	}

	return ses.natsPublisher.PublishActivityLog(ctx, batchEvent)
}

// HashResult contains the result of atomic hash generation
type HashResult struct {
	OldHash  string
	NewHash  string
	ThreadID string
}

// executeAtomicHashScript performs atomic hash generation and thread metadata update via Lua script
// This MUST be atomic to prevent race conditions in the hash chain
func (ses *StepEventService) executeAtomicHashScript(event models.StepEvent, ownerID, serviceName string) (*HashResult, error) {
	stepEventID := event.ThreadID + ":" + event.StepName + ":" + event.IdempotencyKey

	// Get hash configuration
	version := ses.config.Security.HashChainCurrentVersion
	if version == "" {
		perf.Log("[PERF] StepEvent HASH_CALC_FAILED: %s | version not configured", stepEventID)
		logInternalError("executeAtomicHashScript", fmt.Errorf("hash_chain_current_version not configured"))
		return nil, fmt.Errorf("failed to process step event")
	}

	secret := ses.config.Security.HashChainSecrets[version]
	if secret == "" {
		perf.Log("[PERF] StepEvent HASH_CALC_FAILED: %s | secret version %s not configured", stepEventID, version)
		logInternalError("executeAtomicHashScript", fmt.Errorf("hash secret version %s not configured", version))
		return nil, fmt.Errorf("failed to process step event")
	}

	// Get old hash, calculate new hash, and update - with retry on race condition
	var oldHash, newHash string
	maxRetries := 3
	atomicStart := perf.Now()
	threadKey := "thread:" + event.ThreadID

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Load and execute get hash script
		getScript, err := loadLuaScript("get_thread_hash.lua")
		if err != nil {
			perf.Log("[PERF] StepEvent LOAD_SCRIPT_FAILED: %s | attempt=%d | error=%v", stepEventID, attempt+1, err)
			if attempt == maxRetries-1 {
				logInternalErrorWithDetails("executeAtomicHashScript", "threadId="+event.ThreadID, err)
				return nil, sanitizeError(err)
			}
			continue
		}

		result, err := ses.valkeyRepo.Eval(context.Background(), getScript, []string{threadKey})
		if err != nil {
			perf.Log("[PERF] StepEvent GET_HASH FAILED: %s | attempt=%d | error=%v", stepEventID, attempt+1, err)
			if attempt == maxRetries-1 {
				logInternalErrorWithDetails("executeAtomicHashScript", "threadId="+event.ThreadID, err)
				return nil, sanitizeError(err)
			}
			continue
		}

		resultSlice, ok := result.([]interface{})
		if !ok || len(resultSlice) < 1 {
			perf.Log("[PERF] StepEvent LUA_PARSE_FAILED: %s | unexpected result format: %v", stepEventID, result)
			return nil, fmt.Errorf("failed to process step event")
		}

		oldHash, _ = resultSlice[0].(string)

		// Calculate new hash using contentHash for content integrity verification
		// Note: event.Timestamp becomes recorded_at in PostgreSQL, so formats must match exactly
		recordedAt := event.Timestamp.Format(time.RFC3339Nano)
		h := hmac.New(sha256.New, []byte(secret))
		hashData := oldHash + ":" + event.ThreadID + ":" + event.StepID + ":" + event.StepName + ":" + event.ContentHash + ":" + recordedAt
		h.Write([]byte(hashData))
		newHash = "hmac-sha256-" + version + ":" + fmt.Sprintf("%x", h.Sum(nil))

		// Load and execute update hash script with optimistic locking
		updateScript, err := loadLuaScript("update_thread_hash.lua")
		if err != nil {
			perf.Log("[PERF] StepEvent LOAD_SCRIPT_FAILED: %s | attempt=%d | error=%v", stepEventID, attempt+1, err)
			if attempt == maxRetries-1 {
				logInternalErrorWithDetails("executeAtomicHashScript", "threadId="+event.ThreadID, err)
				return nil, sanitizeError(err)
			}
			continue
		}

		result, err = ses.valkeyRepo.Eval(
			context.Background(),
			updateScript,
			[]string{threadKey},
			oldHash,
			newHash,
			version,
		)

		if err != nil {
			perf.Log("[PERF] StepEvent UPDATE_HASH FAILED: %s | attempt=%d | error=%v", stepEventID, attempt+1, err)
			if attempt == maxRetries-1 {
				logInternalErrorWithDetails("executeAtomicHashScript", "threadId="+event.ThreadID, err)
				return nil, sanitizeError(err)
			}
			continue
		}

		// Check if update succeeded
		updateResult, ok := result.([]interface{})
		if !ok || len(updateResult) < 1 {
			perf.Log("[PERF] StepEvent UPDATE_PARSE_FAILED: %s | unexpected result: %v", stepEventID, result)
			return nil, fmt.Errorf("failed to process step event")
		}

		success, _ := updateResult[0].(int64)
		if success == 1 {
			// Success!
			atomicDuration := perf.Since(atomicStart)
			perf.Log("[PERF] StepEvent ATOMIC_HASH: %s | duration=%v | attempts=%d", stepEventID, atomicDuration, attempt+1)
			break
		}

		// Race condition detected, retry
		perf.Log("[PERF] StepEvent HASH_RACE_DETECTED: %s | attempt=%d | retrying...", stepEventID, attempt+1)
		if attempt == maxRetries-1 {
			return nil, fmt.Errorf("failed to update hash after %d attempts (race condition)", maxRetries)
		}
	}

	return &HashResult{
		OldHash:  oldHash,
		NewHash:  newHash,
		ThreadID: event.ThreadID,
	}, nil
}

// createActivityEvent creates an activity event with all required fields for NATS publishing
func (ses *StepEventService) createActivityEvent(hashResult *HashResult, event models.StepEvent, ownerID, serviceName string) map[string]interface{} {
	timestampStr := event.Timestamp.Format(time.RFC3339Nano)

	activityValues := map[string]interface{}{
		"type":            "step_recorded",
		"thread_id":       event.ThreadID,
		"step_id":         fmt.Sprintf("%s:%s", event.StepName, event.IdempotencyKey),
		"step_name":       event.StepName,
		"step_uuid":       event.StepID,
		"idempotency_key": event.IdempotencyKey,
		"content_hash":    event.ContentHash,
		"timestamp":       timestampStr,
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
