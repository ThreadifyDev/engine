package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"go.uber.org/zap"
)

const (
	TimeoutStreamName     = "TRANSITION_TIMEOUTS"
	TimeoutSubjectPattern = "timeout.>"
	TimeoutKVBucket       = "timeout_cancellations"
)

// TimeoutMonitor handles proactive timeout monitoring using NATS JetStream
type TimeoutMonitor struct {
	nc              *nats.Conn
	js              jetstream.JetStream
	kv              jetstream.KeyValue
	threadRepo      interfaces.ThreadRepository
	notificationPub NotificationPublisher
	logger          *zap.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	// Metrics
	scheduledCount atomic.Uint64
	cancelledCount atomic.Uint64
	firedCount     atomic.Uint64
	violationCount atomic.Uint64
}

// NewTimeoutMonitor creates a new timeout monitor service
func NewTimeoutMonitor(
	nc *nats.Conn,
	threadRepo interfaces.ThreadRepository,
	notificationPub NotificationPublisher,
	logger *zap.Logger,
) (*TimeoutMonitor, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tm := &TimeoutMonitor{
		nc:              nc,
		js:              js,
		threadRepo:      threadRepo,
		notificationPub: notificationPub,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Initialize stream and KV bucket
	if err := tm.initializeStream(); err != nil {
		cancel()
		return nil, fmt.Errorf("initialize timeout stream: %w", err)
	}

	if err := tm.initializeKVBucket(); err != nil {
		cancel()
		return nil, fmt.Errorf("initialize KV bucket: %w", err)
	}

	return tm, nil
}

// initializeStream creates the timeout events stream
func (tm *TimeoutMonitor) initializeStream() error {
	_, err := tm.js.CreateStream(tm.ctx, jetstream.StreamConfig{
		Name:        TimeoutStreamName,
		Description: "Proactive timeout monitoring for transitions and thread max duration",
		Subjects:    []string{TimeoutSubjectPattern},
		Retention:   jetstream.WorkQueuePolicy,
		MaxAge:      7 * 24 * time.Hour, // 7 days retention
		Storage:     jetstream.FileStorage,
		Replicas:    1,
		Discard:     jetstream.DiscardOld,
		// Enable delayed message scheduling (NATS 2.12+)
		AllowMsgSchedules: true,
	})

	if err != nil {
		// Stream might already exist, try to update
		_, err = tm.js.UpdateStream(tm.ctx, jetstream.StreamConfig{
			Name:        TimeoutStreamName,
			Description: "Proactive timeout monitoring for transitions and thread max duration",
			Subjects:    []string{TimeoutSubjectPattern},
			Retention:   jetstream.WorkQueuePolicy,
			MaxAge:      7 * 24 * time.Hour,
			Storage:     jetstream.FileStorage,
			Replicas:    1,
			Discard:     jetstream.DiscardOld,
			// Enable delayed message scheduling (NATS 2.12+)
			AllowMsgSchedules: true,
		})
		if err != nil {
			return fmt.Errorf("create or update stream: %w", err)
		}
	}

	tm.logger.Info("initialized timeout stream", zap.String("stream", TimeoutStreamName))
	return nil
}

// initializeKVBucket creates the KV bucket for cancellation flags
func (tm *TimeoutMonitor) initializeKVBucket() error {
	kv, err := tm.js.CreateKeyValue(tm.ctx, jetstream.KeyValueConfig{
		Bucket:      TimeoutKVBucket,
		Description: "Cancellation flags for timeout events",
		TTL:         7 * 24 * time.Hour, // Auto-cleanup after 7 days
		Storage:     jetstream.FileStorage,
		Replicas:    1,
	})

	if err != nil {
		// Bucket might already exist
		kv, err = tm.js.KeyValue(tm.ctx, TimeoutKVBucket)
		if err != nil {
			return fmt.Errorf("create or get KV bucket: %w", err)
		}
	}

	tm.kv = kv
	tm.logger.Info("initialized timeout KV bucket", zap.String("bucket", TimeoutKVBucket))
	return nil
}

// Start begins consuming timeout events
func (tm *TimeoutMonitor) Start() error {
	consumer, err := tm.js.CreateOrUpdateConsumer(tm.ctx, TimeoutStreamName, jetstream.ConsumerConfig{
		Name:          "timeout-monitor",
		Durable:       "timeout-monitor",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		AckWait:       30 * time.Second,
		FilterSubject: TimeoutSubjectPattern,
		DeliverPolicy: jetstream.DeliverAllPolicy,
	})

	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	// Start consuming messages
	consumeCtx, err := consumer.Consume(func(msg jetstream.Msg) {
		if err := tm.handleTimeoutEvent(msg); err != nil {
			tm.logger.Error("failed to handle timeout event",
				zap.Error(err),
				zap.String("subject", msg.Subject()),
			)
		}
	})

	if err != nil {
		return fmt.Errorf("start consuming: %w", err)
	}

	tm.logger.Info("timeout monitor started")

	// Wait for context cancellation
	<-tm.ctx.Done()
	consumeCtx.Stop()

	return nil
}

// handleTimeoutEvent processes a timeout event
func (tm *TimeoutMonitor) handleTimeoutEvent(msg jetstream.Msg) error {
	var event models.TimeoutEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		msg.Ack()
		return fmt.Errorf("unmarshal timeout event: %w", err)
	}

	// Check if timeout was cancelled
	cancelled, err := tm.isTimeoutCancelled(event.ID)
	if err != nil {
		tm.logger.Error("failed to check cancellation",
			zap.Error(err),
			zap.String("timeout_id", event.ID),
		)
		// Don't ack on error - will retry
		return err
	}

	if cancelled {
		tm.logger.Debug("timeout cancelled, skipping",
			zap.String("timeout_id", event.ID),
			zap.String("thread_id", event.ThreadID),
			zap.String("type", string(event.Type)),
		)
		msg.Ack()
		return nil
	}

	// Timeout fired - increment metric
	tm.firedCount.Add(1)

	// Note: No need to check if step has started in Valkey/PostgreSQL
	// Both scheduling and cancellation happen in the same worker pool context,
	// so they experience the same delays. The cancellation flag check above
	// is sufficient to prevent false positives.

	// Build thread object from timeout event metadata
	// All necessary information (owner_id, contract_name) is stored in the event
	thread := &models.Thread{
		ID:           event.ThreadID,
		ContractName: event.ContractName,
		OwnerID:      "",
	}
	if ownerID, ok := event.Metadata["owner_id"].(string); ok {
		thread.OwnerID = ownerID
	}

	violation := tm.buildViolationNotification(event, thread)

	// Use a timeout context for publishing to prevent indefinite hangs
	publishCtx, cancel := context.WithTimeout(tm.ctx, 5*time.Second)
	defer cancel()

	tm.logger.Info("attempting to publish timeout violation",
		zap.String("timeout_id", event.ID),
		zap.String("thread_id", event.ThreadID),
		zap.String("owner_id", thread.OwnerID),
		zap.String("contract", event.ContractName),
	)

	if err := tm.notificationPub.PublishNotification(publishCtx, violation); err != nil {
		tm.logger.Error("failed to publish timeout violation",
			zap.Error(err),
			zap.String("timeout_id", event.ID),
			zap.String("owner_id", thread.OwnerID),
		)
		return err
	}

	tm.logger.Info("successfully published timeout violation",
		zap.String("timeout_id", event.ID),
		zap.String("owner_id", thread.OwnerID),
	)
	tm.violationCount.Add(1)

	tm.logger.Info("timeout violation fired",
		zap.String("timeout_id", event.ID),
		zap.String("thread_id", event.ThreadID),
		zap.String("from_step", event.FromStep),
		zap.String("expected_steps", event.ToStep),
		zap.String("timeout", event.Timeout),
	)

	msg.Ack()
	return nil
}

