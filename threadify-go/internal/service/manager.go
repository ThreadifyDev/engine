package service

import (
	"fmt"
	"sync"

	"github.com/threadify/engine/internal/interfaces"
	"go.uber.org/zap"
)

type ServiceManager struct {
	services []interfaces.BackgroundService
	logger   *zap.Logger
	mu       sync.Mutex
}

func NewServiceManager(logger *zap.Logger) *ServiceManager {
	return &ServiceManager{
		services: make([]interfaces.BackgroundService, 0),
		logger:   logger,
	}
}

func (m *ServiceManager) Register(s interfaces.BackgroundService) {
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
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop in reverse order of registration
	for i := len(m.services) - 1; i >= 0; i-- {
		s := m.services[i]
		if err := s.Stop(); err != nil {
			m.logger.Error("failed to stop service", zap.String("service", fmt.Sprintf("%T", s)), zap.Error(err))
		} else {
			m.logger.Info("stopped background service", zap.String("service", fmt.Sprintf("%T", s)))
		}
	}
}
