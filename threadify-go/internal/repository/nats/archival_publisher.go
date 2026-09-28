package nats

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

type ArchivalPublisher struct {
	client *Client
	logger *zap.Logger
}

func NewArchivalPublisher(client *Client, logger *zap.Logger) *ArchivalPublisher {
	return &ArchivalPublisher{
		client: client,
		logger: logger,
	}
}

func (p *ArchivalPublisher) PublishActivityLog(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "activity.log", event)
}

func (p *ArchivalPublisher) PublishThreadMetadata(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "metadata.thread", event)
}

func (p *ArchivalPublisher) PublishThreadAccess(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "access.thread", event)
}

func (p *ArchivalPublisher) PublishThreadValidation(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "validations.thread", event)
}

func (p *ArchivalPublisher) PublishThreadNotifications(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "notifications.thread", event)
}

func (p *ArchivalPublisher) PublishStepState(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "state.step", event)
}

func (p *ArchivalPublisher) PublishCreditTopupTrigger(ctx context.Context, event map[string]interface{}) error {
	return p.publish(ctx, "credit.topup", event)
}

func (p *ArchivalPublisher) PublishUsageSyncBatch(ctx context.Context, events []map[string]interface{}) error {
	if len(events) == 0 {
		return nil
	}

	jsonData, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal usage sync batch: %w", err)
	}

	dedupID, _ := events[0]["event_id"].(string)

	if _, err := p.client.JetStream().Publish(ctx, "usage.sync", jsonData, jetstream.WithMsgID(dedupID)); err != nil {
		return fmt.Errorf("publish usage sync batch: %w", err)
	}
	return nil
}

func (p *ArchivalPublisher) publish(ctx context.Context, subject string, event map[string]interface{}) error {
	payload := event
	if _, exists := event["timestamp"]; !exists {
		payload = make(map[string]interface{}, len(event)+1)
		for k, v := range event {
			payload[k] = v
		}
		payload["timestamp"] = time.Now().Format(time.RFC3339)
	}
	return p.publishData(ctx, subject, payload)
}

func (p *ArchivalPublisher) publishData(ctx context.Context, subject string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	if _, err := p.client.JetStream().Publish(ctx, subject, jsonData); err != nil {
		return fmt.Errorf("failed to publish to NATS subject %s: %w", subject, err)
	}
	// Browser viewers receive a small change signal scoped to one thread. The
	// durable archival publish above remains the source of truth; losing a live
	// signal does not affect ingestion or persistence.
	if subject == "state.step" || subject == "metadata.thread" || subject == "validations.thread" || subject == "notifications.thread" {
		if event, ok := data.(map[string]interface{}); ok {
			if threadID, ok := event["threadId"].(string); ok && threadID != "" {
				updateData := map[string]string{"type": subject}
				if subject == "state.step" {
					updateData["stepName"], _ = event["stepName"].(string)
					updateData["idempotencyKey"], _ = event["idempotencyKey"].(string)
					updateData["actor"], _ = event["actor"].(string)
				}
				update, _ := json.Marshal(updateData)
				if err := p.client.Conn().Publish(ThreadUpdateSubject(threadID), update); err != nil {
					p.logger.Warn("failed to signal live thread update", zap.String("thread_id", threadID), zap.Error(err))
				}
			}
		}
	}
	return nil
}

// ThreadUpdateSubject avoids placing user supplied IDs directly in NATS subjects.
func ThreadUpdateSubject(threadID string) string {
	return fmt.Sprintf("live.thread.%x", sha256.Sum256([]byte(threadID)))
}

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