// isTimeoutCancelled checks if a timeout has been cancelled
func (tm *TimeoutMonitor) isTimeoutCancelled(timeoutID string) (bool, error) {
	// NATS KV keys cannot contain colons, replace with underscores
	sanitizedID := strings.ReplaceAll(timeoutID, ":", "_")
	key := fmt.Sprintf("cancelled_%s", sanitizedID)
	_, err := tm.kv.Get(tm.ctx, key)

	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return false, nil
		}
		return false, fmt.Errorf("get cancellation flag: %w", err)
	}

	return true, nil
}

// ScheduleTimeout publishes a timeout event with delayed delivery (not integrated yet)
func (tm *TimeoutMonitor) ScheduleTimeout(event models.TimeoutEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal timeout event: %w", err)
	}

	subject := fmt.Sprintf("timeout.%s.%s", event.Type, event.ThreadID)

	// Calculate delay until deadline
	delay := time.Until(event.DeadlineAt)
	if delay < 0 {
		delay = 0
	}

	// Use NATS scheduled message headers for delayed delivery (NATS 2.12+)
	// The message will be held by JetStream until the scheduled time
	_, err = tm.js.PublishMsg(tm.ctx, &nats.Msg{
		Subject: subject,
		Data:    data,
		Header: nats.Header{
			"Nats-Msg-Id":       []string{event.ID},                              // Deduplication
			"Nats-Msg-Schedule": []string{event.DeadlineAt.Format(time.RFC3339)}, // Scheduled delivery
		},
	})

	if err != nil {
		return fmt.Errorf("publish timeout event: %w", err)
	}

	tm.scheduledCount.Add(1)

	tm.logger.Debug("scheduled timeout",
		zap.String("timeout_id", event.ID),
		zap.String("thread_id", event.ThreadID),
		zap.String("type", string(event.Type)),
		zap.Duration("delay", delay),
		zap.Time("deadline", event.DeadlineAt),
	)

	return nil
}

