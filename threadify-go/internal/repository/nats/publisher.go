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

// PublishNotification publishes a notification to NATS with enhanced subject for client-side filtering
func (p *Publisher) PublishNotification(ctx context.Context, notification models.ValidationNotification) error {
	// Determine contract or use "global"
	contract := "global"
	if notification.ContractName != "" {
		contract = notification.ContractName
	}

	// Extract notification type from NotificationType field
	// Format: "execution.success" → type = "success"
	//         "validation.violated" → type = "violated"
	notifType := "unknown"
	if notification.NotificationType != "" {
		// Split on dot and take second part
		parts := splitNotificationType(notification.NotificationType)
		if len(parts) > 1 {
			notifType = parts[1]
		}
	}

	// Build subject: notifications.user.{ownerID}.{source}.{type}.{contract}.{stepName}
	// Examples:
	// - notifications.user.123.execution.success.product_delivery.order_placed
	// - notifications.user.123.validation.violated.product_delivery.order_placed
	subject := fmt.Sprintf("notifications.user.%s.%s.%s.%s.%s",
		notification.OwnerID,
		notification.Source, // execution | validation | thread
		notifType,           // success | failed | passed | violated
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

	fmt.Printf("[NATS-PUBLISH] Published to %s\n", subject)

	return nil
}

// splitNotificationType splits "execution.success" into ["execution", "success"]
func splitNotificationType(notifType string) []string {
	parts := make([]string, 0, 2)
	dotIndex := -1
	for i, c := range notifType {
		if c == '.' {
			dotIndex = i
			break
		}
	}

	if dotIndex > 0 {
		parts = append(parts, notifType[:dotIndex])
		parts = append(parts, notifType[dotIndex+1:])
	} else {
		parts = append(parts, notifType)
	}

	return parts
}
