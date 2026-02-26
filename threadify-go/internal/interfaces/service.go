package interfaces

import (
	"context"

	"github.com/threadify/engine/internal/models"
)

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

	// Contract graph caching
	GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, bool)
	SetContractGraph(contractName string, version int, companyID string, graph *models.ContractGraph)
	ClearContractCache(contractName string, version int, companyID string)

	// Runtime role permission caching (global, not per-user)
	GetRuntimeRolePermissions(runtimeRole string) ([]string, bool)
	SetRuntimeRolePermissions(runtimeRole string, permissions []string)

	// Role caching (per-user per-thread)
	GetUserRole(threadID, userID string) (string, bool)
	SetUserRole(threadID, userID, role string)

	// Clear all roles for a thread
	ClearThreadRoles(threadID string)

	// Step status caching (for duplicate detection)
	GetStepStatus(stepHashKey string) (string, bool)
	SetStepStatus(stepHashKey, status string)
	ClearStepStatus(stepHashKey string)
}

// ContractValidator defines the interface for contract validation operations
type ContractValidator interface {
	ValidateStepInContract(contractName string, version int, stepName string, context map[string]string, companyID string) error
	ValidateStepContext(stepNode models.GraphNode, context map[string]string) error
	GetContractGraph(contractName string, version int, companyID string) (*models.ContractGraph, error)
	LoadContractGraphIntoCache(contractName string, version int, companyID string) (int, error)
	GetContractByNameAndCompany(contractName string, companyID string) (*models.Contract, error)
}
