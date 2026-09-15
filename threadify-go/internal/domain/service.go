package domain

import (
	"context"
	"time"

	"threadify-go/shared/billing"
	"threadify-go/shared/rbac"

	sharedauth "threadify-go/shared/auth"

	"github.com/threadify/engine/pkg/validator"
)

//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/service_mocks.go -source=service.go
//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/plan_service_mock.go threadify-go/shared/billing PlanService

// StepEventProcessor defines the interface for step event processing
type StepEventProcessor interface {
	RecordStepEventDirect(ctx context.Context, event StepEvent, ownerID, serviceName string, subSteps []SubStepCmd) error
	Start() error
	Stop() error
}

// ConnectionManager defines the interface for WebSocket connection management
type ConnectionManager interface {
	ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID string) error
	Disconnect(ownerID string) error
	GetClient(ownerID string) (*ConnectedClient, bool)
	IsConnected(ownerID string) bool
	GetClientCompany(ownerID string) (string, bool)
}

// CacheManager defines the interface for caching operations
type CacheManager interface {
	// Thread caching
	GetThread(threadID string) (*Thread, bool)
	SetThread(threadID string, thread *Thread)
	ClearThreadCache(threadID string)

	// Role caching (per-user per-thread)
	GetUserRole(threadID, userID string) (string, bool)
	SetUserRole(threadID, userID, role string)

	ClearThreadRoles(threadID string)

	// Contract graph caching
	GetContractGraph(contractName string, version int, companyID string) (*ContractGraph, bool)
	SetContractGraph(contractName string, version int, companyID string, graph *ContractGraph)
	ClearContractCache(contractName string, version int, companyID string)

	// Step status caching (for duplicate detection)
	GetStepStatus(stepHashKey string) (string, bool)
	SetStepStatus(stepHashKey, status string)
	ClearStepStatus(stepHashKey string)

	// Runtime role permission caching (global, not per-user)
	GetRuntimeRolePermissions(runtimeRole string) ([]string, bool)
	SetRuntimeRolePermissions(runtimeRole string, permissions []string)
}

// ContractGraphValidator defines the interface for contract graph operations
type ContractGraphValidator interface {
	ValidateStepInContract(ctx context.Context, contractName string, version int, stepName string, context map[string]string, companyID string) error
	ValidateStepContext(ctx context.Context, stepNode GraphNode, context map[string]string) error
	GetContractGraph(ctx context.Context, contractName string, version int, companyID string) (*ContractGraph, error)
	LoadContractGraphIntoCache(ctx context.Context, contractName string, version int, companyID string) (int, error)
	GetContractByNameAndCompany(ctx context.Context, contractName string, companyID string) (*Contract, error)
}

// BackgroundService defines the interface for services that run in the background.
type BackgroundService interface {
	Start() error
	Stop() error
}

type PlanService = billing.PlanService

// ContractValidator defines the interface for contract YAML validation
type ContractValidator interface {
	Validate(yamlString string) (*validator.Contract, *validator.ValidationResult)
	SerializeContract(contract *validator.Contract) (string, string, error)
}

// TimeoutMonitor defines the interface for thread timeouts
type TimeoutMonitor interface {
	ScheduleTimeout(ctx context.Context, event TimeoutEvent) error
	CancelTimeout(ctx context.Context, timeoutID, threadID, reason string) error
}

// ContractService defines the interface for contract management
type ContractService interface {
	GetAllContracts(ctx context.Context, ownerID string, search string, limit, offset int) (int, interface{})
	CreateContract(ctx context.Context, ownerID, companyID, createdBy, contractYAML string) (int, interface{})
	GetContract(ctx context.Context, contractID, requesterID string, version *int) (int, interface{})
	UpdateContract(ctx context.Context, contractID, ownerID, createdBy, contractYAML string) (int, interface{})
	DeleteContract(ctx context.Context, contractID, ownerID string) (int, interface{})
	GetAllContractVersions(ctx context.Context, contractID, requesterID string, limit, offset int) (int, interface{})
	GetContractVersion(ctx context.Context, contractID string, version int, requesterID string) (int, interface{})
	DeleteContractVersion(ctx context.Context, contractID string, version int, ownerID string) (int, interface{})
	PreviewContract(yamlString string) (*validator.Contract, *ContractGraph, *validator.ValidationResult, error)
}

