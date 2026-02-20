package models

import (
	"time"
)

// ThreadNotification represents an individual notification from the async notification system
// Stored in thread_notifications table with full context (execution, validation, thread events)
type ThreadNotification struct {
	NotificationID   string                 `json:"notificationId"`
	ThreadID         string                 `json:"threadId"`
	StepID           string                 `json:"stepId"`
	StepName         string                 `json:"stepName"`
	IdempotencyKey   string                 `json:"idempotencyKey,omitempty"`
	Source           string                 `json:"source"`                     // 'execution', 'validation', 'thread'
	NotificationType string                 `json:"notificationType"`           // 'execution.success', 'validation.violated', etc.
	StepStatus       string                 `json:"stepStatus,omitempty"`       // 'success', 'failed', 'error'
	ValidationStatus string                 `json:"validationStatus,omitempty"` // 'passed', 'violated', 'none'
	ViolationType    string                 `json:"violationType,omitempty"`
	Severity         string                 `json:"severity,omitempty"` // 'critical', 'warning', 'info', 'major', 'minor'
	Message          string                 `json:"message"`
	Details          map[string]interface{} `json:"details,omitempty"`
	Timestamp        time.Time              `json:"timestamp"`
}

// ThreadNotificationQueryOptions for filtering notifications
type ThreadNotificationQueryOptions struct {
	ThreadID         string   `json:"threadId,omitempty"`
	StepID           string   `json:"stepId,omitempty"`
	StepName         string   `json:"stepName,omitempty"`
	Source           string   `json:"source,omitempty"`           // 'execution', 'validation', 'thread'
	NotificationType string   `json:"notificationType,omitempty"` // 'execution.success', etc.
	Severity         []string `json:"severity,omitempty"`         // ['critical', 'warning']
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
}

// NotificationSummary provides aggregated counts for efficient loading
type NotificationSummary struct {
	TotalNotifications int  `json:"totalNotifications"`
	CriticalCount      int  `json:"criticalCount"`
	WarningCount       int  `json:"warningCount"`
	MajorCount         int  `json:"majorCount"`
	MinorCount         int  `json:"minorCount"`
	InfoCount          int  `json:"infoCount"`
	ExecutionCount     int  `json:"executionCount"`
	ValidationCount    int  `json:"validationCount"`
	HasCritical        bool `json:"hasCritical"`
	HasWarnings        bool `json:"hasWarnings"`
}
