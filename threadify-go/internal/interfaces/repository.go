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
	// Hash operations for role and permission management
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
	// List operations for event queues
	LPush(ctx context.Context, key string, values ...interface{}) error
	LRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// Stream operations for archiver
	XAdd(ctx context.Context, stream string, values map[string]interface{}) (string, error)
	XReadGroup(ctx context.Context, group, consumer, stream string, count int, block time.Duration) ([]map[string]interface{}, error)
	XAck(ctx context.Context, stream, group string, ids []string) error
	XGroupCreate(ctx context.Context, stream, group, start string) error
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	// Sorted set operations for execution graph
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRem(ctx context.Context, key string, members ...string) error
	ZCard(ctx context.Context, key string) (int64, error)
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)
	// Lua script operations
	ScriptLoad(ctx context.Context, script string) (string, error)
	EvalSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error)
	// Pipeline operations
	Pipeline() ValkeyPipeline
}

// ValkeyPipeline defines the interface for Redis pipeline operations
type ValkeyPipeline interface {
	HSet(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	HDel(ctx context.Context, key string, fields ...string) ValkeyPipeline
	LPush(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	XAdd(ctx context.Context, stream string, values map[string]interface{}) ValkeyPipeline
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) ValkeyPipeline
	Del(ctx context.Context, keys ...string) ValkeyPipeline
	Expire(ctx context.Context, key string, expiration time.Duration) ValkeyPipeline
	Exec(ctx context.Context) ([]interface{}, error)
}

// ThreadRepository defines the interface for thread CRUD operations only
type ThreadRepository interface {
	Save(ctx context.Context, thread *models.Thread) error
	Get(ctx context.Context, threadID string) (*models.Thread, error)
	Delete(ctx context.Context, threadID string) error
	Exists(ctx context.Context, threadID string) (bool, error)
	GetByOwner(ctx context.Context, ownerID string) ([]string, error)
	ExtendTTL(ctx context.Context, threadID string) error
	AddRefs(ctx context.Context, threadID string, refs map[string]string) error
}

// AccessRepository defines the interface for role and permission management
type AccessRepository interface {
	GrantOrUpdateAccess(ctx context.Context, threadID, userID string, role string, permissions []string, invitedBy string, luaScripts LuaScriptManager) (*UserAccess, error)
	GetUserAccess(ctx context.Context, threadID, userID string) (*UserAccess, error)
	GetAllAccess(ctx context.Context, threadID string) (map[string]*UserAccess, error)
	RevokeAccess(ctx context.Context, threadID, userID string) error
}

// ActivityRepository defines the interface for stream and event operations
type ActivityRepository interface {
	RecordAccessGranted(ctx context.Context, threadID, userID string, access *UserAccess, invitedBy, serviceName string) error
	RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error
	RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error
	StoreValidationNotification(ctx context.Context, notif models.ValidationNotification) error
	ArchiveValidationResults(ctx context.Context, threadID string, stepID string, stepName string, idempotencyKey string, notifications []models.ValidationNotification, finalStatus string, hasCriticalViolation bool) error
	ArchiveThreadMetadata(ctx context.Context, thread *models.Thread, status string) error
	ArchiveStepState(ctx context.Context, threadID string, stepID string, stepName string, idempotencyKey string, status string) error
}

// UserAccess represents merged access control structure
type UserAccess struct {
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	GrantedBy   string   `json:"granted_by"`
	GrantedAt   string   `json:"granted_at"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Status      string   `json:"status"`
}

// ContractGraphRepository defines the interface for contract graph operations
type ContractGraphRepository interface {
	Get(ctx context.Context, contractID string, version int) (*models.ContractGraph, error)
	Save(ctx context.Context, contractID string, version int, graph *models.ContractGraph) error
	Delete(ctx context.Context, contractID string, version int) error
	Exists(ctx context.Context, contractID string, version int) (bool, error)
}

// LuaScriptManager defines the interface for Lua script management
type LuaScriptManager interface {
	GetScriptHash(name string) (string, bool)
}

// Note: ValkeyClient is already properly defined in internal/repository/valkey/thread.go
// We should reuse that instead of redefining it here
