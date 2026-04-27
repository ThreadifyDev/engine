package types

import (
	"context"
	"time"

	billingmodels "threadify-go/shared/models"

	"github.com/threadify/engine/internal/models"
)

//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/repository_mocks.go -source=repository.go

type ContractRepository interface {
	Create(ctx context.Context, contract *models.Contract) error
	CreateContractWithVersion(ctx context.Context, contract *models.Contract, version *models.ContractVersion) error

	// Update applies the fields in params and returns the updated contract.
	Update(ctx context.Context, params UpdateContractParams) (*models.Contract, error)

	// GetByID is the canonical single-contract lookup by primary key.
	GetByID(ctx context.Context, contractID string) (*models.Contract, error)
	GetByIDAndOwner(ctx context.Context, contractID, ownerID string) (*models.Contract, error)

	GetByName(ctx context.Context, name string) (*models.Contract, error)
	// GetByNameSummary returns a contract with only basic fields populated (ID, Name, OwnerID).
	GetByNameSummary(ctx context.Context, name string) (*models.Contract, error)
	GetByNameAndCompany(ctx context.Context, name, companyID string) (*models.Contract, error)

	GetAllByOwner(ctx context.Context, ownerID string, opts ContractListOptions) (ContractListResult, error)

	CountByOwner(ctx context.Context, ownerID string) (int, error)

	SoftDelete(ctx context.Context, contractID string) error

	CreateVersion(ctx context.Context, v *models.ContractVersion) error
	GetVersion(ctx context.Context, contractID string, version int) (*models.ContractVersion, error)
	GetLatestVersion(ctx context.Context, contractID string) (*models.ContractVersion, error)
	GetAllVersions(ctx context.Context, contractID string) ([]*models.ContractVersion, error)

	SoftDeleteVersion(ctx context.Context, contractID string, version int) error
}

type AuthRepository interface {
	ValidateAPIKey(ctx context.Context, keyHash string) (*models.AuthInfo, error)
	GetUserRoles(ctx context.Context, principalID string, principalType string) ([]string, error)
}

type ActorRepository interface {
	ResolveActors(ctx context.Context, ids []string) ([]*models.ActorInfo, error)
}

type BillingRepository interface {
	CreateSnapshot(ctx context.Context, snapshot *billingmodels.BillingSnapshot) error
	FindSnapshotByInvoiceID(ctx context.Context, externalInvoiceID string) (*billingmodels.BillingSnapshot, error)
	UpdateSnapshotInvoiceID(ctx context.Context, snapshotID string, invoiceID string) error
	UpdateSnapshotPaymentStatus(ctx context.Context, snapshotID string, status billingmodels.PaymentStatus) error
	MarkSnapshotPaidByInvoiceID(ctx context.Context, externalInvoiceID string) error
	MarkSnapshotPaidByID(ctx context.Context, snapshotID string, externalInvoiceID string) error
	MarkSnapshotFailedByInvoiceID(ctx context.Context, externalInvoiceID string) error
}

// ValkeyStringClient covers plain key-value operations.
type ValkeyStringClient interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// SetNX sets key only if it does not already exist.
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
	Exists(ctx context.Context, key string) (bool, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	MGet(ctx context.Context, keys ...string) (map[string]string, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
	TTL(ctx context.Context, key string) (time.Duration, error)
}

// ValkeyHashClient covers hash (HSet/HGet/…) operations.
type ValkeyHashClient interface {
	HSet(ctx context.Context, key string, values ...interface{}) error
	HGet(ctx context.Context, key, field string) (string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HDel(ctx context.Context, key string, fields ...string) error
}

type ValkeySortedSetClient interface {
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZCard(ctx context.Context, key string) (int64, error)
	ZRange(ctx context.Context, key string, start, stop int64) ([]string, error)
}

type ValkeySetClient interface {
	SAdd(ctx context.Context, key string, members ...interface{}) error
	SMembers(ctx context.Context, key string) ([]string, error)
	SRem(ctx context.Context, key string, members ...interface{}) error
}


type ValkeyScriptClient interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
	ScriptLoad(ctx context.Context, script string) (string, error)
	EvalSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error)
}

type ValkeyStreamClient interface {
	XAdd(ctx context.Context, stream string, id string, values interface{}) (string, error)
}

// ValkeyPipelineProvider allows creating a pipeline for atomic/batched operations.
type ValkeyPipelineProvider interface {
	Pipeline() ValkeyPipeline
}

type ValkeyBackoffExecutor interface {
	ExecuteWithBackoff(ctx context.Context, operation func() error) error
}

type ValkeyCreditAtomic interface {
	ApplyCreditTopupAtomic(ctx context.Context, balanceKey, pendingKey string, amount int64) (int64, error)
}

type ValidationValkeyClient interface {
	ValkeyStringClient
	ValkeyHashClient
	ValkeyPipelineProvider
}

type AccessValkeyClient interface {
	ValkeyStringClient
	ValkeyHashClient
	ValkeySetClient
	ValkeyScriptClient
	ValkeyPipelineProvider
	ValkeyBackoffExecutor
}

type StepStateValkeyClient interface {
	ValkeyScriptClient
	ValkeyHashClient
	ValkeyStringClient
}

