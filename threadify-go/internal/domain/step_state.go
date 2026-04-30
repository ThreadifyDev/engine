package domain

import (
	"time"
)

// StepStateInfo represents the current state of a step in a thread
// This matches the Redis hash structure from the Lua script
type StepStateInfo struct {
	ThreadID       string
	StepName       string
	IdempotencyKey string
	Status         string // "completed" | "violated" | "failed" | "pending"
	RetryCount     int
	FirstSeenAt    time.Time
	LastUpdatedAt  time.Time
	StartedAt      *string // When step execution started (pointer for optional field)
	FinishedAt     *string // When step execution finished (pointer for optional field)
	LatestStepID   string
	PreviousStep   string // "stepName:idempKey" or empty
	Actor          string // User who recorded this step (for .own permission filtering)
	ActorService   string // Service that recorded this step
	LatestContext  string // Latest context from most recent history (JSON string)
}

// GetStepKey returns the unique step key in format "stepName:idempotencyKey"
func (s *StepStateInfo) GetStepKey() string {
	return s.StepName + ":" + s.IdempotencyKey
}

// IsCompleted returns true if the step status is "completed"
func (s *StepStateInfo) IsCompleted() bool {
	return s.Status == "success"
}

// IsFailed returns true if the step status indicates failure
func (s *StepStateInfo) IsFailed() bool {
	return s.Status == "failed" || s.Status == "violated"
}

// IsPending returns true if the step is still pending
func (s *StepStateInfo) IsPending() bool {
	return s.Status == "pending"
}

// StepHistory represents a single attempt in a step's retry history
type StepHistory struct {
	Attempt      int
	Timestamp    string
	Status       string
	Context      string
	Duration     int
	StartedAt    string // When step execution started
	FinishedAt   string // When step execution finished
	Metadata     string // SDK metadata from threadify_metadata (JSONB as string)
	Actor        string // User/owner who triggered this step
	ActorService string // Service that executed this step
	CompanyId    string // Company that owns the thread
	CompanyName  string // Company name for display
	Hash         string
	PrevHash     string
}
