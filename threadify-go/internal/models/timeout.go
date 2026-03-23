package models

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
	ID           string      `json:"id"`
	ThreadID     string      `json:"thread_id"`
	Type         TimeoutType `json:"type"`
	FromStep     string      `json:"from_step,omitempty"`      // For transition timeouts
	ToStep       string      `json:"to_step,omitempty"`        // For transition timeouts
	StepName     string      `json:"step_name,omitempty"`      // For step timeouts
	Timeout      string      `json:"timeout"`                  // Duration string (e.g., "5m")
	ScheduledAt  time.Time   `json:"scheduled_at"`             // When timeout was scheduled
	DeadlineAt   time.Time   `json:"deadline_at"`              // When timeout should fire
	ContractName string      `json:"contract_name,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TimeoutCancellation represents a cancellation flag in KV store
type TimeoutCancellation struct {
	TimeoutID   string    `json:"timeout_id"`
	ThreadID    string    `json:"thread_id"`
	CancelledAt time.Time `json:"cancelled_at"`
	Reason      string    `json:"reason"`
}