type ThreadValkeyClient interface {
	ValkeyStringClient
	ValkeyHashClient
	ValkeyScriptClient
	ValkeySortedSetClient
	ValkeyPipelineProvider
}

type StepEventValkeyClient interface {
	ValkeyStringClient
	ValkeyScriptClient
}

type PlanValkeyClient interface {
	ValkeyStringClient
	ValkeyScriptClient
}

type ValkeyClient interface {
	ValkeyStringClient
	ValkeyHashClient
	ValkeySortedSetClient
	ValkeySetClient
	ValkeyScriptClient
	ValkeyPipelineProvider
	ValkeyBackoffExecutor
	ValkeyCreditAtomic
	ValkeyStreamClient
}


// ValkeyPipeline provides a fluent interface for batching Valkey commands.
// Note: Method signatures mirror ValkeyStringClient/ValkeyHashClient but return the pipeline itself for chaining.
type ValkeyPipeline interface {
	HSet(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	HDel(ctx context.Context, key string, fields ...string) ValkeyPipeline
	LPush(ctx context.Context, key string, values ...interface{}) ValkeyPipeline
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) ValkeyPipeline
	Del(ctx context.Context, keys ...string) ValkeyPipeline
	Expire(ctx context.Context, key string, expiration time.Duration) ValkeyPipeline
	Exec(ctx context.Context) ([]interface{}, error)
}

type ThreadRepository interface {
	Save(ctx context.Context, thread *models.Thread) error
	Delete(ctx context.Context, threadID string) error
	Exists(ctx context.Context, threadID string) (bool, error)
	GetByOwner(ctx context.Context, ownerID string) ([]string, error)
	ExtendTTL(ctx context.Context, threadID string) error
	AddRefs(ctx context.Context, threadID string, refs map[string]string) error

	Get(ctx context.Context, threadID string, opts ...ThreadReadOptions) (*models.Thread, error)

	GetStepStatus(ctx context.Context, query StepStatusQuery, opts ...ThreadReadOptions) (string, error)
	GetCompletedStepsCount(ctx context.Context, threadID string, opts ...ThreadReadOptions) (int64, error)
	GetCompletedSteps(ctx context.Context, threadID string, opts ...ThreadReadOptions) ([]string, error)

	UpdateThreadStatus(ctx context.Context, threadID string, status string, timestamp time.Time) error
}

type AccessRepository interface {
	GrantOrUpdateAccess(ctx context.Context, params GrantAccessParams) (*UserAccess, error)
	RevokeAccess(ctx context.Context, threadID, userID string) error
	GetUserAccess(ctx context.Context, threadID, userID string, opts ...AccessReadOptions) (*UserAccess, error)
	GetAllAccess(ctx context.Context, threadID string, opts ...AccessReadOptions) (map[string]*UserAccess, error)
}

type ActivityEventRepository interface {
	RecordAccessGranted(ctx context.Context, threadID, userID string, access *UserAccess, invitedBy, serviceName, runtimeRole string) error
	RecordInvitationUsed(ctx context.Context, threadID, userID, role, invitedBy, serviceName string) error
	RecordThreadCreated(ctx context.Context, threadID, creatorID, creatorRole, serviceName string) error
}

type ActivityArchiveRepository interface {
	ArchiveValidationResults(ctx context.Context, threadID string, stepID string, stepName string, idempotencyKey string, notifications []models.ValidationNotification, finalStatus string, hasCriticalViolation bool) error
	ArchiveThreadMetadata(ctx context.Context, thread *models.Thread, status string) error
	ArchiveStepState(ctx context.Context, stepState *StepStateSnapshot) error
	GetActivityLog(ctx context.Context, threadID string) ([]map[string]interface{}, error)
}

type ActivityRepository interface {
	ActivityEventRepository
	ActivityArchiveRepository
}

type ContractGraphRepository interface {
	Get(ctx context.Context, contractName string, version int, companyID string) (*models.ContractGraph, error)
	Save(ctx context.Context, contractName string, version int, companyID string, graph *models.ContractGraph) error
	Delete(ctx context.Context, contractName string, version int, companyID string) error
	Exists(ctx context.Context, contractName string, version int, companyID string) (bool, error)
}

type LuaRegistry interface {
	GetScriptHash(name string) (string, bool)
}

type RateLimiter interface {
	CheckCompanyRateLimit(ctx context.Context, companyID string, requestsPerMinute int, windowSeconds int) (bool, error)
	CheckIPRateLimit(ctx context.Context, ip string, requestsPerWindow int, windowSeconds int) (bool, error)
}

type CreditManager interface {
	DecrementCreditWithAutoTopup(ctx context.Context, params *DebitParams) (DebitResult, error)
	GetAndResetCharged(ctx context.Context, balanceKey, chargedKey string) (int64, int64, error)
}

type LuaScriptManager interface {
	LuaRegistry
	RateLimiter
	CreditManager
}

// NATSClient provides a minimal interface for consuming messages from NATS subjects.
// It is primarily used by NotificationConsumer for thread-level updates.
type NATSClient interface {
	FetchMessage(subject, consumerName string, timeout time.Duration) (*NATSMessage, error)
}
