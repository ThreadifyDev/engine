package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/perf"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
	"go.uber.org/zap"
)

// StepEventService handles step event processing with cryptographic hashing.
// Hash generation is handled atomically via Lua scripts rather than in-memory cache.
type StepEventService struct {
	valkeyRepo    interfaces.ValkeyClient
	threadRepo    *valkey.ThreadRepository
	natsPublisher *natsrepo.ArchivalPublisher
	config        *config.Config
	logger        *zap.Logger
}

// NewStepEventService creates a new step event service.
func NewStepEventService(
	valkeyRepo interfaces.ValkeyClient,
	threadRepo *valkey.ThreadRepository,
	natsPublisher *natsrepo.ArchivalPublisher,
	cfg *config.Config,
	logger *zap.Logger,
) *StepEventService {
	return &StepEventService{
		valkeyRepo:    valkeyRepo,
		threadRepo:    threadRepo,
		natsPublisher: natsPublisher,
		config:        cfg,
		logger:        logger,
	}
}

// Start is a no-op for direct write mode.
func (ses *StepEventService) Start() error { return nil }

// Stop is a no-op for direct write mode.
func (ses *StepEventService) Stop() error { return nil }

// loadLuaScript loads a Lua script from the valkey/lua directory.
func loadLuaScript(filename string) (string, error) {
	paths := []string{
		filepath.Join("internal", "repository", "valkey", "lua", filename),
		filepath.Join("threadify-go", "internal", "repository", "valkey", "lua", filename),
	}
	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("failed to load Lua script: %s", filename)
}

// RecordStepEventDirect records a step event immediately without batching.
// Also processes sub-steps if provided.
func (ses *StepEventService) RecordStepEventDirect(ctx context.Context, event models.StepEvent, ownerID, serviceName string, subSteps []models.SubStepRequest) error {
	startTime := perf.Now()
	stepEventID := fmt.Sprintf("%s:%s:%s", event.ThreadID, event.StepName, event.IdempotencyKey)
	perf.LogStructured("StepEvent START",
		zap.String("event_id", stepEventID),
		zap.String("thread", event.ThreadID),
		zap.String("step", event.StepName),
	)
	defer func() {
		perf.LogStructured("StepEvent COMPLETE",
			zap.String("event_id", stepEventID),
			zap.Duration("duration", perf.Since(startTime)),
		)
	}()

	// 1. Validate.
	validationStart := perf.Now()
	if err := ses.validateStepEvent(event); err != nil {
		perf.LogStructured("StepEvent VALIDATION FAILED",
			zap.String("event_id", stepEventID),
			zap.Duration("duration", perf.Since(validationStart)),
			zap.Error(err),
		)
		return fmt.Errorf("invalid step data: %w", err)
	}
	perf.LogStructured("StepEvent VALIDATION",
		zap.String("event_id", stepEventID),
		zap.Duration("duration", perf.Since(validationStart)),
	)

	// 2. Atomic hash generation via Lua script.
	hashStart := perf.Now()
	hashResult, err := ses.executeAtomicHashScript(ctx, event, ownerID, serviceName)
	perf.LogStructured("StepEvent HASH_GENERATION",
		zap.String("event_id", stepEventID),
		zap.Duration("duration", perf.Since(hashStart)),
		zap.Bool("success", err == nil),
	)
	if err != nil {
		return err // already sanitised by executeAtomicHashScript
	}

	// 3. Publish activity event to NATS (SYNCHRONOUS — critical for audit trail).
	if ses.natsPublisher != nil {
		natsStart := perf.Now()
		natsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		if err := ses.natsPublisher.PublishActivityLog(natsCtx, ses.createActivityEvent(hashResult, event, ownerID, serviceName)); err != nil {
			perf.LogStructured("StepEvent NATS_PUBLISH FAILED",
				zap.String("event_id", stepEventID),
				zap.Duration("duration", perf.Since(natsStart)),
				zap.Error(err),
			)
			ses.logger.Error("nats_publish failed", zap.String("step_id", event.StepID), zap.Error(err))
		} else {
			perf.LogStructured("StepEvent NATS_PUBLISH",
				zap.String("event_id", stepEventID),
				zap.Duration("duration", perf.Since(natsStart)),
			)
		}
	} else {
		perf.LogStructured("StepEvent NATS_PUBLISH SKIPPED", zap.String("event_id", stepEventID))
	}

	// 4. Process sub-steps.
	if len(subSteps) > 0 && ses.natsPublisher != nil {
		subStepsStart := perf.Now()
		if err := ses.processSubSteps(ctx, event.ThreadID, event.StepID, subSteps); err != nil {
			perf.LogStructured("StepEvent SUBSTEPS_PROCESS FAILED",
				zap.String("event_id", stepEventID),
				zap.Duration("duration", perf.Since(subStepsStart)),
				zap.Error(err),
			)
			// Sub-step failure does not fail the main step.
		} else {
			perf.LogStructured("StepEvent SUBSTEPS_PROCESS",
				zap.String("event_id", stepEventID),
				zap.Int("count", len(subSteps)),
				zap.Duration("duration", perf.Since(subStepsStart)),
			)
		}
	}

	return nil
}

