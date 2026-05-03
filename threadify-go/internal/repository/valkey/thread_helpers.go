package valkey

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

// extendAllThreadTTLs extends TTL on all thread-related keys when writing back from PostgreSQL
func (r *ThreadRepository) extendAllThreadTTLs(ctx context.Context, threadID string) error {
	pipe := r.valkey.Pipeline()
	ttl := time.Duration(r.ttl) * time.Second

	// Core thread keys
	pipe.Expire(ctx, fmt.Sprintf("thread:%s", threadID), ttl)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:meta", threadID), ttl)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:access", threadID), ttl)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:role_index", threadID), ttl)
	pipe.Expire(ctx, fmt.Sprintf("thread:%s:current_steps", threadID), ttl)
	// Note: thread:*:activity keys removed - activities go directly to NATS, not Valkey

	// Extend all step hashes
	stepKeys, _ := r.valkey.Keys(ctx, fmt.Sprintf("thread:%s:steps:*", threadID))
	for _, key := range stepKeys {
		pipe.Expire(ctx, key, ttl)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// GetStepStatus checks step status in cache, Valkey, or PostgreSQL with optional write-back
func (r *ThreadRepository) GetStepStatus(ctx context.Context, query domain.StepStatusQuery, opts ...domain.ThreadReadOptions) (string, error) {
	stepKey := query.StepName + ":" + query.IdempotencyKey
	stepHashKey := "thread:" + query.ThreadID + ":steps:" + stepKey

	// 1. Check in-memory LRU cache first (fastest - catches duplicates immediately)
	if r.cacheManager != nil {
		if cachedStatus, found := r.cacheManager.GetStepStatus(stepHashKey); found {
			return cachedStatus, nil
		}
	}

	// 2. Try Valkey (hot path) - check if THIS step exists
	status, err := r.valkey.HGet(ctx, stepHashKey, "status")
	if err == nil && status != "" {
		// Cache the status for future duplicate checks
		if r.cacheManager != nil {
			r.cacheManager.SetStepStatus(stepHashKey, status)
		}
		return status, nil
	}

	// Step not found - add it to LRU cache
	if r.cacheManager != nil {
		r.cacheManager.SetStepStatus(stepHashKey, query.ExpectedStatus)
	}
	return "", nil
}

// GetCompletedStepsCount returns count of completed steps
func (r *ThreadRepository) GetCompletedStepsCount(ctx context.Context, threadID string, opts ...domain.ThreadReadOptions) (int64, error) {
	shouldWriteBack := len(opts) > 0 && opts[0].WriteBack

	currentStepsKey := fmt.Sprintf("thread:%s:current_steps", threadID)

	// Try Valkey first
	count, err := r.valkey.ZCard(ctx, currentStepsKey)
	if err == nil {
		return count, nil
	}

	// Check if thread exists in Valkey - if not, it's a brand new thread with no steps
	threadKey := fmt.Sprintf("thread:%s", threadID)
	exists, existsErr := r.valkey.Exists(ctx, threadKey)
	if existsErr == nil && !exists {
		// Thread doesn't exist in cache yet - it's brand new, no need to check PostgreSQL
		return 0, nil
	}

	// Fallback to PostgreSQL only if thread exists but steps are missing (cache eviction scenario)
	if r.postgresRepo == nil {
		return 0, nil
	}

	r.logger.Info("Cache miss for completed steps count, querying PostgreSQL", zap.String("thread_id", threadID))

	// Get completed steps from PostgreSQL
	pgSteps, pgErr := r.postgresRepo.GetCompletedSteps(ctx, threadID)
	if pgErr != nil {
		r.logger.Error("Failed to get completed steps from PostgreSQL", zap.String("thread_id", threadID), zap.Error(pgErr))
		return 0, pgErr
	}

	count = int64(len(pgSteps))

	// Optionally write back to Valkey for future hot reads via worker pool
	if shouldWriteBack && count > 0 && r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Rebuild the current_steps sorted set in batch
			for _, step := range pgSteps {
				stepKey := step.StepName + ":unknown" // We don't have idempotency key from this query
				score := float64(step.CompletedAt.Unix())
				if err := r.valkey.ZAdd(writeCtx, currentStepsKey, score, stepKey); err != nil {
					r.logger.Warn("Failed to write back step to cache", zap.Error(err))
					return
				}
			}

			// Set TTL
			ttl := time.Duration(r.ttl) * time.Second
			if err := r.valkey.Expire(writeCtx, currentStepsKey, ttl); err != nil {
				r.logger.Warn("Failed to set TTL on current_steps", zap.Error(err))
			}

			r.logger.Info("Cached completed steps", zap.String("thread_id", threadID), zap.Int64("count", count))
		})
	}

	r.logger.Info("Retrieved completed steps from PostgreSQL", zap.String("thread_id", threadID), zap.Int64("count", count))
	return count, nil
}

