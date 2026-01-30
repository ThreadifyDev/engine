package valkey

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/repository/postgres"
)

// Embed Lua script at compile time
//
//go:embed lua/validate_and_update_step_state.lua
var validateAndUpdateStepStateScript string

// StepStateRepository implements interfaces.StepStateRepository
type StepStateRepository struct {
	client       interfaces.ValkeyClient
	scriptHashes map[string]string
	postgresRepo *postgres.StepStateRepository // For PostgreSQL fallback
	ttl          int                           // TTL in seconds for step keys
}

// NewStepStateRepository creates a new step state repository
func NewStepStateRepository(client interfaces.ValkeyClient, ttl int) interfaces.StepStateRepository {
	repo := &StepStateRepository{
		client:       client,
		scriptHashes: make(map[string]string),
		ttl:          ttl,
	}
	return repo
}

// NewStepStateRepositoryWithPostgres creates a new step state repository with PostgreSQL fallback
func NewStepStateRepositoryWithPostgres(client interfaces.ValkeyClient, postgresRepo *postgres.StepStateRepository, ttl int) *StepStateRepository {
	repo := &StepStateRepository{
		client:       client,
		scriptHashes: make(map[string]string),
		postgresRepo: postgresRepo,
		ttl:          ttl,
	}
	return repo
}

// LoadScripts loads all Lua scripts into Valkey and stores their SHA hashes
// Should be called once during application startup
func (r *StepStateRepository) LoadScripts(ctx context.Context) error {
	sha, err := r.client.ScriptLoad(ctx, validateAndUpdateStepStateScript)
	if err != nil {
		return fmt.Errorf("failed to load validate_and_update_step_state script: %w", err)
	}
	r.scriptHashes["validate_and_update"] = sha
	return nil
}

// ValidateAndUpdateStepState implements the interface
func (r *StepStateRepository) ValidateAndUpdateStepState(
	ctx context.Context,
	params interfaces.ValidateStepParams,
) (*interfaces.StepStateResult, error) {

	// Marshal parameters to JSON
	existingViolationsJSON := "[]"
	if len(params.ExistingViolations) > 0 {
		data, err := json.Marshal(params.ExistingViolations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal existing violations: %w", err)
		}
		existingViolationsJSON = string(data)
	}

	// Pre-compute static data in Go to avoid JSON parsing in Lua
	// Convert booleans to strings
	isTerminalStr := "false"
	if params.IsTerminalStep {
		isTerminalStr = "true"
	}

	// Pre-compute terminal steps using URL encoding to handle special characters
	var terminalStepsStr string
	if params.TerminalSteps == nil || len(params.TerminalSteps) == 0 {
		terminalStepsStr = ""
	} else {
		var encodedSteps []string
		for _, step := range params.TerminalSteps {
			encodedSteps = append(encodedSteps, url.QueryEscape(step))
		}
		terminalStepsStr = strings.Join(encodedSteps, ",")
	}

	// Pre-compute allowed transitions using URL encoding to handle special characters
	var transitionsStr string
	if params.TransitionsMap == nil || len(params.TransitionsMap) == 0 {
		transitionsStr = ""
	} else {
		var transitions []string
		for fromStep, toSteps := range params.TransitionsMap {
			if len(toSteps) > 0 {
				// URL encode to handle special characters in step names
				encodedFromStep := url.QueryEscape(fromStep)
				var encodedToSteps []string
				for _, toStep := range toSteps {
					encodedToSteps = append(encodedToSteps, url.QueryEscape(toStep))
				}
				transitions = append(transitions, fmt.Sprintf("%s:%s", encodedFromStep, strings.Join(encodedToSteps, ",")))
			}
		}
		transitionsStr = strings.Join(transitions, "|")
	}

	allowMultipleTerminalsStr := "false"
	if params.AllowMultipleTerminals {
		allowMultipleTerminalsStr = "true"
	}

	// Construct step key
	stepKey := fmt.Sprintf("%s:%s", params.StepName, params.IdempotencyKey)

	// Get script hash
	scriptHash, exists := r.scriptHashes["validate_and_update"]
	if !exists {
		return nil, fmt.Errorf("script validate_and_update not loaded")
	}

	// Prepare keys
	keys := []string{
		fmt.Sprintf("thread:%s:meta", params.ThreadID),
		fmt.Sprintf("thread:%s:current_steps", params.ThreadID),
		fmt.Sprintf("thread:%s:steps:%s:%s", params.ThreadID, params.StepName, params.IdempotencyKey),
		fmt.Sprintf("thread:%s:violations", params.ThreadID),
	}

	// Prepare args
	args := []interface{}{
		stepKey,                   // ARGV[1]
		params.StepID,             // ARGV[2]
		params.Status,             // ARGV[3]
		params.Timestamp,          // ARGV[4]
		existingViolationsJSON,    // ARGV[5]
		isTerminalStr,             // ARGV[6]
		params.MaxRetries,         // ARGV[7]
		transitionsStr,            // ARGV[8] - Pre-computed transitions string
		terminalStepsStr,          // ARGV[9] - Pre-computed terminal steps string
		allowMultipleTerminalsStr, // ARGV[10]
		r.ttl,                     // ARGV[11] - TTL in seconds from config
		params.ThreadID,           // ARGV[12] - threadID (passed to avoid regex extraction)
		params.IdempotencyKey,     // ARGV[13] - idempotencyKey (passed to avoid regex extraction)
		params.Actor,              // ARGV[14] - actor (user who recorded this step, for .own permission filtering)
	}

	// Execute Lua script
	result, err := r.client.EvalSHA(ctx, scriptHash, keys, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Lua script: %w", err)
	}

	// Safely assert result type
	resultStr, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from Lua script: %T, expected string", result)
	}

	// Parse JSON response
	var luaResult interfaces.StepStateResult
	if err := json.Unmarshal([]byte(resultStr), &luaResult); err != nil {
		fmt.Printf("[LUA-RESULT-ERROR] Failed to parse: %s\n", resultStr)
		return nil, fmt.Errorf("failed to parse Lua result: %w", err)
	}

	return &luaResult, nil
}

