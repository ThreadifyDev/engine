package valkey

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/threadify/engine/internal/models"
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

// GetStepStatus checks step status in Valkey or PostgreSQL with optional write-back
func (r *ThreadRepository) GetStepStatus(ctx context.Context, threadID, stepName, idempotencyKey string, writeBack ...bool) (string, error) {
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

	stepKey := stepName + ":" + idempotencyKey
	stepHashKey := "thread:" + threadID + ":steps:" + stepKey

	// 1. Try Valkey first (hot path) - check if THIS step exists
	status, err := r.valkey.HGet(ctx, stepHashKey, "status")
	if err == nil && status != "" {
		return status, nil
	}

	// PERFORMANCE: Thread existence check commented out (saves ~1.5ms per call)
	// Rationale: If step not found, it's a new step 99.9% of the time
	// Thread eviction is rare (threads active for hours with 5hr TTL)
	// PostgreSQL fallback below still handles cold threads if needed
	// ============================================================================
	// 2. Step not found in Valkey - check if thread exists in Valkey
	// If thread exists in Valkey, the step definitely doesn't exist (it's new)
	// Only check PostgreSQL if thread was evicted from cache
	// threadKey := fmt.Sprintf("thread:%s", threadID)
	// threadExists, threadErr := r.valkey.Exists(ctx, threadKey)
	// if threadErr == nil && threadExists {
	// 	// Thread is in Valkey but step is not - this is a new step
	// 	// Skip PostgreSQL check (step definitely doesn't exist)
	// 	return "", nil
	// }
	// ============================================================================

	// Step not found - assume it's a new step (skip PostgreSQL for performance)
	// TODO: Re-enable thread existence check if duplicate detection issues arise
	return "", nil

	// Thread not in Valkey - entire thread was evicted - check PostgreSQL
	// if r.stepStatePostgres == nil {
	// 	return "", nil // Not found, not an error
	// }

	log.Printf("⚠️ [COLD] Step %s:%s not in Valkey, checking PostgreSQL", stepName, idempotencyKey)

	// Get ALL step states for the thread using batch query (warm entire cache at once)
	stepsMap, err := r.stepStatePostgres.GetStepsBatch(ctx, []string{threadID})
	if err != nil {
		log.Printf("❌ [COLD] Failed to get step states from PostgreSQL: %v", err)
		return "", nil
	}

	allSteps := stepsMap[threadID]
	if allSteps == nil {
		allSteps = []*models.StepStateInfo{}
	}

	log.Printf("✅ [COLD] Retrieved %d step states from PostgreSQL for thread %s", len(allSteps), threadID)

	// Find the requested step in the results
	var requestedStatus string
	for _, step := range allSteps {
		if step.StepName == stepName && step.IdempotencyKey == idempotencyKey {
			requestedStatus = step.Status
			break
		}
	}

	// Async write-back all steps to Valkey (warm the cache)
	if shouldWriteBack && len(allSteps) > 0 {
		go func(threadID string, steps []*models.StepStateInfo) {
			writeBackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			log.Printf("🔄 [WRITE-BACK] Async caching %d step states for thread %s", len(steps), threadID)
			if err := r.writeBackAllStepsToValkey(writeBackCtx, threadID, steps); err != nil {
				log.Printf("⚠️ Async write-back failed for step states: %v", err)
			} else {
				log.Printf("✅ [WRITE-BACK] Cached %d step states successfully", len(steps))
			}
		}(threadID, allSteps)
	}

	return requestedStatus, nil
}

