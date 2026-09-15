package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

const (
	TimeoutStreamName     = "TRANSITION_TIMEOUTS"
	TimeoutSubjectPattern = "timeout.>"
	TimeoutKVBucket       = "timeout_cancellations"
)

type TimeoutMonitor struct {
	nc              *nats.Conn
	js              jetstream.JetStream
	kv              timeoutKV
	notificationPub domain.NotificationPublisher
	logger          *zap.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	scheduledCount  atomic.Uint64
	cancelledCount  atomic.Uint64
	firedCount      atomic.Uint64
	violationCount  atomic.Uint64
	lifecycleMu     sync.Mutex
	started         bool
	stopping        bool
	work            sync.WaitGroup
	stopDone        chan struct{}
}

type timeoutKV interface {
	Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error)
	Put(ctx context.Context, key string, value []byte) (uint64, error)
}

type TimeoutKV = timeoutKV

func TimeoutCancellationKey(timeoutID string) string {
	sanitizedID := strings.ReplaceAll(timeoutID, ":", "_")
	return fmt.Sprintf("cancelled_%s", sanitizedID)
}

func NewTimeoutMonitorForTests(kv TimeoutKV, notificationPub domain.NotificationPublisher, logger *zap.Logger) *TimeoutMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &TimeoutMonitor{
		kv:              kv,
		notificationPub: notificationPub,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}
}

func NewTimeoutMonitor(
	nc *nats.Conn,
	notificationPub domain.NotificationPublisher,
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
		notificationPub: notificationPub,
		logger:          logger,
		ctx:             ctx,
		cancel:          cancel,
	}

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

func (tm *TimeoutMonitor) initializeStream() error {
	cfg := jetstream.StreamConfig{
		Name:              TimeoutStreamName,
		Description:       "Proactive timeout monitoring for transitions and thread max duration",
		Subjects:          []string{TimeoutSubjectPattern},
		Retention:         jetstream.WorkQueuePolicy,
		MaxAge:            7 * 24 * time.Hour,
		Storage:           jetstream.FileStorage,
		Replicas:          1,
		Discard:           jetstream.DiscardOld,
		AllowMsgSchedules: true, // Requires NATS 2.12+
	}

	_, err := tm.js.CreateStream(tm.ctx, cfg)
	if err != nil {
		_, err = tm.js.UpdateStream(tm.ctx, cfg)
		if err != nil {
			return fmt.Errorf("create or update stream: %w", err)
		}
	}

	tm.logger.Info("initialized timeout stream", zap.String("stream", TimeoutStreamName))
	return nil
}

func (tm *TimeoutMonitor) initializeKVBucket() error {
	kv, err := tm.js.CreateKeyValue(tm.ctx, jetstream.KeyValueConfig{
		Bucket:      TimeoutKVBucket,
		Description: "Cancellation flags for timeout events",
		TTL:         7 * 24 * time.Hour,
		Storage:     jetstream.FileStorage,
		Replicas:    1,
	})

	if err != nil {
		kv, err = tm.js.KeyValue(tm.ctx, TimeoutKVBucket)
		if err != nil {
			return fmt.Errorf("create or get KV bucket: %w", err)
		}
	}

	tm.kv = kv
	tm.logger.Info("initialized timeout KV bucket", zap.String("bucket", TimeoutKVBucket))
	return nil
}

// Start begins consuming timeout events. It blocks until Stop() is called.
// Run this in a dedicated goroutine:
//
//	go func() {
//	    if err := tm.Start(); err != nil {
//	        log.Fatal(err)
//	    }
//	}()
func (tm *TimeoutMonitor) Start() error {
	tm.lifecycleMu.Lock()
	if tm.stopping || tm.started {
		tm.lifecycleMu.Unlock()
		return fmt.Errorf("timeout monitor is already started or stopped")
	}
	tm.started = true
	tm.work.Add(1)
	tm.lifecycleMu.Unlock()
	defer tm.work.Done()

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

	<-tm.ctx.Done()
	consumeCtx.Stop()
	// Stop only requests cancellation; Closed also joins an in-flight callback.
	<-consumeCtx.Closed()

	return nil
}

// HandleTimeoutEvent is an exported wrapper around handleTimeoutEvent (primarily for tests).
func (tm *TimeoutMonitor) HandleTimeoutEvent(msg jetstream.Msg) error {
	if err := tm.beginOperation(); err != nil {
		return err
	}
	defer tm.work.Done()
	return tm.handleTimeoutEvent(msg)
}

