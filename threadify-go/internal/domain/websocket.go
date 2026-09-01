package domain

type ConnectCmd struct {
	Action           string
	ApiKey           string
	ServiceName      string
	SubscribedEvents []string
	MaxInFlight      int
}

type ConnectResponse struct {
	Action           string
	Status           string
	Message          string
	OwnerID          string
	CompanyID        string
	SubscribedEvents []string
}

type StartThreadCmd struct {
	Action       string
	ThreadID     string
	Label        string
	ContractName string
	Role         string
	ServiceName  string
	Refs         map[string]string
	Tags         []string
}

type StartThreadResponse struct {
	Action   string
	Status   string
	Message  string
	ThreadID string
}

type RecordEventCmd struct {
	Action            string
	ThreadID          string
	StepName          string
	Type              string
	StartedAt         string
	FinishedAt        string
	Context           map[string]string
	Refs              map[string]string
	Status            string
	ServiceName       string
	IdempotencyKey    string
	ThreadifyMetadata map[string]interface{}
	SubSteps          []SubStepCmd
}

type RecordEventResponse struct {
	Action      string
	Status      string
	Message     string
	ThreadID    string
	StepID      string
	IsDuplicate bool
}

type AddRefsCmd struct {
	Action   string
	ThreadID string
	Refs     map[string]string
}

type AddRefsResponse struct {
	Action   string
	Status   string
	Message  string
	ThreadID string
}

type CloseConnectionResponse struct {
	Action  string
	Status  string
	Message string
}

type ErrorResponse struct {
	Action  string
	Status  string
	Message string
	Details string
}

type ThreadWithRefs struct {
	ThreadID   string
	ContractID string
	Role       string
	Refs       map[string]string
	CreatedAt  string
	UpdatedAt  string
}

type UserThreadPermissions struct {
	ThreadID    string
	UserID      string
	Role        string
	Permissions []string
	JoinedAt    string
}