// ThreadService defines the interface for thread-related operations
type ThreadService interface {
	HandleConnect(ctx context.Context, req *ConnectCmd) *ConnectResponse
	HandleStartThread(ctx context.Context, req *StartThreadCmd, ownerID, companyID string) *StartThreadResponse
	HandleRecordEvent(ctx context.Context, req *RecordEventCmd, ownerID, companyID string) *RecordEventResponse
	HandleAddRefs(ctx context.Context, req *AddRefsCmd, ownerID string) *AddRefsResponse
	HandleInviteParty(ctx context.Context, req *InvitePartyCmd, ownerID, companyID string, threadIDs []string) (*InvitePartyResponse, error)
	HandleJoinThread(ctx context.Context, req *JoinThreadCmd, userID, companyID string) (*JoinThreadResponse, error)
	HandleClose(ownerID string) *CloseConnectionResponse
	EndThread(ctx context.Context, threadID, actorID, actorService, status, reason string, recordedAt time.Time) error
}

// OTelThreadWriter exposes the existing thread write path to stateless,
// authenticated telemetry ingestion without requiring a WebSocket session.
type OTelThreadWriter interface {
	CompleteTraceForIngestion(ctx context.Context, threadID, ownerID, companyID, traceID string, endedAt time.Time) error
	StartThreadForIngestion(ctx context.Context, req *StartThreadCmd, ownerID, companyID string) *StartThreadResponse
	RecordEventForIngestion(ctx context.Context, req *RecordEventCmd, ownerID, companyID string) *RecordEventResponse
	ValidateThreadForIngestion(ctx context.Context, threadID, ownerID, companyID string) error
}

// InvitationTokenService defines the interface for invitation JWT token operations
type InvitationTokenService interface {
	CreateToken(threadID, userID, role, accessLevel string, expiry time.Duration) (string, error)
	ValidateToken(tokenString string) (*ThreadInvitationClaims, error)
	ValidateAccessLevel(accessLevel string) error
	ParseExpiry(expiresIn string) (time.Duration, error)
}

// NotificationConsumer defines the interface for thread-level notification subscriptions
type NotificationConsumer interface {
	Unsubscribe(threadID, ownerID string) error
}

// NotificationRouter defines the interface for internal notification routing
type NotificationRouter interface {
	HandleConnect(sessionID, ownerID string, maxInFlight int, conn WSConnection, connMutex WSMutex) error
	HandleDisconnect(sessionID string) error
	HandleSubscribe(sessionID, stepName, contract string, eventTypes []string) error
	HandleAck(ackToken string) error
}

// NotificationPublisher defines the interface for publishing notifications.
type NotificationPublisher interface {
	PublishNotification(ctx context.Context, notification ValidationNotification) error
}

type UserInfo struct {
	OwnerID   string
	CompanyID string
	Role      string
}

type AuthService interface {
	ValidateApiKey(apiKey string) (*UserInfo, error)
	VerifyToken(ctx context.Context, token string) (*sharedauth.TokenClaims, error)
	GetUserRoles(ctx context.Context, userID, scope string, expiresAt time.Time) ([]string, error)
}

// RBACLoader defines the interface for RBAC operations
type RBACLoader interface {
	CheckPermission(userPermissions []string, required string) bool
	GetRolesByLevel(level string) map[string]rbac.Role
	GetPermissionsForRoles(roleNames []string, scopeLevel string) []string
}

// WSConnection is a mockable subset of websocket.Conn
type WSConnection interface {
	WriteJSON(v interface{}) error
}

// WSMutex is a mockable subset of sync.Mutex for connection writing
type WSMutex interface {
	Lock()
	Unlock()
}
