package domain

import "time"

// TimeoutType represents the type of timeout being monitored
type TimeoutType string

const (
	TimeoutTypeTransition  TimeoutType = "transition"
	TimeoutTypeMaxDuration TimeoutType = "max_duration"
	TimeoutTypeStep        TimeoutType = "step"
)

// TimeoutEvent represents a scheduled timeout check
type TimeoutEvent struct {
	ID           string
	ThreadID     string
	Type         TimeoutType
	FromStep     string // For transition timeouts
	ToStep       string // For transition timeouts
	StepName     string // For step timeouts
	Timeout      string // Duration string (e.g., "5m")
	ScheduledAt  time.Time
	DeadlineAt   time.Time
	ContractName string
	Metadata     map[string]interface{}
}

// TimeoutCancellation represents a cancellation flag in KV store
type TimeoutCancellation struct {
	TimeoutID   string
	ThreadID    string
	CancelledAt time.Time
	Reason      string
}
