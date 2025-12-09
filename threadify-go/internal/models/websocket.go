package models

type ConnectRequest struct {
	Action           string   `json:"action"`
	ApiKey           string   `json:"apiKey"`
	OwnerID          string   `json:"ownerId"`
	SubscribedEvents []string `json:"subscribedEvents"`
}

type ConnectResponse struct {
	Action           string   `json:"action"`
	Status           string   `json:"status"`
	Message          string   `json:"message"`
	OwnerID          string   `json:"ownerId,omitempty"`
	SubscribedEvents []string `json:"subscribedEvents,omitempty"`
}

type StartThreadRequest struct {
	Action     string            `json:"action"`
	ContractID string            `json:"contractId,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type StartThreadResponse struct {
	Action     string `json:"action"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	ThreadID   string `json:"threadId,omitempty"`
	ContractID string `json:"contractId,omitempty"`
}

type RecordEventRequest struct {
	Action     string            `json:"action"`
	ThreadID   string            `json:"threadId"`
	StartedAt  string            `json:"startedAt,omitempty"`
	FinishedAt string            `json:"finishedAt,omitempty"`
	Context    map[string]string `json:"context,omitempty"`
	Status     string            `json:"status,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type RecordEventResponse struct {
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
