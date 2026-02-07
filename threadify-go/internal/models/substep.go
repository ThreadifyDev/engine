package models

import "time"

// SubStep represents a sub-step within a parent step
type SubStep struct {
	ID          string                 `json:"id"`
	ThreadID    string                 `json:"threadId"`
	StepID      string                 `json:"stepId"`
	SubStepName string                 `json:"subStepName"`
	Status      string                 `json:"status"` // 'success' or 'failed'
	Payload     map[string]interface{} `json:"payload,omitempty"`
	RecordedAt  time.Time              `json:"recordedAt"`
	CreatedAt   time.Time              `json:"createdAt"`
}

// SubStepRequest represents a sub-step from the SDK
type SubStepRequest struct {
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	RecordedAt string                 `json:"recordedAt"`
}