// GetStepStateWithCache retrieves a step state using read-only cache pattern:
// 1. Try Redis hash first (async validator's live data)
// 2. Fallback to PostgreSQL if cache miss (inactive/archived steps)
// 3. NO write-back to prevent data conflicts with async validator
func (r *StepStateRepository) GetStepStateWithCache(ctx context.Context, threadID, stepName, idempotencyKey string) (*models.StepStateInfo, error) {
	// Add timeout context for production robustness
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stepKey := fmt.Sprintf("thread:%s:steps:%s:%s", threadID, stepName, idempotencyKey)

	// Try Redis first (active step data from async validator)
	result, err := r.client.HGetAll(ctx, stepKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step state from Redis: %w", err)
	}

	// Check if step state exists in Redis
	if len(result) > 0 {
		log.Printf("🎯 Cache HIT for step state %s:%s from Redis (active step)", stepName, idempotencyKey)

		// Parse hash fields into StepState
		stepState := &models.StepStateInfo{
			ThreadID:       threadID,
			StepName:       stepName,
			IdempotencyKey: idempotencyKey,
		}

		// Parse Redis hash fields (same logic as before)
		if status, exists := result["status"]; exists {
			stepState.Status = status
		}
		if retryCount, exists := result["retryCount"]; exists {
			if count, err := strconv.Atoi(retryCount); err == nil {
				stepState.RetryCount = count
			}
		}
		if firstSeenAt, exists := result["firstSeenAt"]; exists {
			if timestamp, err := time.Parse(time.RFC3339, firstSeenAt); err == nil {
				stepState.FirstSeenAt = timestamp
			}
		}
		if lastUpdatedAt, exists := result["lastUpdatedAt"]; exists {
			if timestamp, err := time.Parse(time.RFC3339, lastUpdatedAt); err == nil {
				stepState.LastUpdatedAt = timestamp
			}
		}
		if latestStepID, exists := result["latestStepID"]; exists {
			stepState.LatestStepID = latestStepID
		}
		if previousStep, exists := result["previousStep"]; exists {
			stepState.PreviousStep = previousStep
		}

		return stepState, nil
	}

	log.Printf("❌ Cache MISS for step state %s:%s - falling back to PostgreSQL (inactive step)", stepName, idempotencyKey)

	// Fallback to PostgreSQL if available (inactive/archived steps)
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("step state not found in Redis and no PostgreSQL repository configured")
	}

	// Use GetStepsBatch to get all steps for the thread
	stepsMap, err := r.postgresRepo.GetStepsBatch(ctx, []string{threadID})
	if err != nil {
		return nil, fmt.Errorf("failed to get step states from PostgreSQL: %w", err)
	}

	allSteps := stepsMap[threadID]
	if allSteps == nil || len(allSteps) == 0 {
		log.Printf("❌ No step states found in PostgreSQL: thread=%s", threadID)
		return nil, nil
	}

	// Find the requested step
	for _, step := range allSteps {
		if step.StepName == stepName && step.IdempotencyKey == idempotencyKey {
			log.Printf("📥 Retrieved step state %s:%s from PostgreSQL (inactive step)", stepName, idempotencyKey)
			// NO write-back - let async validator own Redis writes to prevent data conflicts
			return step, nil
		}
	}

	log.Printf("❌ Step state %s:%s not found in PostgreSQL for thread %s", stepName, idempotencyKey, threadID)
	return nil, nil
}

