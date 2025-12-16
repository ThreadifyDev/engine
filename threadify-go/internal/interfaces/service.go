package interfaces

import "github.com/threadify/engine/internal/models"

// StepEventProcessor defines the interface for step event processing
type StepEventProcessor interface {
	ProcessStepEvent(event models.StepEvent) error
	Start() error
	Stop() error
}

// ConnectionManager defines the interface for WebSocket connection management
type ConnectionManager interface {
	Connect(ownerID, apiKey, serviceName string) error
	Disconnect(ownerID string) error
	GetClient(ownerID string) (*models.ConnectedClient, bool)
	IsConnected(ownerID string) bool
}

// CacheManager defines the interface for caching operations
type CacheManager interface {
	GetThread(threadID string) (*models.Thread, bool)
	SetThread(threadID string, thread *models.Thread)
	GetContractGraph(contractID string, version int) (*models.ContractGraph, bool)
	SetContractGraph(contractID string, version int, graph *models.ContractGraph)
	ClearThreadCache(threadID string)
	ClearContractCache(contractID string, version int)
}

// ContractValidator defines the interface for contract validation operations
type ContractValidator interface {
	ValidateStepInContract(contractID string, version int, stepName string, context map[string]string) error
	GetContractGraph(contractID string, version int) (*models.ContractGraph, error)
	LoadContractGraphIntoCache(contractID string, version int) error
}
