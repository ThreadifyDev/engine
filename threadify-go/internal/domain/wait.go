package domain

import "context"

// WaitRequest is shared by SDKs; StepID selects a reported invocation's result.
// Without StepID, InvocationID identifies one idempotent permission request.
type WaitRequest struct {
	Await        bool   `json:"await,omitempty"`
	TimeoutMs    int    `json:"timeoutMs,omitempty"`
	ThreadID     string `json:"threadId"`
	StepName     string `json:"stepName"`
	StepID       string `json:"stepId,omitempty"`
	InvocationID string `json:"invocationId,omitempty"`
	Cancel       bool   `json:"cancel,omitempty"`
}

type WaitResult struct {
	Action       string      `json:"action"`
	Status       string      `json:"status"`
	ThreadID     string      `json:"threadId"`
	StepName     string      `json:"stepName"`
	StepID       string      `json:"stepId,omitempty"`
	InvocationID string      `json:"invocationId,omitempty"`
	Decision     string      `json:"decision"` // allowed, pending, passed, violated, unvalidated, cancelled, unavailable
	Message      string      `json:"message,omitempty"`
	Violations   []Violation `json:"violations,omitempty"`
}

type WaitService interface {
	HandleWaitFor(context.Context, WaitRequest, string, string) *WaitResult
}

// AwaitService returns a final decision while the WebSocket remains available.
type AwaitService interface {
	AwaitWaitFor(context.Context, WaitRequest, string, string) *WaitResult
}
