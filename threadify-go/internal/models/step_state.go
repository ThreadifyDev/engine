package models

import (
	"time"
)

// StepStateInfo represents the current state of a step in a thread
// This matches the Redis hash structure from the Lua script
type StepStateInfo struct {
	ThreadID       string    `json:"threadId"`
	StepName       string    `json:"stepName"`
	IdempotencyKey string    `json:"idempotencyKey"`
	Status         string    `json:"status"` // "completed" | "violated" | "failed" | "pending"
	RetryCount     int       `json:"retryCount"`
	FirstSeenAt    time.Time `json:"firstSeenAt"`
	LastUpdatedAt  time.Time `json:"lastUpdatedAt"`
	LatestStepID   string    `json:"latestStepID"`
	PreviousStep   string    `json:"previousStep"`  // "stepName:idempKey" or empty
	Actor          string    `json:"actor"`         // User who recorded this step (for .own permission filtering)
	ActorService   string    `json:"actorService"`  // Service that recorded this step
	LatestContext  string    `json:"latestContext"` // Latest context from most recent history (JSON string)
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
	Attempt      int    `json:"attempt"`
	Timestamp    string `json:"timestamp"`
	Status       string `json:"status"`
	Context      string `json:"context"`
	Duration     int    `json:"duration"`
	Error        string `json:"error,omitempty"`
	Actor        string `json:"actor"`        // User/owner who triggered this step
	ActorService string `json:"actorService"` // Service that executed this step
	CompanyId    string `json:"companyId"`    // Company that owns the thread
	CompanyName  string `json:"companyName"`  // Company name for display
	Hash         string `json:"hash,omitempty"`
	PrevHash     string `json:"prevHash,omitempty"`
}
