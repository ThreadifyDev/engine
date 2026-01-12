package interfaces

import "github.com/threadify/engine/internal/models"

// StepEventProcessor defines the interface for step event processing
type StepEventProcessor interface {
	RecordStepEventDirect(event models.StepEvent, ownerID, serviceName string) error
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
	GetContractGraph(contractID string, version int, ownerID string) (*models.ContractGraph, bool)
	SetContractGraph(contractID string, version int, ownerID string, graph *models.ContractGraph)
	ClearContractCache(contractID string, version int, ownerID string)

	// Permission caching
	GetUserPermissions(threadID, userID string) ([]string, bool)
	SetUserPermissions(threadID, userID string, permissions []string)

	// Role caching
	GetUserRole(threadID, userID string) (string, bool)
	SetUserRole(threadID, userID, role string)

	// Clear all permissions and roles for a thread
	ClearThreadPermissions(threadID string)
}

// ContractValidator defines the interface for contract validation operations
type ContractValidator interface {
	ValidateStepInContract(contractID string, version int, stepName string, context map[string]string, ownerID string) error
	ValidateStepContext(stepNode models.GraphNode, context map[string]string) error
	GetContractGraph(contractID string, version int, ownerID string) (*models.ContractGraph, error)
	LoadContractGraphIntoCache(contractID string, version int, ownerID string) (int, error)
	GetContractByNameAndCompany(contractName string, companyID string) (*models.Contract, error)
}
