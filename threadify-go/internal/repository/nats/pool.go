package nats

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/config"
	"go.uber.org/zap"
)

type Pool struct {
	clients []*Client
	logger  *zap.Logger
	index   uint32
	size    int
}

func NewPool(cfg *config.NATSConfig, size int, logger *zap.Logger, options ...nats.Option) (*Pool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pool size must be greater than 0")
	}

	pool := &Pool{
		clients: make([]*Client, size),
		logger:  logger,
		size:    size,
	}

	for i := range pool.clients {
		client, err := NewClient(cfg, logger, options...)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("create NATS client %d: %w", i, err)
		}
		pool.clients[i] = client
	}

	initClient, err := NewClient(cfg, logger, options...)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("create NATS init client: %w", err)
	}
	defer initClient.Close()

	if err := initClient.InitStreams(context.Background()); err != nil {
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

// Drain flushes pending publications and waits for every client to close. Stop
// producers and persistence workers first; no new work may use this pool while
// it is draining. Close remains safe for forced cleanup after a timeout.
func (p *Pool) Drain(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	var errs []error
	for _, client := range p.clients {
		if client == nil || client.conn.IsClosed() {
			continue
		}
		if err := client.conn.FlushWithContext(ctx); err != nil {
			errs = append(errs, fmt.Errorf("flush NATS client: %w", err))
		}
		if err := client.conn.Drain(); err != nil {
			errs = append(errs, fmt.Errorf("drain NATS client: %w", err))
		}
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		closed := true
		for _, client := range p.clients {
			if client != nil && !client.conn.IsClosed() {
				closed = false
				break
			}
		}
		if closed {
			return errors.Join(errs...)
		}
		select {
		case <-ctx.Done():
			errs = append(errs, ctx.Err())
			p.Close()
			return errors.Join(errs...)
		case <-ticker.C:
		}
	}
}
