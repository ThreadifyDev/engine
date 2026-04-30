package domain

import (
	"time"
)

// ValidationResultInfo represents a validation result from thread_validations table
// Renamed from ValidationResult to avoid conflicts with existing types
type ValidationResultInfo struct {
	ValidationID         string
	ThreadID             string
	StepID               string
	StepName             string
	IdempotencyKey       string
	Timestamp            time.Time
	Validations          []ValidationIssue
	OverallStatus        string
	HasCriticalViolation bool
	CriticalCount        int
	WarningCount         int
	MinorCount           int
	InfoCount            int
	TotalValidations     int
}

// ValidationIssue represents a single validation violation or warning
// Renamed from ValidationNotification to avoid conflicts with stream notifications
type ValidationIssue struct {
	Type     string // "critical", "warning", "info"
	Message  string
	Field    string
	Expected string
	Actual   string
	Rule     string
}

// ValidationQueryOptions for filtering validation results
type ValidationQueryOptions struct {
	ThreadID       string
	StepID         string
	StepName       string
	IdempotencyKey string
	ValidationType string // "critical", "warning", "info"
	Limit          int
	Offset           int
}
