package dto

import "time"

type StepEventResponse struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
	StepID  string `json:"step_id"`
}

type StepEvent struct {
	StepID         string                 `json:"step_id"`
	Type           string                 `json:"type"`
	Context        map[string]interface{} `json:"context"`
	Status         string                 `json:"status"`
	Timestamp      time.Time              `json:"timestamp"`
	ServiceName    string                 `json:"service_name"`
	ThreadID       string                 `json:"thread_id"`
	StepName       string                 `json:"step_name"`
	StartedAt      string                 `json:"started_at"`
	FinishedAt     string                 `json:"finished_at"`
	IdempotencyKey string                 `json:"idempotency_key"`
	ContentHash    string                 `json:"content_hash"`
	Metadata       map[string]interface{} `json:"metadata"`
}

type HashedStepEvent struct {
	StepEvent
	Hash         string    `json:"hash"`
	PreviousHash string    `json:"prev_hash"`
	CreatedAt    time.Time `json:"created_at"`
}
