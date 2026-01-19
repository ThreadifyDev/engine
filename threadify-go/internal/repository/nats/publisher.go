package nats

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/threadify/engine/internal/models"
)

// Publisher handles publishing notifications to NATS
type Publisher struct {
	client *Client
}

// NewPublisher creates a new notification publisher
func NewPublisher(client *Client) *Publisher {
	return &Publisher{
		client: client,
	}
}

// PublishNotification publishes a notification to NATS with contract-aware subject
func (p *Publisher) PublishNotification(ctx context.Context, notification models.ValidationNotification) error {
	// Determine contract or use "global"
	contract := "global"
	if notification.ContractName != "" {
		contract = notification.ContractName
	}

	// Build subject: notifications.user.{ownerID}.{contract}.{stepName}
	subject := fmt.Sprintf("notifications.user.%s.%s.%s",
		notification.OwnerID,
		contract,
		notification.StepName)

	// Marshal notification
	data, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Publish to NATS
	_, err = p.client.JetStream().Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish to NATS: %w", err)
	}

	fmt.Printf("[NATS-PUBLISH] Published to %s (status=%s, severity=%s)\n",
		subject, notification.Status, notification.Severity)

	return nil
}