// processSubSteps publishes sub-steps to NATS as a batch for archival.
func (ses *StepEventService) processSubSteps(ctx context.Context, threadID, stepID string, subSteps []models.SubStepRequest) error {
	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	events := make([]map[string]interface{}, 0, len(subSteps))
	for _, ss := range subSteps {
		recordedAt, err := time.Parse(time.RFC3339, ss.RecordedAt)
		if err != nil {
			ses.logger.Warn("invalid sub-step recordedAt, using current time", zap.Error(err))
			recordedAt = time.Now()
		}
		events = append(events, map[string]interface{}{
			"threadId":   threadID,
			"stepId":     stepID,
			"name":       ss.Name,
			"status":     ss.Status,
			"payload":    ss.Payload,
			"recordedAt": recordedAt.Format(time.RFC3339Nano),
		})
	}

	return ses.natsPublisher.PublishActivityLog(pubCtx, map[string]interface{}{
		"type":     "substeps_batch",
		"threadId": threadID,
		"stepId":   stepID,
		"substeps": events,
	})
}

// HashResult contains the result of atomic hash generation.
type HashResult struct {
	OldHash  string
	NewHash  string
	ThreadID string
}

// executeAtomicHashScript performs atomic hash generation and thread metadata update via Lua script.
// This MUST be atomic to prevent race conditions in the hash chain.
func (ses *StepEventService) executeAtomicHashScript(ctx context.Context, event models.StepEvent, ownerID, serviceName string) (*HashResult, error) {
	stepEventID := event.ThreadID + ":" + event.StepName + ":" + event.IdempotencyKey

	version := ses.config.Security.HashChainCurrentVersion
	if version == "" {
		perf.LogStructured("StepEvent HASH_CALC_FAILED",
			zap.String("event_id", stepEventID),
			zap.String("reason", "version not configured"),
		)
		ses.logger.Error("hash_chain_current_version not configured")
		return nil, ErrFailedToProcessStep
	}

	secret := ses.config.Security.HashChainSecrets[version]
	if secret == "" {
		perf.LogStructured("StepEvent HASH_CALC_FAILED",
			zap.String("event_id", stepEventID),
			zap.String("reason", fmt.Sprintf("secret version %s not configured", version)),
		)
		ses.logger.Error("hash secret mapping missing", zap.String("version", version))
		return nil, ErrFailedToProcessStep
	}

	// Load scripts once before the retry loop — content never changes between attempts.
	getScript, err := loadLuaScript("get_thread_hash.lua")
	if err != nil {
		ses.logger.Error("failed to load get_thread_hash.lua", zap.Error(err))
		return nil, sanitizeError(err)
	}
	updateScript, err := loadLuaScript("update_thread_hash.lua")
	if err != nil {
		ses.logger.Error("failed to load update_thread_hash.lua", zap.Error(err))
		return nil, sanitizeError(err)
	}

	threadKey := "thread:" + event.ThreadID
	recordedAt := event.Timestamp.Format(time.RFC3339Nano)
	const maxRetries = 3
	atomicStart := perf.Now()

	var oldHash, newHash string

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Fetch current hash.
		result, err := ses.valkeyRepo.Eval(ctx, getScript, []string{threadKey})
		if err != nil {
			perf.LogStructured("StepEvent GET_HASH FAILED",
				zap.String("event_id", stepEventID),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			if attempt == maxRetries-1 {
				ses.logger.Error("get hash eval failed", zap.String("thread_id", event.ThreadID), zap.Error(err))
				return nil, sanitizeError(err)
			}
			continue
		}

		resultSlice, ok := result.([]interface{})
		if !ok || len(resultSlice) < 1 {
			perf.LogStructured("StepEvent LUA_PARSE_FAILED",
				zap.String("event_id", stepEventID),
				zap.Any("result", result),
			)
			return nil, ErrFailedToProcessStep
		}
		oldHash, _ = resultSlice[0].(string)

		// Compute new hash.
		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(oldHash + ":" + event.ThreadID + ":" + event.StepID + ":" + event.StepName + ":" + event.ContentHash + ":" + recordedAt))
		newHash = fmt.Sprintf("hmac-sha256-%s:%x", version, h.Sum(nil))

		// Attempt atomic update with optimistic locking.
		result, err = ses.valkeyRepo.Eval(ctx, updateScript, []string{threadKey}, oldHash, newHash, version)
		if err != nil {
			perf.LogStructured("StepEvent UPDATE_HASH FAILED",
				zap.String("event_id", stepEventID),
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			if attempt == maxRetries-1 {
				ses.logger.Error("update hash eval failed", zap.String("thread_id", event.ThreadID), zap.Error(err))
				return nil, sanitizeError(err)
			}
			continue
		}

		updateResult, ok := result.([]interface{})
		if !ok || len(updateResult) < 1 {
			perf.LogStructured("StepEvent UPDATE_PARSE_FAILED",
				zap.String("event_id", stepEventID),
				zap.Any("result", result),
			)
			return nil, ErrFailedToProcessStep
		}

		if success, _ := updateResult[0].(int64); success == 1 {
			perf.LogStructured("StepEvent ATOMIC_HASH",
				zap.String("event_id", stepEventID),
				zap.Duration("duration", perf.Since(atomicStart)),
				zap.Int("attempts", attempt+1),
			)
			return &HashResult{OldHash: oldHash, NewHash: newHash, ThreadID: event.ThreadID}, nil
		}

		// Hash changed between get and update — retry.
		perf.LogStructured("StepEvent HASH_RACE_DETECTED",
			zap.String("event_id", stepEventID),
			zap.Int("attempt", attempt+1),
		)
	}

	return nil, fmt.Errorf("failed to update hash after %d attempts (race condition)", maxRetries)
}

