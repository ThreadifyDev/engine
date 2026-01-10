package valkey

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
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
}

// NewStepStateRepository creates a new step state repository
func NewStepStateRepository(client interfaces.ValkeyClient) interfaces.StepStateRepository {
	repo := &StepStateRepository{
		client:       client,
		scriptHashes: make(map[string]string),
	}
	return repo
}

// NewStepStateRepositoryWithPostgres creates a new step state repository with PostgreSQL fallback
func NewStepStateRepositoryWithPostgres(client interfaces.ValkeyClient, postgresRepo *postgres.StepStateRepository) *StepStateRepository {
	repo := &StepStateRepository{
		client:       client,
		scriptHashes: make(map[string]string),
		postgresRepo: postgresRepo,
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
	fmt.Printf("Loaded Lua script 'validate_and_update_step_state' with SHA: %s\n", sha)
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

	// Marshal transitions map (handle nil/empty map)
	var transitionsMapJSON []byte
	if params.TransitionsMap == nil || len(params.TransitionsMap) == 0 {
		transitionsMapJSON = []byte("{}")
	} else {
		var err error
		transitionsMapJSON, err = json.Marshal(params.TransitionsMap)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal transitions map: %w", err)
		}
	}

	// Marshal terminal steps (handle nil/empty slices)
	var terminalStepsJSON []byte
	if params.TerminalSteps == nil || len(params.TerminalSteps) == 0 {
		terminalStepsJSON = []byte("[]")
	} else {
		var err error
		terminalStepsJSON, err = json.Marshal(params.TerminalSteps)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal terminal steps: %w", err)
		}
	}

	// Convert booleans to strings
	isTerminalStr := "false"
	if params.IsTerminalStep {
		isTerminalStr = "true"
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
		stepKey,                    // ARGV[1]
		params.StepID,              // ARGV[2]
		params.Status,              // ARGV[3]
		params.Timestamp,           // ARGV[4]
		existingViolationsJSON,     // ARGV[5]
		isTerminalStr,              // ARGV[6]
		params.MaxRetries,          // ARGV[7]
		string(transitionsMapJSON), // ARGV[8] - Changed to transitions map
		string(terminalStepsJSON),  // ARGV[9]
		allowMultipleTerminalsStr,  // ARGV[10]
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
		// Debug: Print the actual Lua result
		fmt.Printf("[LUA-RESULT-ERROR] Failed to parse: %s\n", resultStr)
		return nil, fmt.Errorf("failed to parse Lua result: %w", err)
	}

	// DEBUG: Always log the result
	fmt.Printf("[LUA-RESULT-DEBUG] violations=%d, hasCritical=%v, status=%s\n",
		len(luaResult.Violations), luaResult.HasCriticalViolation, luaResult.Status)

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

	stepState, err := r.postgresRepo.GetStepState(ctx, threadID, stepName, idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get step state from PostgreSQL: %w", err)
	}

	if stepState == nil {
		log.Printf("❌ Step state not found in PostgreSQL: thread=%s, step=%s:%s", threadID, stepName, idempotencyKey)
		return nil, nil
	}

	log.Printf("📥 Retrieved step state %s:%s from PostgreSQL (inactive step)", stepName, idempotencyKey)

	// NO write-back - let async validator own Redis writes to prevent data conflicts
	return stepState, nil
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

// ListSteps retrieves all step states for a thread by scanning Redis keys
// Returns list of step states (may be slow for threads with many steps)
func (r *StepStateRepository) ListSteps(ctx context.Context, threadID string) ([]*models.StepStateInfo, error) {
	pattern := fmt.Sprintf("thread:%s:steps:*", threadID)

	// Scan for all step keys
	keys, err := r.client.Keys(ctx, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to scan step keys: %w", err)
	}

	if len(keys) == 0 {
		return []*models.StepStateInfo{}, nil
	}

	var steps []*models.StepStateInfo

	for _, key := range keys {
		// Extract stepName and idempotencyKey from key
		// Key format: thread:{threadID}:steps:{stepName}:{idempotencyKey}
		parts := strings.Split(key, ":")
		if len(parts) < 5 {
			continue // Skip malformed keys
		}

		stepName := parts[3]
		idempotencyKey := parts[4]

		// Get individual step state
		stepState, err := r.GetStepState(ctx, threadID, stepName, idempotencyKey)
		if err != nil {
			log.Printf("Warning: Failed to get step state for key %s: %v", key, err)
			continue
		}

		if stepState != nil {
			steps = append(steps, stepState)
		}
	}

	log.Printf("📋 Found %d step states for thread %s", len(steps), threadID)
	return steps, nil
}

// GetStepHistory delegates to PostgreSQL repository for step history queries
func (r *StepStateRepository) GetStepHistory(ctx context.Context, threadID, stepIdentifier string, limit, offset int, startAt, endAt, activityType, actor *string) ([]models.StepHistory, error) {
	// Step history queries PostgreSQL directly (archival data)
	if r.postgresRepo == nil {
		return nil, fmt.Errorf("step history requires PostgreSQL repository")
	}

	return r.postgresRepo.GetStepHistory(ctx, threadID, stepIdentifier, limit, offset, startAt, endAt, activityType, actor)
}