// GetStepState retrieves a step state from Redis hash only (legacy method)
// Returns nil if not found (step states are ephemeral operational data)
func (r *StepStateRepository) GetStepState(ctx context.Context, threadID, stepName, idempotencyKey string) (*models.StepStateInfo, error) {
	stepKey := fmt.Sprintf("thread:%s:steps:%s:%s", threadID, stepName, idempotencyKey)

	// Get all hash fields
	result, err := r.client.HGetAll(ctx, stepKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step state from Redis: %w", err)
	}

	// Check if step state exists
	if len(result) == 0 {
		log.Printf("❌ Step state not found in Redis: thread=%s, step=%s:%s", threadID, stepName, idempotencyKey)
		return nil, nil // Not found is expected for ephemeral data
	}

	log.Printf("🎯 Step state found in Redis: thread=%s, step=%s:%s", threadID, stepName, idempotencyKey)

	// Parse hash fields into StepStateInfo
	stepState := &models.StepStateInfo{
		ThreadID:       threadID,
		StepName:       stepName,
		IdempotencyKey: idempotencyKey,
	}

	// Parse status
	if status, exists := result["status"]; exists {
		stepState.Status = status
	}

	// Parse retryCount
	if retryCount, exists := result["retryCount"]; exists {
		if count, err := strconv.Atoi(retryCount); err == nil {
			stepState.RetryCount = count
		}
	}

	// Parse timestamps
	if firstSeenAt, exists := result["firstSeenAt"]; exists {
		if timestamp, err := time.Parse(time.RFC3339, firstSeenAt); err == nil {
			stepState.FirstSeenAt = timestamp
		}
	}

	if lastUpdatedAt, exists := result["lastUpdatedAt"]; exists {
		if timestamp, err := time.Parse(time.RFC3339, lastUpdatedAt); err == nil {
			stepState.LastUpdatedAt = timestamp
		}
	}

	// Parse step ID
	if latestStepID, exists := result["latestStepID"]; exists {
		stepState.LatestStepID = latestStepID
	}

	// Parse previous step
	if previousStep, exists := result["previousStep"]; exists {
		stepState.PreviousStep = previousStep
	}

	return stepState, nil
}

