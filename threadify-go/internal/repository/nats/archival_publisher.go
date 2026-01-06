package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

// ArchivalPublisher handles publishing archival events to NATS JetStream
type ArchivalPublisher struct {
	client *Client
}

// NewArchivalPublisher creates a new archival event publisher
func NewArchivalPublisher(client *Client) *ArchivalPublisher {
	return &ArchivalPublisher{
		client: client,
	}
}

// PublishActivityLog publishes an activity log event
func (p *ArchivalPublisher) PublishActivityLog(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "activity.log", event)
}

// PublishThreadMetadata publishes thread metadata event
func (p *ArchivalPublisher) PublishThreadMetadata(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "metadata.thread", event)
}

// PublishThreadAccess publishes thread access event
func (p *ArchivalPublisher) PublishThreadAccess(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "access.thread", event)
}

// PublishThreadValidation publishes thread validation event
func (p *ArchivalPublisher) PublishThreadValidation(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "validations.thread", event)
}

// publish is the internal method that handles the actual NATS publish
func (p *ArchivalPublisher) publish(ctx context.Context, subject string, event map[string]interface{}) error {
	// Add timestamp if not present
	if _, exists := event["timestamp"]; !exists {
		event["timestamp"] = time.Now().Format(time.RFC3339)
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish to NATS JetStream
	_, err = p.client.JetStream().Publish(subject, data, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("failed to publish to NATS subject %s: %w", subject, err)
	}

	fmt.Printf("✅ [NATS-ARCHIVAL] Published to %s (size: %d bytes)\n", subject, len(data))
	return nil
}

// PublishAsync publishes event asynchronously (fire-and-forget with error logging)
func (p *ArchivalPublisher) PublishAsync(subject string, event map[string]interface{}) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.publish(ctx, subject, event); err != nil {
			fmt.Printf("❌ [NATS-ARCHIVAL] Failed to publish to %s: %v\n", subject, err)
		}
	}()
}
