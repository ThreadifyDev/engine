package models

import "time"

// ViolationType represents the type of contract violation
type ViolationType string

const (
	ViolationInvalidTransition      ViolationType = "invalid_transition"
	ViolationStepTimeoutExceeded    ViolationType = "step_timeout_exceeded"
	ViolationMaxDurationExceeded    ViolationType = "max_duration_exceeded"
	ViolationMultipleTerminalStates ViolationType = "multiple_terminal_states"
	ViolationRetryLimitExceeded     ViolationType = "retry_limit_exceeded"
	ViolationMissingOptionalField   ViolationType = "missing_optional_field"
	ViolationExtraUndocumentedField ViolationType = "extra_undocumented_field"
)

// ViolationSeverity represents the severity level of a violation
type ViolationSeverity string

const (
	SeverityCritical ViolationSeverity = "critical"
	SeverityMajor    ViolationSeverity = "major"
	SeverityMinor    ViolationSeverity = "minor"
	SeverityWarning  ViolationSeverity = "warning"
	SeverityInfo     ViolationSeverity = "info"
)

// NotificationStatus represents the status of a notification
type NotificationStatus string

const (
	NotificationStatusCompleted NotificationStatus = "completed"
	NotificationStatusFailed    NotificationStatus = "failed"
)

// NotificationSource represents the origin of a notification
type NotificationSource string

const (
	NotificationSourceExecution  NotificationSource = "execution"  // SDK-reported step status
	NotificationSourceValidation NotificationSource = "validation" // Contract validation result
	NotificationSourceThread     NotificationSource = "thread"     // Thread-level events
)

// ThreadViolation tracks all failed steps in a thread
type ThreadViolation struct {
	FailedSteps map[string]StepViolation `json:"failedSteps"` // key: stepID
	ViolatedAt  time.Time                `json:"violatedAt"`
}

// StepViolation represents a single step violation
type StepViolation struct {
	StepID        string                 `json:"stepId"`
	StepName      string                 `json:"stepName"`
	OwnerID       string                 `json:"ownerId"` // Who published the step
	ViolationType ViolationType          `json:"violationType"`
	Severity      ViolationSeverity      `json:"severity"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
	ViolatedAt    time.Time              `json:"violatedAt"`
}

// ValidationNotification represents a notification stored in the stream
type ValidationNotification struct {
	// Identity (always present)
	NotificationID string `json:"notificationId"`
	ThreadID       string `json:"threadId"`
	StepID         string `json:"stepId"`
	StepName       string `json:"stepName"`
	OwnerID        string `json:"ownerId"`      // Who published the step
	ContractName   string `json:"contractName"` // Contract name (empty for non-contract threads)

	// Notification metadata
	Source           NotificationSource `json:"source"`           // execution, validation, or thread
	NotificationType string             `json:"notificationType"` // e.g., "execution.failed", "validation.violated"

	// Status (always present)
	StepStatus string `json:"stepStatus"` // User's set status: "success", "failed", "error"
	Status     string `json:"status"`     // Validation status: "passed", "violated", "none"

	// Violation info (always present, empty string if no violation)
	ViolationType string `json:"violationType"` // Empty if no violation
	Severity      string `json:"severity"`      // Empty if no violation
	Message       string `json:"message"`       // Always has a message

	// Details (always present, can be empty map)
	// Contains violation-specific data: fromStep, toStep, allowedSteps, retryCount, etc.
	Details map[string]interface{} `json:"details"`

	// Timestamp (always present)
	Timestamp time.Time `json:"timestamp"`
}

// ValidationViolation is an internal struct used during validation checks
type ValidationViolation struct {
	Message       string
	FromStep      string
	ToStep        string
	Duration      string
	Limit         string
	MissingFields []string
	ExtraFields   []string
	Severity      ViolationSeverity
	Details       map[string]interface{}
}
