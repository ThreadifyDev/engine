package types

import (
	"context"
	"time"

	"threadify-go/shared/billing"
	"threadify-go/shared/rbac"

	sharedauth "threadify-go/shared/auth"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/pkg/validator"
)

//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/service_mocks.go -source=service.go
//go:generate mockgen -package=enginemocks -destination=../service/mocks/engine/plan_service_mock.go threadify-go/shared/billing PlanService

// StepEventProcessor defines the interface for step event processing
type StepEventProcessor interface {
	RecordStepEventDirect(ctx context.Context, event models.StepEvent, ownerID, serviceName string, subSteps []models.SubStepRequest) error
	Start() error
	Stop() error
}

// ConnectionManager defines the interface for WebSocket connection management
type ConnectionManager interface {
	ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID string) error
	Disconnect(ownerID string) error
	GetClient(ownerID string) (*models.ConnectedClient, bool)
	IsConnected(ownerID string) bool
	GetClientCompany(ownerID string) (string, bool)
}

// CacheManager defines the interface for caching operations
type CacheManager interface {
	// Thread caching
	GetThread(threadID string) (*models.Thread, bool)
	SetThread(threadID string, thread *models.Thread)
	ClearThreadCache(threadID string)

	// Role caching (per-user per-thread)
	GetUserRole(threadID, userID string) (string, bool)
	SetUserRole(threadID, userID, role string)

	// Clear all roles for a thread
	ClearThreadRoles(threadID string)

	// Contract graph caching
	GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, bool)
	SetContractGraph(contractName string, version int, companyID string, graph *models.ContractGraph)
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
	ValidateStepInContract(contractName string, version int, stepName string, context map[string]string, companyID string) error
	ValidateStepContext(stepNode models.GraphNode, context map[string]string) error
	GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, error)
	LoadContractGraphIntoCache(contractName string, version int, companyID string) (int, error)
	GetContractByNameAndCompany(contractName string, companyID string) (*models.Contract, error)
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
	ScheduleTimeout(event models.TimeoutEvent) error
	CancelTimeout(timeoutID, threadID, reason string) error
}

// ContractService defines the interface for contract management
type ContractService interface {
	GetAllContracts(ctx context.Context, ownerID string, search string, limit, offset int) (int, interface{})
	CreateContract(ctx context.Context, ownerID, companyID, createdBy, contractYAML string) (int, interface{})
	GetContract(ctx context.Context, contractID, requesterID string, version *int) (int, interface{})
	UpdateContract(ctx context.Context, contractID, ownerID, createdBy, contractYAML string) (int, interface{})
	DeleteContract(ctx context.Context, contractID, ownerID string) (int, interface{})
	GetAllContractVersions(ctx context.Context, contractID, requesterID string) (int, interface{})
	GetContractVersion(ctx context.Context, contractID string, version int, requesterID string) (int, interface{})
	DeleteContractVersion(ctx context.Context, contractID string, version int, ownerID string) (int, interface{})
	PreviewContract(yamlString string) (*validator.Contract, *models.ContractGraph, *validator.ValidationResult, error)
}

// ThreadService defines the interface for thread-related operations
type ThreadService interface {
	HandleConnect(ctx context.Context, req *models.ConnectRequest) *models.ConnectResponse
	HandleStartThread(ctx context.Context, req *models.StartThreadRequest, ownerID, companyID string) *models.StartThreadResponse
	HandleRecordEvent(ctx context.Context, req *models.RecordEventRequest, ownerID, companyID string) *models.RecordEventResponse
	HandleInviteParty(req *models.InvitePartyRequest, ownerID, companyID string, threadIDs []string) (*models.InvitePartyResponse, error)
	HandleJoinThread(req *models.JoinThreadRequest, ownerID, companyID string) (*models.JoinThreadResponse, error)
	HandleClose(ownerID string) *models.CloseConnectionResponse
	HandleAddRefs(ctx context.Context, req *models.AddRefsRequest, ownerID string) *models.AddRefsResponse
	EndThread(ctx context.Context, threadID, actorID, actorService, status, reason string, recordedAt time.Time) error
}

// InvitationTokenService defines the interface for invitation JWT token operations
type InvitationTokenService interface {
	CreateToken(threadID, userID, role, accessLevel string, expiry time.Duration) (string, error)
	ValidateToken(tokenString string) (*models.ThreadInvitationClaims, error)
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
	PublishNotification(ctx context.Context, notification models.ValidationNotification) error
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
