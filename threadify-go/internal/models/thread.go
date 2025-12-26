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
	ID              string                `json:"id"`
	ContractID      *string               `json:"contractId,omitempty"`
	ContractVersion *int                  `json:"contractVersion,omitempty"`
	ContractName    string                `json:"contractName,omitempty"` // New field for contract name
	Refs            map[string]string     `json:"refs,omitempty"`         // New field for external references
	OwnerID         string                `json:"ownerId"`
	CompanyID       string                `json:"companyId"` // Company ID for multi-tenancy
	Status          ThreadStatus          `json:"status"`
	CurrentStep     string                `json:"currentStep"`
	LastHash        string                `json:"lastHash"`
	Steps           map[string]*StepState `json:"steps,omitempty"` // Key: stepName:idempKey
	StartedAt       time.Time             `json:"startedAt"`
	CompletedAt     *time.Time            `json:"completedAt,omitempty"`
	Error           string                `json:"error,omitempty"`
}

// StepState represents the state of a step in a thread
type StepState struct {
	StepID         string            `json:"stepId"`         // Unique step event ID (UUID)
	StepName       string            `json:"stepName"`       // Base step name (without :idempKey suffix)
	Status         string            `json:"status"`         // "success" | "failed" | "in_progress"
	IdempotencyKey string            `json:"idempotencyKey"` // For deduplication
	Context        map[string]string `json:"context"`        // Step context
	CreatedAt      time.Time         `json:"createdAt"`      // When step was first created
	UpdatedAt      time.Time         `json:"updatedAt"`      // When step was last updated
	RetryCount     int               `json:"retryCount"`     // Number of retry attempts
}

// NewThread creates a new thread instance
// contractID and contractVersion can be empty/0 for threads without contracts
func NewThread(id, contractID string, contractVersion int, ownerID string) *Thread {
	return NewThreadWithCompany(id, contractID, contractVersion, ownerID, "")
}

// NewThreadWithCompany creates a new thread instance with company information
func NewThreadWithCompany(id, contractID string, contractVersion int, ownerID, companyID string) *Thread {
	thread := &Thread{
		ID:        id,
		OwnerID:   ownerID,
		CompanyID: companyID,
		Status:    ThreadStatusActive,
		Steps:     make(map[string]*StepState), // Initialize steps map
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

// ThreadEvent represents an event in a thread's activity queue
type ThreadEvent struct {
	ThreadID  string            `json:"threadId"`
	StepName  string            `json:"stepName"`
	Action    string            `json:"action"` // "thread_created", "step_completed", etc.
	UserID    string            `json:"userId"`
	Role      string            `json:"role"`
	Refs      map[string]string `json:"refs,omitempty"`
	Context   map[string]string `json:"context,omitempty"`
	Status    string            `json:"status"` // "success", "failed", "in_progress"
	Timestamp time.Time         `json:"timestamp"`
}
