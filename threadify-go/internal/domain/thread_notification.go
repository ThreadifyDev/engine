package domain

import (
	"time"
)

// ThreadNotification represents an individual notification from the async notification system
// Stored in thread_notifications table with full context (execution, validation, thread events)
type ThreadNotification struct {
	NotificationID   string
	ThreadID         string
	StepID           string
	StepName         string
	IdempotencyKey   string
	Source           string // 'execution', 'validation', 'thread'
	NotificationType string // 'step.success', 'rule.violated', etc.
	StepStatus       string // 'success', 'failed', 'error'
	ValidationStatus string // 'passed', 'violated', 'none'
	ViolationType    string
	Severity         string // 'critical', 'warning', 'info', 'major', 'minor'
	Message          string
	Details          map[string]interface{}
	Timestamp        time.Time
}

// ThreadNotificationQueryOptions for filtering notifications
type ThreadNotificationQueryOptions struct {
	ThreadID         string
	StepID           string
	StepName         string
	Source           string   // 'execution', 'validation', 'thread'
	NotificationType string   // 'step.success', etc.
	Severity         []string // ['critical', 'warning']
	Limit            int
	Offset           int
}

// NotificationSummary provides aggregated counts for efficient loading
type NotificationSummary struct {
	TotalNotifications int
	CriticalCount      int
	WarningCount       int
	MajorCount         int
	MinorCount         int
	InfoCount          int
	ExecutionCount     int
	ValidationCount    int
	HasCritical        bool
	HasWarnings        bool
}
