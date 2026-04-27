package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/threadify/engine/internal/models"
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

func (p *Publisher) PublishNotification(ctx context.Context, notification models.ValidationNotification) error {
	contract := notification.ContractName
	if contract == "" {
		contract = "global"
	}

	_, notifType, _ := strings.Cut(notification.NotificationType, ".")
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

	data, err := json.Marshal(notification)
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
