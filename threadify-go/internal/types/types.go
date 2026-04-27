package types

import (
	"time"

	"github.com/threadify/engine/internal/models"
)

type ContractListOptions struct {
	Search string
	Limit  int
	Offset int
}

type ContractListResult struct {
	Contracts  []*models.Contract
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
	ID             string     `json:"id"`
	ThreadID       string     `json:"thread_id"`
	StepName       string     `json:"step_name"`
	IdempotencyKey string     `json:"idempotency_key"`
	Status         string     `json:"status"`
	RetryCount     int        `json:"retry_count"`
	FirstSeenAt    time.Time  `json:"first_seen_at"`
	LastUpdatedAt  time.Time  `json:"last_updated_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	PreviousStep   string     `json:"previous_step,omitempty"`
	Actor          string     `json:"actor,omitempty"`
	ActorService   string     `json:"actor_service,omitempty"`
	LatestContext  string     `json:"latest_context,omitempty"`
}

type StepWithTimestamp struct {
	StepName    string
	CompletedAt time.Time
}

type UserAccess struct {
	Roles       []string   `json:"roles"`
	RuntimeRole string     `json:"runtime_role"`
	Permissions []string   `json:"permissions"`
	GrantedBy   string     `json:"granted_by"`
	GrantedAt   time.Time  `json:"granted_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	Status      string     `json:"status"`
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
	Type     string                 `json:"violationType"`
	Severity string                 `json:"severity"`
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// StepStateResult contains the results of step validation and update
type StepStateResult struct {
	Status               string      `json:"status"`
	Violations           []Violation `json:"violations"`
	RetryCount           int         `json:"retryCount"`
	HasCriticalViolation bool        `json:"hasCriticalViolation"`
	FirstSeenAt          string      `json:"firstSeenAt"`  // Timestamp when step was first seen
	PreviousStep         string      `json:"previousStep"` // Previous step key (stepName:idempKey)
}
