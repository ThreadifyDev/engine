package interfaces

import (
	"context"
	"time"

	"github.com/threadify/engine/internal/models"
)

// ValkeyClient defines the interface for Valkey operations
type ValkeyClient interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	TTL(ctx context.Context, key string) (time.Duration, error)
	// Hash operations for role and permission management
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	// List operations for event queues
	LPush(ctx context.Context, key string, values ...interface{}) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// Sorted set operations for execution graph
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRem(ctx context.Context, key string, members ...string) error
	ZCard(ctx context.Context, key string) (int64, error)
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// Set operations for notification recipient caching
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SRem(ctx context.Context, key string, members ...interface{}) error
	// Script operations for atomic operations
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	// Lua script operations
	ScriptLoad(ctx context.Context, script string) (string, error)
	EvalSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error)
	// Pipeline operations
	Pipeline() ValkeyPipeline
	// Retry operations with exponential backoff
	ExecuteWithBackoff(ctx context.Context, operation func() error) error
}

// ValkeyPipeline defines the interface for Redis pipeline operations
type ValkeyPipeline interface {
	HSet(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	HDel(ctx context.Context, key string, fields ...string) ValkeyPipeline
	LPush(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) ValkeyPipeline
	Del(ctx context.Context, keys ...string) ValkeyPipeline
	Expire(ctx context.Context, key string, expiration time.Duration) ValkeyPipeline
	Exec(ctx context.Context) ([]interface{}, error)
}

// ThreadRepository defines the interface for thread CRUD operations with hot/cold fallback
type ThreadRepository interface {
	// Core CRUD operations
	Save(ctx context.Context, thread *models.Thread) error
	Delete(ctx context.Context, threadID string) error
	Exists(ctx context.Context, threadID string) (bool, error)
	GetByOwner(ctx context.Context, ownerID string) ([]string, error)
	ExtendTTL(ctx context.Context, threadID string) error
	AddRefs(ctx context.Context, threadID string, refs map[string]string) error

	// Hot/cold fallback operations (writeBack defaults to false)
	// Get retrieves thread from Valkey (hot) or PostgreSQL (cold)
	// writeBack: if true, caches PostgreSQL data back to Valkey
	Get(ctx context.Context, threadID string, writeBack ...bool) (*models.Thread, error)

	// GetStepStatus checks step status in Valkey or PostgreSQL
	GetStepStatus(ctx context.Context, threadID, stepName, stepStatus, idempotencyKey string, writeBack ...bool) (string, error)

	// GetCompletedStepsCount returns count of completed steps
	GetCompletedStepsCount(ctx context.Context, threadID string, writeBack ...bool) (int64, error)

	// GetCompletedSteps returns list of completed step names
	GetCompletedSteps(ctx context.Context, threadID string, writeBack ...bool) ([]string, error)
}

// AccessRepository defines the interface for role and permission management with hot/cold fallback
type AccessRepository interface {
	// Write operations
	// GrantOrUpdateAccess grants or updates user access with thread role and runtime_role
	// role: Thread-specific business role (e.g., "merchant", "supplier")
	// runtimeRole: Runtime-level permission scope (e.g., "owner", "participant", "observer")
	// permissions: Resolved permissions from runtime_role (e.g., ["notification.violations.*", "thread.read"])
	// runtime_role and permissions are stored in both Valkey (for fast permission checks) and PostgreSQL (via ActivityRepository)
	GrantOrUpdateAccess(ctx context.Context, threadID, userID string, role string, runtimeRole string, permissions []string, invitedBy string, luaScripts LuaScriptManager, threadData *string, threadTTL *int) (*UserAccess, error)
	RevokeAccess(ctx context.Context, threadID, userID string) error

	// Hot/cold fallback read operations (writeBack defaults to false)
	// GetUserAccess retrieves user access from Valkey or PostgreSQL
	GetUserAccess(ctx context.Context, threadID, userID string, writeBack ...bool) (*UserAccess, error)

	// GetAllAccess retrieves all user access for a thread
	GetAllAccess(ctx context.Context, threadID string, writeBack ...bool) (map[string]*UserAccess, error)
}

// ActivityRepository defines the interface for stream and event operations with hot/cold fallback
type ActivityRepository interface {
	// Write operations (archival)
	RecordAccessGranted(ctx context.Context, threadID, userID string, access *UserAccess, invitedBy, serviceName, runtimeRole string) error
	RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error
	RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error
	ArchiveValidationResults(ctx context.Context, threadID string, stepID string, stepName string, idempotencyKey string, notifications []models.ValidationNotification, finalStatus string, hasCriticalViolation bool) error
	ArchiveThreadMetadata(ctx context.Context, thread *models.Thread, status string) error
	ArchiveStepState(ctx context.Context, stepState *StepStateSnapshot) error

	// Read operations
	// GetActivityLog retrieves activity log from PostgreSQL
	// Returns activity events as map[string]interface{} (matches NATS/archival format)
	// Note: Activities are never cached in Valkey, so no write-back is performed
	GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error)
}

// StepStateSnapshot represents a snapshot of step state for archival
type StepStateSnapshot struct {
	ID             string `json:"id"`
	ThreadID       string `json:"thread_id"`
	StepName       string `json:"step_name"`
	IdempotencyKey string `json:"idempotency_key"`
	Status         string `json:"status"`
	RetryCount     int    `json:"retry_count"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastUpdatedAt  string `json:"last_updated_at"`
	PreviousStep   string `json:"previous_step,omitempty"`
}

// StepWithTimestamp represents a completed step with its completion time
type StepWithTimestamp struct {
	StepName    string
	CompletedAt time.Time
}

// UserAccess represents merged access control structure
type UserAccess struct {
	Roles       []string `json:"roles"`        // Thread-specific business roles (e.g., ["merchant", "supplier"])
	RuntimeRole string   `json:"runtime_role"` // Runtime-level permission scope (e.g., "owner", "participant", "observer")
	Permissions []string `json:"permissions"`  // Resolved permissions from runtime_role (e.g., ["notification.violations.*", "thread.read"])
	GrantedBy   string   `json:"granted_by"`
	GrantedAt   string   `json:"granted_at"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Status      string   `json:"status"`
}

// ContractGraphRepository defines the interface for contract graph operations
type ContractGraphRepository interface {
	Get(ctx context.Context, contractName string, version int, companyID string) (*models.ContractGraph, error)
	Save(ctx context.Context, contractName string, version int, companyID string, graph *models.ContractGraph) error
	Delete(ctx context.Context, contractName string, version int, companyID string) error
	Exists(ctx context.Context, contractName string, version int, companyID string) (bool, error)
}

// LuaScriptManager defines the interface for Lua script management
type LuaScriptManager interface {
	GetScriptHash(name string) (string, bool)
	CheckUserRateLimit(ctx context.Context, userID string, requestsPerMinute int, windowSeconds int) (bool, error)
}