// ListSteps retrieves all step states for a thread with hot/cold fallback
// Uses pipeline for efficient Valkey reads and batch query for PostgreSQL fallback
// Optional filters: stepName, idempotencyKey, status (nil = no filter)
func (r *StepStateRepository) ListSteps(ctx context.Context, threadID string, stepName, idempotencyKey, status *string) ([]*models.StepStateInfo, error) {
	pattern := fmt.Sprintf("thread:%s:steps:*", threadID)

	// Scan for all step keys
	keys, err := r.client.Keys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan step keys: %w", err)
	}

	// Hot path: Read from Valkey (direct calls since pipeline doesn't support HGetAll)
	if len(keys) > 0 {
		var steps []*models.StepStateInfo
		hasData := false

		for _, key := range keys {
			// Extract stepName and idempotencyKey from key
			// Key format: thread:{threadID}:steps:{stepName}:{idempotencyKey}
			parts := strings.Split(key, ":")
			if len(parts) < 5 {
				continue
			}

			stepNameFromKey := parts[3]
			idempKeyFromKey := parts[4]

			// Apply key-level filtering (before fetching data)
			if stepName != nil && *stepName != stepNameFromKey {
				continue // Skip if stepName filter doesn't match
			}
			if idempotencyKey != nil && *idempotencyKey != idempKeyFromKey {
				continue // Skip if idempotencyKey filter doesn't match
			}

			// Get hash data from Valkey
			hashData, err := r.client.HGetAll(ctx, key)
			if err != nil || len(hashData) == 0 {
				continue
			}

			hasData = true

			// Parse step state from hash
			step := &models.StepStateInfo{
				ThreadID:       threadID,
				StepName:       stepNameFromKey,
				IdempotencyKey: idempKeyFromKey,
				Status:         hashData["status"],
				LatestStepID:   hashData["latestStepID"],
				PreviousStep:   hashData["previousStep"],
			}

			if retryCount, err := strconv.Atoi(hashData["retryCount"]); err == nil {
				step.RetryCount = retryCount
			}

			if firstSeenAt, err := time.Parse(time.RFC3339, hashData["firstSeenAt"]); err == nil {
				step.FirstSeenAt = firstSeenAt
			}

			if lastUpdatedAt, err := time.Parse(time.RFC3339, hashData["lastUpdatedAt"]); err == nil {
				step.LastUpdatedAt = lastUpdatedAt
			}

			// Apply status filter
			if status != nil && *status != step.Status {
				continue // Skip if status filter doesn't match
			}

			steps = append(steps, step)
		}

		if hasData {
			log.Printf("✅ [HOT] Found %d step states in Valkey for thread %s", len(steps), threadID)
			return steps, nil
		}
	}

	// Cold path: Fallback to PostgreSQL with batch query
	if r.postgresRepo == nil {
		log.Printf("⚠️ No steps in Valkey and no PostgreSQL fallback for thread %s", threadID)
		return []*models.StepStateInfo{}, nil
	}

	log.Printf("⚠️ [COLD] Steps not in Valkey for thread %s, querying PostgreSQL", threadID)

	// Use batch query (single SQL query for all steps)
	stepsMap, err := r.postgresRepo.GetStepsBatch(ctx, []string{threadID})
	if err != nil {
		return nil, fmt.Errorf("failed to get steps from PostgreSQL: %w", err)
	}

	steps := stepsMap[threadID]
	if steps == nil {
		steps = []*models.StepStateInfo{}
	}

	// Apply filters to PostgreSQL results
	if stepName != nil || idempotencyKey != nil || status != nil {
		filteredSteps := make([]*models.StepStateInfo, 0)
		for _, step := range steps {
			if stepName != nil && step.StepName != *stepName {
				continue
			}
			if idempotencyKey != nil && step.IdempotencyKey != *idempotencyKey {
				continue
			}
			if status != nil && step.Status != *status {
				continue
			}
			filteredSteps = append(filteredSteps, step)
		}
		steps = filteredSteps
	}

	log.Printf("✅ [COLD] Retrieved %d step states from PostgreSQL for thread %s", len(steps), threadID)
	return steps, nil
}

