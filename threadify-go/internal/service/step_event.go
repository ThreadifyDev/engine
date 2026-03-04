package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
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

type StepEventService struct {
	valkeyRepo       interfaces.ValkeyClient
	threadRepo       *valkey.ThreadRepository
	natsPublisher    *natsrepo.ArchivalPublisher
	config           *config.Config
	logger           *zap.Logger
	getHashScript    string
	updateHashScript string
}

func NewStepEventService(
	valkeyRepo interfaces.ValkeyClient,
	threadRepo *valkey.ThreadRepository,
	natsPublisher *natsrepo.ArchivalPublisher,
	cfg *config.Config,
	logger *zap.Logger,
) (*StepEventService, error) {
	getScript, err := loadLuaScript("get_thread_hash.lua")
	if err != nil {
		return nil, fmt.Errorf("load get_thread_hash.lua: %w", err)
	}
	updateScript, err := loadLuaScript("update_thread_hash.lua")
	if err != nil {
		return nil, fmt.Errorf("load update_thread_hash.lua: %w", err)
	}

	return &StepEventService{
		valkeyRepo:       valkeyRepo,
		threadRepo:       threadRepo,
		natsPublisher:    natsPublisher,
		config:           cfg,
		logger:           logger,
		getHashScript:    getScript,
		updateHashScript: updateScript,
	}, nil
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
	return "", fmt.Errorf("lua script not found: %s", filename)
}

func (ses *StepEventService) RecordStepEventDirect(ctx context.Context, event models.StepEvent, ownerID, serviceName string, subSteps []models.SubStepRequest) error {
	if err := ses.validateStepEvent(event); err != nil {
		return fmt.Errorf("invalid step data: %w", err)
	}

	hashResult, err := ses.executeAtomicHashScript(ctx, event, ownerID, serviceName)
	if err != nil {
		return err
	}

	if ses.natsPublisher != nil {
		natsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := ses.natsPublisher.PublishActivityLog(natsCtx, ses.createActivityEvent(hashResult, event, ownerID, serviceName)); err != nil {
			ses.logger.Error("nats_publish failed", zap.String("step_id", event.StepID), zap.Error(err))
		}
		cancel()

		if len(subSteps) > 0 {
			if err := ses.processSubSteps(ctx, event.ThreadID, event.StepID, subSteps); err != nil {
				ses.logger.Error("sub-steps processing failed",
					zap.String("thread_id", event.ThreadID),
					zap.String("step_id", event.StepID),
					zap.Error(err),
				)
			}
		}
	}

	return nil
}

func (ses *StepEventService) processSubSteps(ctx context.Context, threadID, stepID string, subSteps []models.SubStepRequest) error {
	pubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	events := make([]map[string]interface{}, 0, len(subSteps))
	for _, ss := range subSteps {
		recordedAt, err := time.Parse(time.RFC3339, ss.RecordedAt)
		if err != nil {
			return fmt.Errorf("invalid sub-step recordedAt %q: %w", ss.RecordedAt, err)
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

type HashResult struct {
	OldHash  string
	NewHash  string
	ThreadID string
}

func (ses *StepEventService) executeAtomicHashScript(ctx context.Context, event models.StepEvent, ownerID, serviceName string) (*HashResult, error) {
	version := ses.config.Security.HashChainCurrentVersion
	if version == "" {
		ses.logger.Error("hash_chain_current_version not configured")
		return nil, ErrFailedToProcessStep
	}

	secret := ses.config.Security.HashChainSecrets[version]
	if secret == "" {
		ses.logger.Error("hash secret mapping missing", zap.String("version", version))
		return nil, ErrFailedToProcessStep
	}

	threadKey := "thread:" + event.ThreadID
	recordedAt := event.Timestamp.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)

	const maxRetries = 10
	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := ses.valkeyRepo.Eval(ctx, ses.getHashScript, []string{threadKey})
		if err != nil {
			if attempt == maxRetries-1 {
				ses.logger.Error("get hash eval failed", zap.String("thread_id", event.ThreadID), zap.Error(err))
				return nil, sanitizeError(err)
			}
			continue
		}

		resultSlice, ok := result.([]interface{})
		if !ok || len(resultSlice) < 1 {
			return nil, ErrFailedToProcessStep
		}
		oldHash, _ := resultSlice[0].(string)

		h := hmac.New(sha256.New, []byte(secret))
		h.Write([]byte(oldHash + ":" + event.ThreadID + ":" + event.StepID + ":" + event.StepName + ":" + event.ContentHash + ":" + recordedAt))
		newHash := fmt.Sprintf("hmac-sha256-%s:%x", version, h.Sum(nil))

		result, err = ses.valkeyRepo.Eval(ctx, ses.updateHashScript, []string{threadKey}, oldHash, newHash, version)
		if err != nil {
			if attempt == maxRetries-1 {
				ses.logger.Error("update hash eval failed", zap.String("thread_id", event.ThreadID), zap.Error(err))
				return nil, sanitizeError(err)
			}
			continue
		}

		updateResult, ok := result.([]interface{})
		if !ok || len(updateResult) < 1 {
			return nil, ErrFailedToProcessStep
		}

		if success, _ := updateResult[0].(int64); success == 1 {
			return &HashResult{OldHash: oldHash, NewHash: newHash, ThreadID: event.ThreadID}, nil
		}

		perf.LogStructured("StepEvent HASH_RACE_DETECTED",
			zap.String("thread_id", event.ThreadID),
			zap.Int("attempt", attempt+1),
		)
		select {
		case <-time.After(time.Duration(attempt+1) * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed to update hash after %d attempts (race condition)", maxRetries)
}

func (ses *StepEventService) createActivityEvent(hashResult *HashResult, event models.StepEvent, ownerID, serviceName string) map[string]interface{} {
	activityValues := map[string]interface{}{
		"type":            "step_recorded",
		"threadId":        event.ThreadID,
		"stepId":          fmt.Sprintf("%s:%s", event.StepName, event.IdempotencyKey),
		"step_uuid":       event.StepID,
		"step_name":       event.StepName,
		"idempotency_key": event.IdempotencyKey,
		"contentHash":     event.ContentHash,
		"timestamp":       event.Timestamp.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano),
		"context":         event.ContextJSON(),
		"actor":           ownerID,
		"actorService":    serviceName,
		"status":          event.Status,
		"hash":            hashResult.NewHash,
		"prevHash":        hashResult.OldHash,
		"startedAt":       event.StartedAt,
		"finishedAt":      event.FinishedAt,
	}

	// Add metadata if present (marshal to JSON string)
	if event.Metadata != nil && len(event.Metadata) > 0 {
		if metadataBytes, err := json.Marshal(event.Metadata); err == nil {
			activityValues["metadata"] = string(metadataBytes)
		}
	}

	return activityValues
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
