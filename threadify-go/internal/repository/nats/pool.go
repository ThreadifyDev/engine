package nats

import (
	"fmt"
	"sync/atomic"

	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

type Pool struct {
	clients []*Client
	logger  *zap.Logger
	index   uint32
	size    int
}

func NewPool(cfg *config.NATSConfig, size int, logger *zap.Logger) (*Pool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pool size must be greater than 0")
	}

	pool := &Pool{
		clients: make([]*Client, size),
		logger:  logger,
		size:    size,
	}

	for i := range pool.clients {
		client, err := NewClient(cfg, logger)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("create NATS client %d: %w", i, err)
		}
		pool.clients[i] = client
	}

	initClient, err := NewClient(cfg, logger)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create NATS init client: %w", err)
	}
	defer initClient.Close()

	if err := initClient.InitStreams(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("initialize NATS streams: %w", err)
	}

	logger.Info("created NATS pool", zap.Int("connections", size))
	return pool, nil
}

func (p *Pool) GetClient() *Client {
	idx := atomic.AddUint32(&p.index, 1) % uint32(p.size)
	return p.clients[idx]
}

func (p *Pool) GetClientByIndex(index int) *Client {
	if index < 0 || index >= p.size {
		return p.GetClient()
	}
	return p.clients[index]
}

func (p *Pool) Size() int {
	return p.size
}

func (p *Pool) IsHealthy() bool {
	for _, client := range p.clients {
		if !client.IsConnected() {
			return false
		}
	}
	return true
}

func (p *Pool) Close() {
	for _, client := range p.clients {
		if client != nil {
			client.Close()
		}
	}
	p.logger.Info("NATS pool closed")
}
