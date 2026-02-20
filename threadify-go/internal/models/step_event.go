package models

import (
	"encoding/json"
	"time"
)

// StepEventResponse represents the response for step event processing
type StepEventResponse struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
	StepID  string `json:"step_id"`
}

// StepEvent represents a complete step event from the SDK
type StepEvent struct {
	StepID         string                 `json:"step_id"`
	Type           string                 `json:"type"` // step_started, step_completed, step_failed
	Context        map[string]interface{} `json:"context"`
	Status         string                 `json:"status"` // success, failure
	Timestamp      time.Time              `json:"timestamp"`
	ServiceName    string                 `json:"service_name"`
	ThreadID       string                 `json:"thread_id"`
	StepName       string                 `json:"step_name"`
	StartedAt      string                 `json:"started_at"`
	FinishedAt     string                 `json:"finished_at"`
	IdempotencyKey string                 `json:"idempotency_key"` // User-provided or context hash
	ContentHash    string                 `json:"content_hash"`    // Always auto-generated SHA-256 hash of context
}

// HashedStepEvent represents a step event with its calculated hash
type HashedStepEvent struct {
	StepEvent
	Hash         string    `json:"hash"`
	PreviousHash string    `json:"prev_hash"`
	CreatedAt    time.Time `json:"created_at"`
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
