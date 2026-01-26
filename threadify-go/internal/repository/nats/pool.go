package nats

import (
	"fmt"
	"log"
	"sync/atomic"

	"github.com/threadify/engine/internal/config"
)

// Pool manages multiple NATS connections for load distribution
type Pool struct {
	clients []*Client
	index   uint32 // atomic counter for round-robin selection
	size    int
}

// NewPool creates a pool of NATS connections
func NewPool(cfg *config.NATSConfig, size int) (*Pool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pool size must be greater than 0")
	}

	pool := &Pool{
		clients: make([]*Client, size),
		size:    size,
	}

	// Create connections
	for i := 0; i < size; i++ {
		client, err := NewClient(cfg)
		if err != nil {
			// Clean up any already created connections
			pool.Close()
			return nil, fmt.Errorf("failed to create NATS client %d: %w", i, err)
		}
		pool.clients[i] = client
	}

	log.Printf("[NATS-POOL] Created pool with %d connections", size)
	return pool, nil
}

// GetClient returns a client using round-robin selection
func (p *Pool) GetClient() *Client {
	// Atomic increment and modulo for thread-safe round-robin
	idx := atomic.AddUint32(&p.index, 1) % uint32(p.size)
	return p.clients[idx]
}

// GetClientByIndex returns a specific client by index (for advanced use cases)
func (p *Pool) GetClientByIndex(index int) *Client {
	if index < 0 || index >= p.size {
		return p.GetClient() // Fallback to round-robin
	}
	return p.clients[index]
}

// Size returns the number of connections in the pool
func (p *Pool) Size() int {
	return p.size
}

// IsHealthy checks if all connections in the pool are healthy
func (p *Pool) IsHealthy() bool {
	for _, client := range p.clients {
		if !client.IsConnected() {
			return false
		}
	}
	return true
}

// Close closes all connections in the pool
func (p *Pool) Close() {
	for i, client := range p.clients {
		if client != nil {
			client.Close()
			log.Printf("[NATS-POOL] Closed connection %d", i)
		}
	}
	log.Printf("[NATS-POOL] Pool closed")
}