// GetStepsWithPermissionCheck retrieves steps with permission filtering
// Hot path: Valkey first with in-memory permission filtering
// Cold path: PostgreSQL with SQL-level permission filtering
// Permission logic:
// - thread.read.* = can see all steps
// - thread.read.own = can only see steps where actor = userID
func (r *StepStateRepository) GetStepsWithPermissionCheck(
	ctx context.Context,
	threadID string,
	userID string,
	permCheck *PermissionCheckResult,
	stepName *string,
	idempotencyKey *string,
	status *string,
) ([]*models.StepStateInfo, error) {
	// If no access, return empty
	if permCheck == nil || !permCheck.HasAccess {
		return []*models.StepStateInfo{}, nil
	}

	pattern := fmt.Sprintf("thread:%s:steps:*", threadID)

	// Scan for all step keys
	keys, err := r.client.Keys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan step keys: %w", err)
	}

	// Hot path: Read from Valkey
	if len(keys) > 0 {
		var steps []*models.StepStateInfo
		hasData := false

		for _, key := range keys {
			// Extract stepName and idempotencyKey from key
			// Key format: thread:{threadID}:steps:{stepName}:{idempotencyKey}
			parts := strings.Split(key, ":")
			if len(parts) < 5 {
				continue
			}

			stepNameFromKey := parts[3]
			idempKeyFromKey := parts[4]

			// Apply key-level filtering (before fetching data)
			if stepName != nil && *stepName != stepNameFromKey {
				continue
			}
			if idempotencyKey != nil && *idempotencyKey != idempKeyFromKey {
				continue
			}

			// Get hash data from Valkey
			hashData, err := r.client.HGetAll(ctx, key)
			if err != nil || len(hashData) == 0 {
				continue
			}

			hasData = true

			// Parse step state from hash
			step := &models.StepStateInfo{
				ThreadID:       threadID,
				StepName:       stepNameFromKey,
				IdempotencyKey: idempKeyFromKey,
				Status:         hashData["status"],
				LatestStepID:   hashData["latestStepID"],
				PreviousStep:   hashData["previousStep"],
			}

			// Parse actor field for permission filtering
			if actor, exists := hashData["actor"]; exists {
				step.Actor = actor
			}

			if retryCount, err := strconv.Atoi(hashData["retryCount"]); err == nil {
				step.RetryCount = retryCount
			}

			if firstSeenAt, err := time.Parse(time.RFC3339, hashData["firstSeenAt"]); err == nil {
				step.FirstSeenAt = firstSeenAt
			}

			if lastUpdatedAt, err := time.Parse(time.RFC3339, hashData["lastUpdatedAt"]); err == nil {
				step.LastUpdatedAt = lastUpdatedAt
			}

			// Apply status filter
			if status != nil && *status != step.Status {
				continue
			}

			// Apply permission-based filtering
			// If user only has .own permission, filter by actor
			if !permCheck.HasFullRead && permCheck.HasOwnRead {
				if step.Actor != userID {
					continue // Skip steps not owned by this user
				}
			}

			steps = append(steps, step)
		}

		if hasData {
			log.Printf("✅ [HOT] Found %d permission-filtered step states in Valkey for thread %s, user %s", len(steps), threadID, userID)
			return steps, nil
		}
	}

	// Cold path: Fallback to PostgreSQL with SQL-level permission filtering
	if r.postgresRepo == nil {
		log.Printf("⚠️ No steps in Valkey and no PostgreSQL fallback for thread %s", threadID)
		return []*models.StepStateInfo{}, nil
	}

	log.Printf("⚠️ [COLD] Steps not in Valkey for thread %s, querying PostgreSQL with permission check", threadID)

	// Use SQL-level permission filtering
	return r.postgresRepo.GetStepsWithPermissionCheck(ctx, threadID, userID, stepName, idempotencyKey, status)
}

// GetStepHistoryWithPermissionCheck retrieves step history with permission filtering
// Always goes to PostgreSQL (archival data) with SQL-level permission filtering
func (r *StepStateRepository) GetStepHistoryWithPermissionCheck(
	ctx context.Context,
	threadID string,
	userID string,
	stepIdentifier string,
	limit int,
	offset int,
	startAt *string,
	endAt *string,
	activityType *string,
	actorFilter *string,
) ([]models.StepHistory, error) {
	// Step history always queries PostgreSQL directly (archival data)
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("step history requires PostgreSQL repository")
	}

	// Use SQL-level permission filtering
	return r.postgresRepo.GetStepHistoryWithPermissionCheck(ctx, threadID, userID, stepIdentifier, limit, offset, startAt, endAt, activityType, actorFilter)
}
