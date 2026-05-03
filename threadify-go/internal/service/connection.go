package service

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/threadify/engine/internal/domain"
)

// ConnectionService implements the ConnectionManager interface with in-memory client tracking.
type ConnectionService struct {
	clients       map[string]*domain.ConnectedClient
	sessionCounts map[string]int
	mu            sync.RWMutex
	logger        *zap.Logger
}

// NewConnectionService creates a new connection service.
func NewConnectionService(logger *zap.Logger) domain.ConnectionManager {
	return &ConnectionService{
		clients:       make(map[string]*domain.ConnectedClient),
		sessionCounts: make(map[string]int),
		logger:        logger,
	}
}

// ConnectWithOwnerAndCompany registers a new client session.
// If the owner is already connected, all fields are refreshed to reflect the latest connection.
// Note: ApiKey is stored in plaintext in memory — consider storing only its hash.
func (c *ConnectionService) ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, exists := c.clients[ownerID]; exists {
		existing.ConnectedAt = time.Now()
		existing.ApiKey = apiKey
		existing.ServiceName = serviceName
		existing.CompanyID = companyID
	} else {
		c.clients[ownerID] = &domain.ConnectedClient{
			OwnerID:          ownerID,
			CompanyID:        companyID,
			ApiKey:           apiKey,
			ServiceName:      serviceName,
			ConnectedAt:      time.Now(),
			SubscribedEvents: []string{},
		}
	}

	c.sessionCounts[ownerID]++
	c.logger.Info("session connected",
		zap.String("owner_id", ownerID),
		zap.Int("total_sessions", c.sessionCounts[ownerID]),
	)
	return nil
}

// Disconnect decrements the session count for an owner.
// The client entry is removed only when the last session closes.
func (c *ConnectionService) Disconnect(ownerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	count, exists := c.sessionCounts[ownerID]
	if !exists {
		c.logger.Warn("disconnect called for unknown owner", zap.String("owner_id", ownerID))
		return nil
	}

	switch {
	case count > 1:
		c.sessionCounts[ownerID]--
		c.logger.Info("session disconnected",
			zap.String("owner_id", ownerID),
			zap.Int("remaining_sessions", c.sessionCounts[ownerID]),
		)
	case count == 1:
		delete(c.clients, ownerID)
		delete(c.sessionCounts, ownerID)
		c.logger.Info("last session disconnected", zap.String("owner_id", ownerID))
	default:
		c.logger.Warn("invalid session count during disconnect",
			zap.String("owner_id", ownerID),
			zap.Int("count", count),
		)
		delete(c.clients, ownerID)
		delete(c.sessionCounts, ownerID)
	}

	return nil
}

// GetClient retrieves a connected client by owner ID.
func (c *ConnectionService) GetClient(ownerID string) (*domain.ConnectedClient, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	client, exists := c.clients[ownerID]
	return client, exists
}

// IsConnected reports whether a client currently has at least one active session.
func (c *ConnectionService) IsConnected(ownerID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.clients[ownerID]
	return exists
}

// GetClientCompany returns the company ID for a connected client.
func (c *ConnectionService) GetClientCompany(ownerID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	client, exists := c.clients[ownerID]
	if !exists {
		return "", false
	}
	return client.CompanyID, true
}

// GetSessionCount returns the number of active sessions for an owner.
// Returns 0 when the owner is not connected.
func (c *ConnectionService) GetSessionCount(ownerID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionCounts[ownerID]
}
