package models

type ConnectRequest struct {
	Action           string   `json:"action"`
	ApiKey           string   `json:"apiKey"`
	ServiceName      string   `json:"serviceName,omitempty"`
	SubscribedEvents []string `json:"subscribedEvents"`
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
	ContractName string            `json:"contractName"`
	Role         string            `json:"role"`
	Refs         map[string]string `json:"refs,omitempty"`
}

type StartThreadResponse struct {
	Action   string `json:"action"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	ThreadID string `json:"threadId,omitempty"`
}

type RecordEventRequest struct {
	Action         string            `json:"action"`
	ThreadID       string            `json:"threadId"`
	StepName       string            `json:"stepName"`
	Type           string            `json:"type"`
	StartedAt      string            `json:"startedAt"`
	FinishedAt     string            `json:"finishedAt"`
	Context        map[string]string `json:"context"`
	Refs           map[string]string `json:"refs,omitempty"`
	Status         string            `json:"status"`
	ServiceName    string            `json:"serviceName,omitempty"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
}

type RecordEventResponse struct {
	Action      string `json:"action"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	ThreadID    string `json:"threadId,omitempty"`
	StepID      string `json:"stepId,omitempty"`
	IsDuplicate bool   `json:"isDuplicate,omitempty"`
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

// ThreadWithRefs represents a thread with external references
type ThreadWithRefs struct {
	ThreadID   string            `json:"threadId"`
	ContractID string            `json:"contractId"`
	Role       string            `json:"role"`
	Refs       map[string]string `json:"refs"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}

// UserThreadPermissions represents user permissions in a thread
type UserThreadPermissions struct {
	ThreadID    string   `json:"threadId"`
	UserID      string   `json:"userId"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	JoinedAt    string   `json:"joinedAt"`
}