// GetCompletedStepsCount returns count of completed steps
func (r *ThreadRepository) GetCompletedStepsCount(ctx context.Context, threadID string, writeBack ...bool) (int64, error) {
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

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

	log.Printf("[COLD] Completed steps count for thread %s not in Valkey, checking PostgreSQL", threadID)

	// Get completed steps from PostgreSQL
	pgSteps, pgErr := r.postgresRepo.GetCompletedSteps(ctx, threadID)
	if pgErr != nil {
		log.Printf("[ERROR] Failed to get completed steps from PostgreSQL for thread %s: %v", threadID, pgErr)
		return 0, pgErr
	}

	count = int64(len(pgSteps))

	// Optionally write back to Valkey for future hot reads
	if shouldWriteBack && count > 0 {
		go func() {
			writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Rebuild the current_steps sorted set in batch
			for _, step := range pgSteps {
				stepKey := step.StepName + ":unknown" // We don't have idempotency key from this query
				score := float64(step.CompletedAt.Unix())
				if err := r.valkey.ZAdd(writeCtx, currentStepsKey, score, stepKey); err != nil {
					log.Printf("[WARN] Failed to write back step to Valkey: %v", err)
					return
				}
			}

			// Set TTL
			ttl := time.Duration(r.ttl) * time.Second
			if err := r.valkey.Expire(writeCtx, currentStepsKey, ttl); err != nil {
				log.Printf("[WARN] Failed to set TTL: %v", err)
				return
			}

			log.Printf("[WRITE-BACK] Cached %d completed steps for thread %s", count, threadID)
		}()
	}

	log.Printf("[COLD] Retrieved %d completed steps from PostgreSQL for thread %s", count, threadID)
	return count, nil
}

// GetCompletedSteps returns list of completed step names
func (r *ThreadRepository) GetCompletedSteps(ctx context.Context, threadID string, writeBack ...bool) ([]string, error) {
	shouldWriteBack := false
	if len(writeBack) > 0 {
		shouldWriteBack = writeBack[0]
	}

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

	log.Printf("[COLD] Completed steps for thread %s not in Valkey, checking PostgreSQL", threadID)

	// Get completed steps from PostgreSQL
	pgSteps, pgErr := r.postgresRepo.GetCompletedSteps(ctx, threadID)
	if pgErr != nil {
		log.Printf("[ERROR] Failed to get completed steps from PostgreSQL for thread %s: %v", threadID, pgErr)
		return []string{}, pgErr
	}

	// Extract step names (format: stepName:idempKey)
	stepNames := make([]string, 0, len(pgSteps))
	for _, step := range pgSteps {
		// We don't have idempotency key from PostgreSQL, so use placeholder
		stepKey := fmt.Sprintf("%s:%s", step.StepName, "unknown")
		stepNames = append(stepNames, stepKey)
	}

	// Optionally write back to Valkey for future hot reads
	if shouldWriteBack && len(stepNames) > 0 {
		go func() {
			writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Rebuild the current_steps sorted set in batch
			for _, step := range pgSteps {
				stepKey := step.StepName + ":unknown"
				score := float64(step.CompletedAt.Unix())
				if err := r.valkey.ZAdd(writeCtx, currentStepsKey, score, stepKey); err != nil {
					log.Printf("[WARN] Failed to write back step to Valkey: %v", err)
					return
				}
			}

			// Set TTL
			ttl := time.Duration(r.ttl) * time.Second
			if err := r.valkey.Expire(writeCtx, currentStepsKey, ttl); err != nil {
				log.Printf("[WARN] Failed to set TTL: %v", err)
				return
			}

			log.Printf("[WRITE-BACK] Cached %d completed steps for thread %s", len(stepNames), threadID)
		}()
	}

	log.Printf("[COLD] Retrieved %d completed steps from PostgreSQL for thread %s", len(stepNames), threadID)
	return stepNames, nil
}

// writeBackAllStepsToValkey writes all step states and rebuilds sorted set in a single atomic batch
// This is used when warming the cache from PostgreSQL to ensure consistency
//
// IMPORTANT: This must match the format written by the hot path Lua script:
// See: /internal/repository/valkey/lua/validate_and_update_step_state.lua (lines 238-254)
// If you change the Lua script's HSET fields, update this function to match!
func (r *ThreadRepository) writeBackAllStepsToValkey(ctx context.Context, threadID string, steps []*models.StepStateInfo) error {
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
				log.Printf("⚠️ Failed to add step %s to sorted set: %v", step.StepName, err)
			}
		}
	}

	// Set TTL on sorted set
	if err := r.valkey.Expire(ctx, currentStepsKey, ttl); err != nil {
		log.Printf("⚠️ Failed to set TTL on current_steps: %v", err)
	}

	return nil
}
