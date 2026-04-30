package domain

import (
	"time"
)

type ContractListOptions struct {
	Search string
	Limit  int
	Offset int
}

type ContractListResult struct {
	Contracts  []*Contract
	TotalCount int
}

type UpdateContractParams struct {
	ContractID    string
	Description   string
	ContentHash   string
	LatestVersion int
	UpdatedAt     time.Time
}

type StepStatusQuery struct {
	ThreadID       string
	StepName       string
	ExpectedStatus string
	IdempotencyKey string
}

type ThreadReadOptions struct {
	WriteBack bool
}

type AccessReadOptions struct {
	WriteBack bool
}

type StepStateSnapshot struct {
	ID             string
	ThreadID       string
	StepName       string
	IdempotencyKey string
	Status         string
	RetryCount     int
	FirstSeenAt    time.Time
	LastUpdatedAt  time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	PreviousStep   string
	Actor          string
	ActorService   string
	LatestContext  string
}

type StepWithTimestamp struct {
	StepName    string
	CompletedAt time.Time
}

type UserAccess struct {
	Roles       []string
	RuntimeRole string
	Permissions []string
	GrantedBy   string
	GrantedAt   time.Time
	UpdatedAt   *time.Time
	Status      string
}

type GrantAccessParams struct {
	ThreadID    string
	UserID      string
	Role        string
	RuntimeRole string
	Permissions []string
	InvitedBy   string
	LuaRegistry LuaRegistry
	ThreadData  *string
	ThreadTTL   *int
}

type DebitParams struct {
	BalanceKey        string
	ChargedKey        string
	PendingKey        string
	StreamKey         string
	Cost              int64
	MinBalance        int64
	TopupAmount       int64
	MaxMonthly        int64
	AllowTopup        bool
	SpendEventID      string
	TopupEventID      string
	CompanyID         string
	BillingCycleStart time.Time
	OccurredAt        time.Time
	SeedBalance       int64
	SeedCharged       int64
}

type DebitResult struct {
	NewBalance    int64
	Allowed       int64
	SpendStreamID string
	TopupStreamID string
	TopupApplied  int64
}

type NATSMessage struct {
	Subject string
	Data    []byte
}

type PermissionCheckResult struct {
	HasAccess   bool
	HasFullRead bool
	HasOwnRead  bool
	Permissions []string
	RuntimeRole string
}

// ValidateStepParams contains all parameters for step validation and update
type ValidateStepParams struct {
	ThreadID               string
	StepID                 string
	StepName               string
	IdempotencyKey         string
	Status                 string
	ExistingViolations     []Violation
	IsTerminalStep         bool
	Timestamp              string
	MaxRetries             int
	TransitionsMap         map[string][]string // Map of stepName -> allowed next steps
	TerminalSteps          []string
	AllowMultipleTerminals bool
	Actor                  string // User who recorded this step (for .own permission filtering)
}

// Violation represents a validation violation
type Violation struct {
	Type     string
	Severity string
	Message  string
	Details  map[string]interface{}
}

// StepStateResult contains the results of step validation and update
type StepStateResult struct {
	Status               string
	Violations           []Violation
	RetryCount           int
	HasCriticalViolation bool
	FirstSeenAt          string
	PreviousStep         string
}
