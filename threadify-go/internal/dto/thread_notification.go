package dto

import "time"

type ThreadNotification struct {
	NotificationID   string                 `json:"notificationId"`
	ThreadID         string                 `json:"threadId"`
	StepID           string                 `json:"stepId"`
	StepName         string                 `json:"stepName"`
	IdempotencyKey   string                 `json:"idempotencyKey,omitempty"`
	Source           string                 `json:"source"`
	NotificationType string                 `json:"notificationType"`
	StepStatus       string                 `json:"stepStatus,omitempty"`
	ValidationStatus string                 `json:"validationStatus,omitempty"`
	ViolationType    string                 `json:"violationType,omitempty"`
	Severity         string                 `json:"severity,omitempty"`
	Message          string                 `json:"message"`
	Details          map[string]interface{} `json:"details,omitempty"`
	Timestamp        time.Time              `json:"timestamp"`
}

type ThreadNotificationQueryOptions struct {
	ThreadID         string   `json:"threadId,omitempty"`
	StepID           string   `json:"stepId,omitempty"`
	StepName         string   `json:"stepName,omitempty"`
	Source           string   `json:"source,omitempty"`
	NotificationType string   `json:"notificationType,omitempty"`
	Severity         []string `json:"severity,omitempty"`
	Limit            int      `json:"limit,omitempty"`
	Offset           int      `json:"offset,omitempty"`
}

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

type ValidationNotification struct {
	NotificationID   string                 `json:"notificationId"`
	ThreadID         string                 `json:"threadId"`
	StepID           string                 `json:"stepId"`
	StepName         string                 `json:"stepName"`
	OwnerID          string                 `json:"ownerId"`
	ContractName     string                 `json:"contractName"`
	Source           string                 `json:"source"`
	NotificationType string                 `json:"notificationType"`
	StepStatus       string                 `json:"stepStatus"`
	Status           string                 `json:"status"`
	ViolationType    string                 `json:"violationType"`
	Severity         string                 `json:"severity"`
	Message          string                 `json:"message"`
	Details          map[string]interface{} `json:"details"`
	Timestamp        time.Time              `json:"timestamp"`
}

type ThreadViolation struct {
	FailedSteps map[string]StepViolation `json:"failedSteps"`
	ViolatedAt  time.Time                `json:"violatedAt"`
}

type StepViolation struct {
	StepID        string                 `json:"stepId"`
	StepName      string                 `json:"stepName"`
	OwnerID       string                 `json:"ownerId"`
	ViolationType string                 `json:"violationType"`
	Severity      string                 `json:"severity"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
	ViolatedAt    time.Time              `json:"violatedAt"`
}
