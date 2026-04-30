package service

import (
	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

// InitializeTimeoutMonitor creates and initializes the timeout monitor
// This is a standalone function that can be called when ready to integrate
func InitializeTimeoutMonitor(
	nc *nats.Conn,
	threadRepo domain.ThreadRepository,
	notificationPub domain.NotificationPublisher,
	logger *zap.Logger,
) (*TimeoutMonitor, error) {
	monitor, err := NewTimeoutMonitor(nc, threadRepo, notificationPub, logger)
	if err != nil {
		return nil, err
	}

	// Start the consumer in a goroutine
	go func() {
		if err := monitor.Start(); err != nil {
			logger.Error("timeout monitor stopped with error", zap.Error(err))
		}
	}()

	return monitor, nil
}