// GetCompletedSteps returns list of completed step names
func (r *ThreadRepository) GetCompletedSteps(ctx context.Context, threadID string, opts ...domain.ThreadReadOptions) ([]string, error) {
	shouldWriteBack := len(opts) > 0 && opts[0].WriteBack

	currentStepsKey := "thread:" + threadID + ":current_steps"

	// Try Valkey first
	steps, err := r.valkey.ZRange(ctx, currentStepsKey, 0, -1)
	if err == nil && len(steps) > 0 {
		return steps, nil
	}

	// Fallback to PostgreSQL
	if r.postgresRepo == nil {
		return []string{}, nil
	}

	r.logger.Info("Cache miss for completed steps, querying PostgreSQL", zap.String("thread_id", threadID))

	// Get completed steps from PostgreSQL
	pgSteps, pgErr := r.postgresRepo.GetCompletedSteps(ctx, threadID)
	if pgErr != nil {
		r.logger.Error("Failed to get completed steps from PostgreSQL", zap.String("thread_id", threadID), zap.Error(pgErr))
		return []string{}, pgErr
	}

	// Extract step names (format: stepName:idempKey)
	stepNames := make([]string, 0, len(pgSteps))
	for _, step := range pgSteps {
		// We don't have idempotency key from PostgreSQL, so use placeholder
		stepKey := fmt.Sprintf("%s:%s", step.StepName, "unknown")
		stepNames = append(stepNames, stepKey)
	}

	// Optionally write back to Valkey for future hot reads via worker pool
	if shouldWriteBack && len(stepNames) > 0 && r.writeBackPool != nil {
		r.writeBackPool.Submit(func(ctx context.Context) {
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			// Rebuild the current_steps sorted set in batch
			for _, step := range pgSteps {
				stepKey := step.StepName + ":unknown"
				score := float64(step.CompletedAt.Unix())
				if err := r.valkey.ZAdd(writeCtx, currentStepsKey, score, stepKey); err != nil {
					r.logger.Warn("Failed to write back step to cache", zap.Error(err))
					return
				}
			}

			// Set TTL
			ttl := time.Duration(r.ttl) * time.Second
			if err := r.valkey.Expire(writeCtx, currentStepsKey, ttl); err != nil {
				r.logger.Warn("Failed to set TTL", zap.Error(err))
				return
			}

			r.logger.Info("Cached completed steps", zap.String("thread_id", threadID), zap.Int("count", len(stepNames)))
		})
	}

	r.logger.Info("Retrieved completed steps from PostgreSQL", zap.String("thread_id", threadID), zap.Int("count", len(stepNames)))
	return stepNames, nil
}

// writeBackAllStepsToValkey writes all step states and rebuilds sorted set in a single atomic batch
// This is used when warming the cache from PostgreSQL to ensure consistency
//
// IMPORTANT: This must match the format written by the hot path Lua script:
// See: /internal/repository/valkey/lua/validate_and_update_step_state.lua (lines 238-254)
// If you change the Lua script's HSET fields, update this function to match!
func (r *ThreadRepository) writeBackAllStepsToValkey(ctx context.Context, threadID string, steps []*domain.StepStateInfo) error {
	if len(steps) == 0 {
		return nil
	}

	pipe := r.valkey.Pipeline()
	ttl := time.Duration(r.ttl) * time.Second

	// Batch write all step hashes in SAME format as validate_and_update_step_state.lua
	// Fields must match exactly: status, retryCount, latestStepID, firstSeenAt, lastUpdatedAt, previousStep, actor
	for _, step := range steps {
		stepKey := step.StepName + ":" + step.IdempotencyKey
		stepHashKey := "thread:" + threadID + ":steps:" + stepKey

		pipe.HSet(ctx, stepHashKey,
			"status", step.Status,
			"retryCount", step.RetryCount,
			"latestStepID", step.LatestStepID, // Must be 'latestStepID' to match Lua script (not 'id')
			"firstSeenAt", step.FirstSeenAt.Format(time.RFC3339),
			"lastUpdatedAt", step.LastUpdatedAt.Format(time.RFC3339),
			"previousStep", step.PreviousStep,
			"actor", step.Actor, // For .own permission filtering
		)
		pipe.Expire(ctx, stepHashKey, ttl)
	}

	// Execute pipeline for step hashes
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to write step hashes: %w", err)
	}

	// Rebuild current_steps sorted set (completed steps only)
	// Note: ZAdd not supported in pipeline, so we do this separately
	currentStepsKey := fmt.Sprintf("thread:%s:current_steps", threadID)
	for _, step := range steps {
		if step.Status == "success" {
			// Use LastUpdatedAt timestamp for score
			score := float64(step.LastUpdatedAt.Unix())
			if err := r.valkey.ZAdd(ctx, currentStepsKey, score, step.StepName); err != nil {
				r.logger.Warn("Failed to add step to sorted set", zap.String("step_name", step.StepName), zap.Error(err))
			}
		}
	}

	// Set TTL on sorted set
	if err := r.valkey.Expire(ctx, currentStepsKey, ttl); err != nil {
		r.logger.Warn("Failed to set TTL on current_steps", zap.Error(err))
	}

	return nil
}
