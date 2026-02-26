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

	if _, err = p.client.JetStream().Publish(subject, data); err != nil {
		return fmt.Errorf("publish to NATS: %w", err)
	}

	p.client.logger.Debug("published notification", zap.String("subject", subject))
	return nil
}
