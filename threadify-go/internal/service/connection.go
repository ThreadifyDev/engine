package service

import (
	"log"
	"sync"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ConnectionService implements the ConnectionManager interface with in-memory client tracking
type ConnectionService struct {
	clients       map[string]*models.ConnectedClient
	sessionCounts map[string]int // Track number of active sessions per ownerID
	mu            sync.RWMutex   // Thread safety for concurrent access
}

// NewConnectionService creates a new connection service
func NewConnectionService() interfaces.ConnectionManager {
	return &ConnectionService{
		clients:       make(map[string]*models.ConnectedClient),
		sessionCounts: make(map[string]int),
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

	// Store client in memory (or update if exists)
	if _, exists := c.clients[ownerID]; !exists {
		c.clients[ownerID] = client
	} else {
		// Update ConnectedAt to reflect latest connection
		c.clients[ownerID].ConnectedAt = time.Now()
	}

	// Increment session count
	c.sessionCounts[ownerID]++
	log.Printf("[CONNECTION] Session connected for owner %s (total sessions: %d)", ownerID, c.sessionCounts[ownerID])
	return nil
}

// Disconnect removes a client connection (only removes if last session)
func (c *ConnectionService) Disconnect(ownerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Decrement session count
	if count, exists := c.sessionCounts[ownerID]; exists {
		if count <= 1 {
			// Last session closing, remove client
			log.Printf("[CONNECTION] Last session disconnected for owner %s, removing client", ownerID)
			delete(c.clients, ownerID)
			delete(c.sessionCounts, ownerID)
		} else if count > 1 {
			// Other sessions still active, just decrement
			c.sessionCounts[ownerID]--
			log.Printf("[CONNECTION] Session disconnected for owner %s (remaining sessions: %d)", ownerID, c.sessionCounts[ownerID])
		} else {
			// Safety check: count should never be < 1
			log.Printf("[CONNECTION] WARNING: Invalid session count %d for owner %s, cleaning up", count, ownerID)
			delete(c.clients, ownerID)
			delete(c.sessionCounts, ownerID)
		}
	} else {
		log.Printf("[CONNECTION] WARNING: Disconnect called for non-existent owner %s", ownerID)
	}
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
