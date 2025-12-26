package service

import (
	"sync"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ConnectionService implements the ConnectionManager interface with in-memory client tracking
type ConnectionService struct {
	clients map[string]*models.ConnectedClient
	mu      sync.RWMutex // Thread safety for concurrent access
}

// NewConnectionService creates a new connection service
func NewConnectionService() interfaces.ConnectionManager {
	return &ConnectionService{
		clients: make(map[string]*models.ConnectedClient),
	}
}

// ConnectWithOwnerAndCompany adds a new client connection with owner and company information
func (c *ConnectionService) ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	client := &models.ConnectedClient{
		OwnerID:          ownerID,
		CompanyID:        companyID,
		ApiKey:           apiKey,
		ServiceName:      serviceName,
		ConnectedAt:      time.Now(),
		SubscribedEvents: []string{}, // Default empty events
	}

	// Store client in memory
	c.clients[ownerID] = client
	return nil
}

// Disconnect removes a client connection
func (c *ConnectionService) Disconnect(ownerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.clients, ownerID)
	return nil
}

// GetClient retrieves a connected client by owner ID
func (c *ConnectionService) GetClient(ownerID string) (*models.ConnectedClient, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	client, exists := c.clients[ownerID]
	return client, exists
}

// IsConnected checks if a client is connected
func (c *ConnectionService) IsConnected(ownerID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.clients[ownerID]
	return exists
}

// GetClientCompany retrieves the company ID for a connected client
func (c *ConnectionService) GetClientCompany(ownerID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	client, exists := c.clients[ownerID]
	if !exists {
		return "", false
	}
	return client.CompanyID, true
}
