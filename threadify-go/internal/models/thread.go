package models

import (
	"encoding/json"
	"time"
)

// ThreadStatus represents the current state of a thread
type ThreadStatus string

const (
	ThreadStatusActive    ThreadStatus = "active"
	ThreadStatusCompleted ThreadStatus = "completed"
	ThreadStatusFailed    ThreadStatus = "failed"
	ThreadStatusCancelled ThreadStatus = "cancelled"
)

// StepStatus represents the current state of a step
type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusFailed     StepStatus = "failed"
	StepStatusSkipped    StepStatus = "skipped"
)

// Thread represents a contract execution instance
type Thread struct {
	ID              string                 `json:"id"`
	ContractID      *string                `json:"contractId,omitempty"`
	ContractVersion *int                   `json:"contractVersion,omitempty"`
	OwnerID         string                 `json:"ownerId"`
	Status          ThreadStatus           `json:"status"`
	CurrentStep     string                 `json:"currentStep"`
	Context         map[string]interface{} `json:"context"`
	Steps           map[string]*StepState  `json:"steps"`
	LastHash        string                 `json:"lastHash"`
	StartedAt       time.Time              `json:"startedAt"`
	CompletedAt     *time.Time             `json:"completedAt,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// StepState represents the state of a step in a thread
type StepState struct {
	ID          string    `json:"id"`          // stepId (same as stepName from StepEvent)
	Status      string    `json:"status"`      // "completed" | "failed" | "pending"
	CreatedAt   time.Time `json:"createdAt"`   // When step state was first created
	UpdatedAt   time.Time `json:"updatedAt"`   // When step state was last updated
	RetryCount  int       `json:"retryCount"`  // Number of retry attempts
	IsCompleted bool      `json:"isCompleted"` // Convenience flag to prevent updates
}

// NewThread creates a new thread instance
// contractID and contractVersion can be empty/0 for threads without contracts
func NewThread(id, contractID string, contractVersion int, ownerID string) *Thread {
	thread := &Thread{
		ID:        id,
		OwnerID:   ownerID,
		Status:    ThreadStatusActive,
		Context:   make(map[string]interface{}),
		Steps:     make(map[string]*StepState),
		StartedAt: time.Now(),
	}

	// Only set contract fields if provided
	if contractID != "" {
		thread.ContractID = &contractID
	}
	if contractVersion > 0 {
		thread.ContractVersion = &contractVersion
	}

	return thread
}

// StartStep marks a step as in progress
func (t *Thread) StartStep(stepID string) {
	now := time.Now()
	if t.Steps[stepID] == nil {
		t.Steps[stepID] = &StepState{
			ID:          stepID,
			Status:      string(StepStatusInProgress),
			CreatedAt:   now,
			UpdatedAt:   now,
			RetryCount:  0,
			IsCompleted: false,
		}
	} else {
		t.Steps[stepID].Status = string(StepStatusInProgress)
		t.Steps[stepID].UpdatedAt = now
	}
	t.CurrentStep = stepID
}

// CompleteStep marks a step as completed
func (t *Thread) CompleteStep(stepID string, context map[string]interface{}) {
	now := time.Now()
	if t.Steps[stepID] != nil {
		t.Steps[stepID].Status = string(StepStatusCompleted)
		t.Steps[stepID].UpdatedAt = now
		t.Steps[stepID].IsCompleted = true
		// Context is no longer stored in StepState - it's in StepEvent
	}
}

// FailStep marks a step as failed
func (t *Thread) FailStep(stepID string, errorMsg string) {
	now := time.Now()
	if t.Steps[stepID] != nil {
		t.Steps[stepID].Status = string(StepStatusFailed)
		t.Steps[stepID].UpdatedAt = now
		t.Steps[stepID].IsCompleted = false
		// Error is no longer stored in StepState - it's in StepEvent
	}
}

// Complete marks the thread as completed
func (t *Thread) Complete() {
	now := time.Now()
	t.Status = ThreadStatusCompleted
	t.CompletedAt = &now
}

// Fail marks the thread as failed
func (t *Thread) Fail(errorMsg string) {
	now := time.Now()
	t.Status = ThreadStatusFailed
	t.CompletedAt = &now
	t.Error = errorMsg
}

// Cancel marks the thread as cancelled
func (t *Thread) Cancel() {
	now := time.Now()
	t.Status = ThreadStatusCancelled
	t.CompletedAt = &now
}

// UpdateContext updates the thread's global context
func (t *Thread) UpdateContext(key string, value interface{}) {
	if t.Context == nil {
		t.Context = make(map[string]interface{})
	}
	t.Context[key] = value
}

// ToJSON serializes the thread to JSON
func (t *Thread) ToJSON() ([]byte, error) {
	return json.Marshal(t)
}

// FromJSON deserializes a thread from JSON
func FromJSON(data []byte) (*Thread, error) {
	var thread Thread
	err := json.Unmarshal(data, &thread)
	if err != nil {
		return nil, err
	}
	return &thread, nil
}

// IsActive returns true if the thread is active
func (t *Thread) IsActive() bool {
	return t.Status == ThreadStatusActive
}

// IsCompleted returns true if the thread is completed (success or failure)
func (t *Thread) IsCompleted() bool {
	return t.Status == ThreadStatusCompleted ||
		t.Status == ThreadStatusFailed ||
		t.Status == ThreadStatusCancelled
}
