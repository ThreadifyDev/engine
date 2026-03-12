package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

// ArchivalPublisher handles publishing archival events to NATS JetStream
type ArchivalPublisher struct {
	client *Client
	logger *zap.Logger
}

// NewArchivalPublisher creates a new archival event publisher
func NewArchivalPublisher(client *Client, logger *zap.Logger) *ArchivalPublisher {
	return &ArchivalPublisher{
		client: client,
		logger: logger,
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

// PublishThreadNotifications publishes individual thread notifications for archival
func (p *ArchivalPublisher) PublishThreadNotifications(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "notifications.thread", event)
}

// PublishStepState publishes step state snapshot for archival
func (p *ArchivalPublisher) PublishStepState(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "state.step", event)
}

// PublishUsageSyncBatch publishes a batch of usage sync events as a single NATS message
func (p *ArchivalPublisher) PublishUsageSyncBatch(ctx context.Context, events []map[string]interface{}) error {
	if len(events) == 0 {
		return nil
	}

	// Add timestamps to each event if missing
	now := time.Now().Format(time.RFC3339)
	for _, event := range events {
		if _, exists := event["timestamp"]; !exists {
			event["timestamp"] = now
		}
	}

	return p.publishData(ctx, "usage.sync", events)
}

// publish is the internal method that handles the actual NATS publish for a single event
func (p *ArchivalPublisher) publish(ctx context.Context, subject string, event map[string]interface{}) error {
	// Add timestamp if not present
	if _, exists := event["timestamp"]; !exists {
		event["timestamp"] = time.Now().Format(time.RFC3339)
	}
	return p.publishData(ctx, subject, event)
}

// publishData marshals and publishes arbitrary data to NATS JetStream
func (p *ArchivalPublisher) publishData(ctx context.Context, subject string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = p.client.JetStream().Publish(subject, jsonData, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("failed to publish to NATS subject %s: %w", subject, err)
	}

	return nil
}

// PublishAsync publishes event asynchronously (fire-and-forget with error logging).
// Intentional: uses detached context — this goroutine outlives any request lifecycle.
func (p *ArchivalPublisher) PublishAsync(subject string, event map[string]interface{}) {
	if subject == "" {
		p.logger.Warn("skipped async NATS message - empty subject")
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.publish(ctx, subject, event); err != nil {
			p.logger.Error("failed to publish async NATS message",
				zap.String("subject", subject),
				zap.String("company_id", fmt.Sprintf("%v", event["company_id"])),
				zap.Error(err),
			)
		} else {
			p.logger.Debug("published async NATS message", zap.String("subject", subject))
		}
	}()
}
