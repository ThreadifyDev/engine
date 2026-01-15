package interfaces

import (
	"context"
)

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

// ValidateStepParams contains all parameters for step validation and update
type ValidateStepParams struct {
	ThreadID               string
	StepID                 string
	StepName               string
	IdempotencyKey         string
	Status                 string
	ExistingViolations     []Violation
	IsTerminalStep         bool
	Timestamp              string
	MaxRetries             int
	TransitionsMap         map[string][]string // Map of stepName -> allowed next steps
	TerminalSteps          []string
	AllowMultipleTerminals bool
}

// Violation represents a validation violation
type Violation struct {
	Type     string                 `json:"violationType"`
	Severity string                 `json:"severity"`
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// StepStateResult contains the results of step validation and update
type StepStateResult struct {
	Status               string      `json:"status"`
	Violations           []Violation `json:"violations"`
	RetryCount           int         `json:"retryCount"`
	HasCriticalViolation bool        `json:"hasCriticalViolation"`
	FirstSeenAt          string      `json:"firstSeenAt"`  // Timestamp when step was first seen
	PreviousStep         string      `json:"previousStep"` // Previous step key (stepName:idempKey)
}
