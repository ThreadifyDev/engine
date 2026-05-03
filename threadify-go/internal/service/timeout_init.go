package service

import (
	"github.com/nats-io/nats.go"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

func InitializeTimeoutMonitor(
	nc *nats.Conn,
	notificationPub domain.NotificationPublisher,
	logger *zap.Logger,
) (*TimeoutMonitor, error) {
	monitor, err := NewTimeoutMonitor(nc, notificationPub, logger)
	if err != nil {
		return nil, err
	}

	go func() {
		if err := monitor.Start(); err != nil {
			logger.Error("timeout monitor stopped with error", zap.Error(err))
		}
	}()

	return monitor, nil
}
