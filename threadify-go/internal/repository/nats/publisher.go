package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

const (
	unknownNotificationType = "unknown"
	globalContractName      = "global"
)

type Publisher struct {
	client *Client
}

func NewPublisher(client *Client) *Publisher {
	return &Publisher{client: client}
}

func (p *Publisher) PublishNotification(ctx context.Context, notification domain.ValidationNotification) error {
	contract := notification.ContractName
	if contract == "" {
		contract = "global"
	}

	_, notifType, _ := strings.Cut(string(notification.NotificationType), ".")
	if notifType == "" {
		notifType = unknownNotificationType
	}

	subject := fmt.Sprintf("%s.%s.%s.%s.%s.%s",
		PrefixNotificationsUser,
		notification.OwnerID,
		notification.Source,
		notifType,
		contract,
		notification.StepName,
	)

	// Explicitly map to map[string]interface{} to maintain camelCase JSON keys
	// while keeping the domain struct pure (no JSON tags).
	data, err := json.Marshal(map[string]interface{}{
		"notificationId":   notification.NotificationID,
		"threadId":         notification.ThreadID,
		"stepId":           notification.StepID,
		"stepName":         notification.StepName,
		"ownerId":          notification.OwnerID,
		"contractName":     notification.ContractName,
		"source":           string(notification.Source),
		"notificationType": string(notification.NotificationType),
		"stepStatus":       notification.StepStatus,
		"status":           notification.Status,
		"violationType":    notification.ViolationType,
		"severity":         notification.Severity,
		"message":          notification.Message,
		"details":          notification.Details,
		"timestamp":        notification.Timestamp,
	})
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	// Use PublishAsync with timeout to prevent indefinite hangs
	p.client.logger.Debug("starting NATS publish", zap.String("subject", subject))

	js := p.client.JetStream()
	pubAck, err := js.PublishAsync(subject, data)
	if err != nil {
		p.client.logger.Error("PublishAsync failed immediately", zap.Error(err), zap.String("subject", subject))
		return fmt.Errorf("publish to NATS: %w", err)
	}

	p.client.logger.Debug("waiting for publish acknowledgment", zap.String("subject", subject))

	// Wait for acknowledgment with timeout from context
	select {
	case <-pubAck.Ok():
		p.client.logger.Debug("published notification", zap.String("subject", subject))
		return nil
	case err := <-pubAck.Err():
		p.client.logger.Error("publish acknowledgment error", zap.Error(err), zap.String("subject", subject))
		return fmt.Errorf("publish to NATS: %w", err)
	case <-ctx.Done():
		p.client.logger.Error("publish context timeout", zap.Error(ctx.Err()), zap.String("subject", subject))
		return fmt.Errorf("publish to NATS: %w", ctx.Err())
	}
}