// CancelTimeout writes a cancellation flag to KV store
func (tm *TimeoutMonitor) CancelTimeout(timeoutID, threadID, reason string) error {
	cancellation := models.TimeoutCancellation{
		TimeoutID:   timeoutID,
		ThreadID:    threadID,
		CancelledAt: time.Now(),
		Reason:      reason,
	}

	data, err := json.Marshal(cancellation)
	if err != nil {
		return fmt.Errorf("marshal cancellation: %w", err)
	}

	// NATS KV keys cannot contain colons, replace with underscores
	sanitizedID := strings.ReplaceAll(timeoutID, ":", "_")
	key := fmt.Sprintf("cancelled_%s", sanitizedID)
	_, err = tm.kv.Put(tm.ctx, key, data)
	if err != nil {
		return fmt.Errorf("write cancellation flag: %w", err)
	}

	tm.cancelledCount.Add(1)

	tm.logger.Debug("cancelled timeout",
		zap.String("timeout_id", timeoutID),
		zap.String("thread_id", threadID),
		zap.String("reason", reason),
	)

	return nil
}

// buildViolationNotification creates a validation notification for a timeout violation
func (tm *TimeoutMonitor) buildViolationNotification(event models.TimeoutEvent, thread *models.Thread) models.ValidationNotification {
	var message string
	var stepName string

	switch event.Type {
	case models.TimeoutTypeMaxDuration:
		// For max_duration timeouts, use "global" as a placeholder step name
		// since these are thread-level timeouts, not step-specific
		message = fmt.Sprintf(
			"Thread exceeded max_duration of %s",
			event.Timeout,
		)
		stepName = "global"

	case models.TimeoutTypeTransition:
		// Transition timeout between steps
		expectedStepsDisplay := event.ToStep
		if strings.Contains(event.ToStep, ",") {
			expectedStepsDisplay = fmt.Sprintf("[%s]", event.ToStep)
		}
		message = fmt.Sprintf(
			"Transition timeout: Expected one of %s to start within %s after '%s' completed",
			expectedStepsDisplay,
			event.Timeout,
			event.FromStep,
		)
		stepName = event.FromStep

	default:
		// Fallback for unknown timeout types
		message = fmt.Sprintf("Timeout violation: %s", event.Type)
		stepName = event.FromStep
	}

	return models.ValidationNotification{
		NotificationID:   uuid.New().String(),
		ThreadID:         event.ThreadID,
		StepID:           "", // No specific step ID for timeouts
		StepName:         stepName,
		OwnerID:          thread.OwnerID,
		ContractName:     event.ContractName,
		Status:           "violated",
		Severity:         "critical",
		ViolationType:    "timeout",
		Message:          message,
		Timestamp:        time.Now(),
		Source:           models.NotificationSourceValidation,
		NotificationType: "validation.violated.timeout",
		Details: map[string]interface{}{
			"timeout_id":     event.ID,
			"timeout_type":   event.Type,
			"from_step":      event.FromStep,
			"expected_steps": event.ToStep,
			"timeout":        event.Timeout,
			"deadline":       event.DeadlineAt.Format(time.RFC3339),
			"scheduled_at":   event.ScheduledAt.Format(time.RFC3339),
			"contract_name":  event.ContractName,
			"metadata":       event.Metadata,
		},
	}
}

// GetMetrics returns current timeout monitoring metrics
func (tm *TimeoutMonitor) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"scheduled":  tm.scheduledCount.Load(),
		"cancelled":  tm.cancelledCount.Load(),
		"fired":      tm.firedCount.Load(),
		"violations": tm.violationCount.Load(),
	}
}

// Stop gracefully shuts down the timeout monitor
func (tm *TimeoutMonitor) Stop() {
	tm.logger.Info("stopping timeout monitor")
	tm.cancel()
}