// createActivityEvent builds the activity event payload for NATS publishing.
func (ses *StepEventService) createActivityEvent(hashResult *HashResult, event models.StepEvent, ownerID, serviceName string) map[string]interface{} {
	return map[string]interface{}{
		"type":           "step_recorded",
		"threadId":       event.ThreadID,
		"stepId":         fmt.Sprintf("%s:%s", event.StepName, event.IdempotencyKey),
		"stepName":       event.StepName,
		"stepUuid":       event.StepID,
		"idempotencyKey": event.IdempotencyKey,
		"contentHash":    event.ContentHash,
		"timestamp":      event.Timestamp.Format(time.RFC3339Nano),
		"context":        event.ContextJSON(),
		"actor":          ownerID,
		"actorService":   serviceName,
		"status":         event.Status,
		"hash":           hashResult.NewHash,
		"prevHash":       hashResult.OldHash,
		"startedAt":      event.StartedAt,
		"finishedAt":     event.FinishedAt,
	}
}

func (ses *StepEventService) validateStepEvent(event models.StepEvent) error {
	if event.StepID == "" {
		return ErrStepIdRequired
	}
	if event.ThreadID == "" {
		return ErrThreadIdRequired
	}
	if event.Context == nil {
		return ErrContextRequired
	}
	return nil
}
