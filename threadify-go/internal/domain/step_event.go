package domain

import (
	"encoding/json"
	"time"
)

// StepEventResponse represents the response for step event processing
type StepEventResponse struct {
	Action  string
	Status  string
	Message string
	StepID  string
}

// StepEvent represents a complete step event from the SDK
type StepEvent struct {
	StepID         string
	Type           string // step_started, step_completed, step_failed
	Context        map[string]interface{}
	Status         string // success, failure
	Timestamp      time.Time
	ServiceName    string
	ThreadID       string
	StepName       string
	StartedAt      string
	FinishedAt     string
	IdempotencyKey string                 // User-provided or context hash
	ContentHash    string                 // Always auto-generated SHA-256 hash of context
	Metadata       map[string]interface{} // SDK metadata from threadify_metadata (message, etc.)
}

// HashedStepEvent represents a step event with its calculated hash
type HashedStepEvent struct {
	StepEvent
	Hash         string
	PreviousHash string
	CreatedAt    time.Time
}

// ContextJSON returns the context as a JSON string for hashing
func (se *StepEvent) ContextJSON() string {
	if se.Context == nil {
		return "{}"
	}

	data, err := json.Marshal(se.Context)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// ToHashedStepEvent converts a StepEvent to HashedStepEvent
func (se *StepEvent) ToHashedStepEvent(hash, previousHash string) *HashedStepEvent {
	return &HashedStepEvent{
		StepEvent:    *se,
		Hash:         hash,
		PreviousHash: previousHash,
		CreatedAt:    time.Now().UTC(), // Use UTC for consistency
	}
}
