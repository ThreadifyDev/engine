package models

import (
	"time"
)

// ValidationResultInfo represents a validation result from thread_validations table
// Renamed from ValidationResult to avoid conflicts with existing types
type ValidationResultInfo struct {
	ValidationID   string            `json:"validationId"`
	ThreadID       string            `json:"threadId"`
	StepID         string            `json:"stepId"`
	StepName       string            `json:"stepName"`
	IdempotencyKey string            `json:"idempotencyKey"`
	Timestamp      time.Time         `json:"timestamp"`
	Validations    []ValidationIssue `json:"validations"`
}

// ValidationIssue represents a single validation violation or warning
// Renamed from ValidationNotification to avoid conflicts with stream notifications
type ValidationIssue struct {
	Type     string `json:"type"` // "critical", "warning", "info"
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Rule     string `json:"rule,omitempty"`
}

// ValidationQueryOptions for filtering validation results
type ValidationQueryOptions struct {
	ThreadID       string `json:"threadId,omitempty"`
	StepID         string `json:"stepId,omitempty"`
	StepName       string `json:"stepName,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	ValidationType string `json:"validationType,omitempty"` // "critical", "warning", "info"
	Limit          int    `json:"limit,omitempty"`
	Offset         int    `json:"offset,omitempty"`
}
