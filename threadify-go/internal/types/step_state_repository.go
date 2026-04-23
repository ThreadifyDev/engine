package types

import (
	"context"
)

//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/step_state_repository_mock.go -source=step_state_repository.go

// StepStateRepository handles atomic step state operations with validation
type StepStateRepository interface {
	// LoadScripts loads all Lua scripts into Redis/Valkey
	// Should be called once during application startup
	LoadScripts(ctx context.Context) error

	// ValidateAndUpdateStepState atomically validates and updates step state
	// Returns validation results including any violations found
	ValidateAndUpdateStepState(
		ctx context.Context,
		params ValidateStepParams,
	) (*StepStateResult, error)
}
