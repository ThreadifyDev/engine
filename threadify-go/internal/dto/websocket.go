package dto

import "github.com/threadify/engine/internal/domain"

type ConnectRequest struct {
	Action           string   `json:"action"`
	ApiKey           string   `json:"apiKey"`
	ServiceName      string   `json:"serviceName,omitempty"`
	SubscribedEvents []string `json:"subscribedEvents"`
	MaxInFlight      int      `json:"maxInFlight,omitempty"` // Client-specified max unACKed notifications
}

type ConnectResponse struct {
	Action           string   `json:"action"`
	Status           string   `json:"status"`
	Message          string   `json:"message"`
	OwnerID          string   `json:"ownerId,omitempty"`
	CompanyID        string   `json:"companyId,omitempty"`
	SubscribedEvents []string `json:"subscribedEvents,omitempty"`
}

type StartThreadRequest struct {
	Action       string            `json:"action"`
	ThreadKey    string            `json:"threadKey,omitempty"`
	ServiceName  string            `json:"serviceName,omitempty"`
	Label        string            `json:"label,omitempty"`
	ContractName string            `json:"contractName"`
	Role         string            `json:"role"`
	Refs         map[string]string `json:"refs,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
}

type StartThreadResponse struct {
	Action          string            `json:"action"`
	Status          string            `json:"status"`
	Message         string            `json:"message"`
	ThreadID        string            `json:"threadId,omitempty"`
	ThreadKey       string            `json:"threadKey,omitempty"`
	Label           string            `json:"label,omitempty"`
	ContractID      *string           `json:"contractId,omitempty"`
	ContractName    string            `json:"contractName,omitempty"`
	ContractVersion *int              `json:"contractVersion,omitempty"`
	Refs            map[string]string `json:"refs,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
}

type RecordEventRequest struct {
	WaitFor           bool                   `json:"waitFor,omitempty"`
	TimeoutMs         int                    `json:"timeoutMs,omitempty"`
	InvocationID      string                 `json:"invocationId,omitempty"`
	Action            string                 `json:"action"`
	ThreadID          string                 `json:"threadId"`
	StepName          string                 `json:"stepName"`
	Type              string                 `json:"type"`
	StartedAt         string                 `json:"startedAt"`
	FinishedAt        string                 `json:"finishedAt"`
	Context           map[string]string      `json:"context"`
	Refs              map[string]string      `json:"refs,omitempty"`
	Status            string                 `json:"status"`
	ServiceName       string                 `json:"serviceName,omitempty"`
	IdempotencyKey    string                 `json:"idempotencyKey,omitempty"`
	ThreadifyMetadata map[string]interface{} `json:"threadify_metadata,omitempty"`
	SubSteps          []SubStepRequest       `json:"subSteps,omitempty"`
}

type RecordEventResponse struct {
	Validation  *domain.WaitResult `json:"validation,omitempty"`
	Action      string             `json:"action"`
	Status      string             `json:"status"`
	Message     string             `json:"message"`
	ThreadID    string             `json:"threadId,omitempty"`
	StepID      string             `json:"stepId,omitempty"`
	IsDuplicate bool               `json:"isDuplicate,omitempty"`
}

type AddRefsRequest struct {
	Action   string            `json:"action"`
	ThreadID string            `json:"threadId"`
	Refs     map[string]string `json:"refs"`
}

type AddRefsResponse struct {
	Action   string `json:"action"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	ThreadID string `json:"threadId,omitempty"`
}

type CloseConnectionResponse struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

type ThreadWithRefs struct {
	ThreadID   string            `json:"threadId"`
	ContractID string            `json:"contractId"`
	Role       string            `json:"role"`
	Refs       map[string]string `json:"refs"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}

type UserThreadPermissions struct {
	ThreadID    string   `json:"threadId"`
	UserID      string   `json:"userId"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	JoinedAt    string   `json:"joinedAt"`
}
