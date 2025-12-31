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
	NotificationID string                 `json:"notificationId"`
	ThreadID       string                 `json:"threadId"`
	StepID         string                 `json:"stepId,omitempty"`
	StepName       string                 `json:"stepName,omitempty"`
	OwnerID        string                 `json:"ownerId"` // Who published the step
	Status         NotificationStatus     `json:"status"`  // "completed", "failed"
	ViolationType  ViolationType          `json:"violationType,omitempty"`
	Severity       ViolationSeverity      `json:"severity,omitempty"`
	Message        string                 `json:"message"`
	Details        map[string]interface{} `json:"details,omitempty"`
	Timestamp      time.Time              `json:"timestamp"`

	// For transition violations
	FromStep string `json:"fromStep,omitempty"`
	ToStep   string `json:"toStep,omitempty"`

	// For timeout violations
	Duration string `json:"duration,omitempty"`
	Limit    string `json:"limit,omitempty"`

	// For field violations
	MissingFields []string `json:"missingFields,omitempty"`
	ExtraFields   []string `json:"extraFields,omitempty"`
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
