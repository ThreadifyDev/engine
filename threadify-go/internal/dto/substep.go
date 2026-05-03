package dto

type SubStep struct {
	ID         string                 `json:"id"`
	ThreadID   string                 `json:"threadId"`
	StepID     string                 `json:"stepId"`
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	RecordedAt string                 `json:"recordedAt"`
	CreatedAt  string                 `json:"createdAt"`
}

type SubStepRequest struct {
	Name       string                 `json:"name"`
	Status     string                 `json:"status"`
	Payload    map[string]interface{} `json:"payload,omitempty"`
	RecordedAt string                 `json:"recordedAt"`
}
