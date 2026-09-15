package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

type ServiceManager struct {
	services []domain.BackgroundService
	logger   *zap.Logger
	mu       sync.Mutex
}

func NewServiceManager(logger *zap.Logger) *ServiceManager {
	return &ServiceManager{
		services: make([]domain.BackgroundService, 0),
		logger:   logger,
	}
}

func (m *ServiceManager) Register(s domain.BackgroundService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.services = append(m.services, s)
}

func (m *ServiceManager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.services {
		if err := s.Start(); err != nil {
			return fmt.Errorf("failed to start service %T: %w", s, err)
		}
		m.logger.Info("started background service", zap.String("service", fmt.Sprintf("%T", s)))
	}
	return nil
}

func (m *ServiceManager) StopAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = m.StopAllContext(ctx)
}

func (m *ServiceManager) StopAllContext(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result error
	for i := len(m.services) - 1; i >= 0; i-- {
		s := m.services[i]
		var err error
		if bounded, ok := s.(interface{ StopContext(context.Context) error }); ok {
			err = bounded.StopContext(ctx)
		} else {
			err = s.Stop()
		}
		if err != nil {
			m.logger.Error("failed to stop service", zap.String("service", fmt.Sprintf("%T", s)), zap.Error(err))
			result = errors.Join(result, err)
		}
	}
	return result
}
