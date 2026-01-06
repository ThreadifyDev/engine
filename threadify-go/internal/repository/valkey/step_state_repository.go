package valkey

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/threadify/engine/internal/interfaces"
)

// Embed Lua script at compile time
//
//go:embed lua/validate_and_update_step_state.lua
var validateAndUpdateStepStateScript string

// StepStateRepository implements interfaces.StepStateRepository
type StepStateRepository struct {
	client       interfaces.ValkeyClient
	scriptHashes map[string]string
}

// NewStepStateRepository creates a new step state repository
func NewStepStateRepository(client interfaces.ValkeyClient) interfaces.StepStateRepository {
	repo := &StepStateRepository{
		client:       client,
		scriptHashes: make(map[string]string),
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

	// Marshal allowed transitions (handle nil/empty slices)
	var allowedTransitionsJSON []byte
	if params.AllowedTransitions == nil || len(params.AllowedTransitions) == 0 {
		allowedTransitionsJSON = []byte("[]")
	} else {
		var err error
		allowedTransitionsJSON, err = json.Marshal(params.AllowedTransitions)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal allowed transitions: %w", err)
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
		stepKey,                        // ARGV[1]
		params.StepID,                  // ARGV[2]
		params.Status,                  // ARGV[3]
		params.Timestamp,               // ARGV[4]
		existingViolationsJSON,         // ARGV[5]
		isTerminalStr,                  // ARGV[6]
		params.MaxRetries,              // ARGV[7]
		string(allowedTransitionsJSON), // ARGV[8]
		string(terminalStepsJSON),      // ARGV[9]
		allowMultipleTerminalsStr,      // ARGV[10]
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
		return nil, fmt.Errorf("failed to parse Lua result: %w", err)
	}

	return &luaResult, nil
}
