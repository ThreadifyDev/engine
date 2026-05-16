package domain

import (
	"time"
)

// ThreadStatus represents the current state of a thread
type ThreadStatus string

const (
	ThreadStatusActive    ThreadStatus = "active"
	ThreadStatusCompleted ThreadStatus = "completed"
	ThreadStatusFailed    ThreadStatus = "failed"
	ThreadStatusCancelled ThreadStatus = "cancelled"
	ThreadStatusClosed    ThreadStatus = "closed"
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
	ID              string
	ContractID      *string
	ContractVersion *int
	ContractName    string            // New field for contract name
	Refs            map[string]string // New field for external references
	OwnerID         string
	CompanyID       string // Company ID for multi-tenancy
	Label           string // New field for user-friendly thread label
	CreatedBy       string // User or service account that created the thread
	Status          ThreadStatus
	Tags            []string // Tags associated with the thread
	LastHash        string
	Violated        *ThreadViolation // Tracks failed steps and violations
	StartedAt       time.Time
	CompletedAt     *time.Time
	Error           string
	// Step data is stored separately in Valkey:
	// - Current steps: thread:ID:current_steps (sorted set)
	// - Step state: thread:ID:steps:{stepName}:{idempKey} (hashes)
}

// ThreadConnection wraps thread results with pagination metadata
type ThreadConnection struct {
	Threads    []*Thread
	TotalCount int
}

// StepState represents the state of a step in a thread
type StepState struct {
	StepID         string
	StepName       string
	Status         string
	IdempotencyKey string
	Context        map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RetryCount     int
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
		StartedAt: time.Now(),
	}

	// Only set contract fields if provided
	if contractID != "" {
		thread.ContractID = &contractID
	}
	if contractVersion >= 0 {
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
	ThreadID  string
	StepName  string
	Action    string // "thread_created", "step_completed", etc.
	UserID    string
	Role      string
	Refs      map[string]string
	Context   map[string]string
	Status    string // "success", "failed", "in_progress"
	Timestamp time.Time
}
