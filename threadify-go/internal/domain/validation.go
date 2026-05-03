package domain

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
	NotificationSourceStep   NotificationSource = "step"   // SDK-reported step status (was: execution)
	NotificationSourceRule   NotificationSource = "rule"   // Contract validation result (was: validation)
	NotificationSourceThread NotificationSource = "thread" // Thread-level events
)

// NotificationType represents the full notification type
type NotificationType string

const (
	// Step notifications (execution events)
	NotificationTypeStepStarted    NotificationType = "step.started"
	NotificationTypeStepInProgress NotificationType = "step.in_progress"
	NotificationTypeStepSuccess    NotificationType = "step.success"
	NotificationTypeStepCompleted  NotificationType = "step.completed"
	NotificationTypeStepFailed     NotificationType = "step.failed"
	NotificationTypeStepError      NotificationType = "step.error"
	NotificationTypeStepCancelled  NotificationType = "step.cancelled"
	NotificationTypeStepSkipped    NotificationType = "step.skipped"

	// Rule notifications (validation events)
	NotificationTypeRuleViolated NotificationType = "rule.violated"
	NotificationTypeRulePassed   NotificationType = "rule.passed"

	// Specific rule violation types
	NotificationTypeRuleViolatedTimeout    NotificationType = "rule.violated.timeout"
	NotificationTypeRuleViolatedRetryLimit NotificationType = "rule.violated.retry_limit"
	NotificationTypeRuleViolatedCritical   NotificationType = "rule.violated.critical"

	// Thread notifications
	NotificationTypeThreadCompleted NotificationType = "thread.completed"
	NotificationTypeThreadCancelled NotificationType = "thread.cancelled"
	NotificationTypeThreadClosed    NotificationType = "thread.closed"
)

// ThreadViolation tracks all failed steps in a thread
type ThreadViolation struct {
	FailedSteps map[string]StepViolation
	ViolatedAt  time.Time
}

// StepViolation represents a single step violation
type StepViolation struct {
	StepID        string
	StepName      string
	OwnerID       string // Who published the step
	ViolationType ViolationType
	Severity      ViolationSeverity
	Message       string
	Details       map[string]interface{}
	ViolatedAt    time.Time
}

// ValidationNotification represents a notification stored in the stream
type ValidationNotification struct {
	// Identity (always present)
	NotificationID string
	ThreadID       string
	StepID         string
	StepName       string
	OwnerID        string // Who published the step
	ContractName   string // Contract name (empty for non-contract threads)

	// Notification metadata
	Source           NotificationSource // step, rule, or thread
	NotificationType NotificationType   // e.g., "step.failed", "rule.violated"

	// Status (always present)
	StepStatus string // User's set status: "success", "failed", "error"
	Status     string // Validation status: "passed", "violated", "none"

	// Violation info (always present, empty string if no violation)
	ViolationType string // Empty if no violation
	Severity      string // Empty if no violation
	Message       string // Always has a message

	// Details (always present, can be empty map)
	// Contains violation-specific data: fromStep, toStep, allowedSteps, retryCount, etc.
	Details map[string]interface{}

	// Timestamp (always present)
	Timestamp time.Time
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