// timeoutEventModel is the wire representation of a timeout event.
// It is used for both publishing (ScheduleTimeout) and consuming (handleTimeoutEvent)
// to keep serialization in sync.
type timeoutEventModel struct {
	ID           string                 `json:"id"`
	ThreadID     string                 `json:"threadId"`
	Type         string                 `json:"type"`
	FromStep     string                 `json:"fromStep"`
	ToStep       string                 `json:"toStep"`
	Timeout      string                 `json:"timeout"`
	DeadlineAt   time.Time              `json:"deadlineAt"`
	ScheduledAt  time.Time              `json:"scheduledAt"`
	ContractName string                 `json:"contractName"`
	OwnerID      string                 `json:"ownerId"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

func domainEventToModel(event domain.TimeoutEvent) timeoutEventModel {
	ownerID := ""
	if event.Metadata != nil {
		if v, ok := event.Metadata["owner_id"].(string); ok {
			ownerID = v
		}
	}
	return timeoutEventModel{
		ID:           event.ID,
		ThreadID:     event.ThreadID,
		Type:         string(event.Type),
		FromStep:     event.FromStep,
		ToStep:       event.ToStep,
		Timeout:      event.Timeout,
		DeadlineAt:   event.DeadlineAt,
		ScheduledAt:  event.ScheduledAt,
		ContractName: event.ContractName,
		OwnerID:      ownerID,
		Metadata:     event.Metadata,
	}
}

func (tm *TimeoutMonitor) handleTimeoutEvent(msg jetstream.Msg) error {
	var model timeoutEventModel
	if err := json.Unmarshal(msg.Data(), &model); err != nil {
		// Poison pill — ack to discard, no point retrying.
		msg.Ack()
		return fmt.Errorf("unmarshal timeout event: %w", err)
	}

	cancelled, err := tm.isTimeoutCancelled(tm.ctx, model.ID)
	if err != nil {
		tm.logger.Error("failed to check cancellation",
			zap.Error(err),
			zap.String("timeout_id", model.ID),
		)
		// Do not ack — allow retry up to MaxDeliver.
		return err
	}

	if cancelled {
		tm.logger.Debug("timeout cancelled, skipping",
			zap.String("timeout_id", model.ID),
			zap.String("thread_id", model.ThreadID),
			zap.String("type", model.Type),
		)
		msg.Ack()
		return nil
	}

	// Older NATS servers may accept the scheduled-message header while still
	// delivering the message immediately. Keep the consumer authoritative for
	// the deadline so a future timeout can never become a false violation.
	if delay := time.Until(model.DeadlineAt); delay > 0 {
		if err := msg.NakWithDelay(delay); err != nil {
			return fmt.Errorf("defer timeout until deadline: %w", err)
		}
		tm.logger.Debug("deferred early timeout delivery",
			zap.String("timeout_id", model.ID),
			zap.String("thread_id", model.ThreadID),
			zap.Duration("delay", delay),
			zap.Time("deadline", model.DeadlineAt),
		)
		return nil
	}

	tm.firedCount.Add(1)

	thread := &domain.Thread{
		ID:           model.ThreadID,
		ContractName: model.ContractName,
		OwnerID:      model.OwnerID,
	}

	event := domain.TimeoutEvent{
		ID:           model.ID,
		ThreadID:     model.ThreadID,
		Type:         domain.TimeoutType(model.Type),
		FromStep:     model.FromStep,
		ToStep:       model.ToStep,
		Timeout:      model.Timeout,
		DeadlineAt:   model.DeadlineAt,
		ScheduledAt:  model.ScheduledAt,
		ContractName: model.ContractName,
		Metadata:     model.Metadata,
	}

	violation := BuildTimeoutViolationNotification(uuid.New().String(), event, thread)

	publishCtx, cancel := context.WithTimeout(tm.ctx, 5*time.Second)
	defer cancel()

	tm.logger.Info("publishing timeout violation",
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
		// Do not ack — allow retry up to MaxDeliver.
		return err
	}

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

func (tm *TimeoutMonitor) isTimeoutCancelled(ctx context.Context, timeoutID string) (bool, error) {
	_, err := tm.kv.Get(ctx, TimeoutCancellationKey(timeoutID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("get cancellation flag: %w", err)
	}
	return true, nil
}

// IsTimeoutCancelled is an exported wrapper around isTimeoutCancelled (primarily for tests).
func (tm *TimeoutMonitor) IsTimeoutCancelled(ctx context.Context, timeoutID string) (bool, error) {
	return tm.isTimeoutCancelled(ctx, timeoutID)
}

// ScheduleTimeout publishes a timeout event with delayed delivery via NATS scheduled messages.
func (tm *TimeoutMonitor) ScheduleTimeout(ctx context.Context, event domain.TimeoutEvent) error {
	if err := tm.beginOperation(); err != nil {
		return err
	}
	defer tm.work.Done()
	model := domainEventToModel(event)

	data, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("marshal timeout event: %w", err)
	}

	subject := fmt.Sprintf("timeout.%s.%s", event.Type, event.ThreadID)

	_, err = tm.js.PublishMsg(ctx, &nats.Msg{
		Subject: subject,
		Data:    data,
		Header: nats.Header{
			"Nats-Msg-Id":       []string{event.ID},
			"Nats-Msg-Schedule": []string{event.DeadlineAt.Format(time.RFC3339)},
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
		zap.Duration("delay", time.Until(event.DeadlineAt)),
		zap.Time("deadline", event.DeadlineAt),
	)

	return nil
}

// CancelTimeout writes a cancellation flag to the KV store.
func (tm *TimeoutMonitor) CancelTimeout(ctx context.Context, timeoutID, threadID, reason string) error {
	if err := tm.beginOperation(); err != nil {
		return err
	}
	defer tm.work.Done()
	data, err := json.Marshal(map[string]interface{}{
		"timeoutId":   timeoutID,
		"threadId":    threadID,
		"cancelledAt": time.Now(),
		"reason":      reason,
	})
	if err != nil {
		return fmt.Errorf("marshal cancellation: %w", err)
	}

	_, err = tm.kv.Put(ctx, TimeoutCancellationKey(timeoutID), data)
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

// BuildTimeoutViolationNotification creates a violation notification for a timeout event.
// notificationID is accepted as a parameter to keep the function deterministic and testable.
func BuildTimeoutViolationNotification(notificationID string, event domain.TimeoutEvent, thread *domain.Thread) domain.ValidationNotification {
	var message, stepName string

	switch event.Type {
	case domain.TimeoutTypeMaxDuration:
		message = fmt.Sprintf("Thread exceeded max_duration of %s", event.Timeout)
		stepName = "global"

	case domain.TimeoutTypeTransition:
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
		message = fmt.Sprintf("Timeout violation: %s", event.Type)
		stepName = event.FromStep
	}

	return domain.ValidationNotification{
		NotificationID:   notificationID,
		ThreadID:         event.ThreadID,
		StepID:           "",
		StepName:         stepName,
		OwnerID:          thread.OwnerID,
		ContractName:     event.ContractName,
		Status:           "violated",
		Severity:         "critical",
		ViolationType:    "timeout",
		Message:          message,
		Timestamp:        time.Now(),
		Source:           domain.NotificationSourceRule,
		NotificationType: domain.NotificationTypeRuleViolatedTimeout,
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

// GetMetrics returns current timeout monitoring metrics.
func (tm *TimeoutMonitor) GetMetrics() map[string]uint64 {
	return map[string]uint64{
		"scheduled":  tm.scheduledCount.Load(),
		"cancelled":  tm.cancelledCount.Load(),
		"fired":      tm.firedCount.Load(),
		"violations": tm.violationCount.Load(),
	}
}

// Stop gracefully shuts down the timeout monitor.
func (tm *TimeoutMonitor) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tm.StopContext(ctx); err != nil {
		tm.logger.Error("timeout monitor shutdown did not complete", zap.Error(err))
	}
}

func (tm *TimeoutMonitor) beginOperation() error {
	tm.lifecycleMu.Lock()
	defer tm.lifecycleMu.Unlock()
	if tm.stopping {
		return fmt.Errorf("timeout monitor is stopped")
	}
	tm.work.Add(1)
	return nil
}

// StopContext prevents new work, cancels consumption, and joins outstanding
// callbacks and publications before the application closes its broker.
func (tm *TimeoutMonitor) StopContext(ctx context.Context) error {
	tm.lifecycleMu.Lock()
	if !tm.stopping {
		tm.stopping = true
		tm.stopDone = make(chan struct{})
		tm.cancel()
		go func() {
			tm.work.Wait()
			close(tm.stopDone)
		}()
	}
	done := tm.stopDone
	tm.lifecycleMu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop timeout monitor: %w", ctx.Err())
	}
}
