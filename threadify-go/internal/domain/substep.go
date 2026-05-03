package domain

import "time"

type SubStep struct {
	ID         string
	ThreadID   string
	StepID     string
	Name       string
	Status     string // 'success' or 'failed'
	Payload    map[string]interface{}
	RecordedAt time.Time
	CreatedAt  time.Time
}

// SubStepCmd represents a sub-step from the SDK
type SubStepCmd struct {
	Name       string
	Status     string
	Payload    map[string]interface{}
	RecordedAt string
}
