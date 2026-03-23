package service

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/models"
)

// Example of how to schedule a transition timeout (not integrated yet)
func ExampleScheduleTransitionTimeout() {
	// This would be called when a step completes successfully
	timeoutEvent := models.TimeoutEvent{
		ID:           uuid.New().String(),
		ThreadID:     "thread-123",
		Type:         models.TimeoutTypeTransition,
		FromStep:     "order_placed",
		ToStep:       "payment_validation",
		Timeout:      "2m",
		ScheduledAt:  time.Now(),
		DeadlineAt:   time.Now().Add(2 * time.Minute), // startedAt + timeout
		ContractName: "product_delivery",
		Metadata: map[string]interface{}{
			"step_id": "step-456",
		},
	}

	// monitor.ScheduleTimeout(timeoutEvent)
	_ = timeoutEvent
}

// Example of how to cancel a timeout (not integrated yet)
func ExampleCancelTimeout() {
	// This would be called when the next step starts
	timeoutID := "timeout-123"
	threadID := "thread-123"
	reason := "next_step_started"

	// monitor.CancelTimeout(timeoutID, threadID, reason)
	_, _, _ = timeoutID, threadID, reason
}

// Example of how to schedule a thread max duration timeout (not integrated yet)
func ExampleScheduleMaxDurationTimeout() {
	// This would be called when thread starts
	timeoutEvent := models.TimeoutEvent{
		ID:           uuid.New().String(),
		ThreadID:     "thread-123",
		Type:         models.TimeoutTypeMaxDuration,
		Timeout:      "72h",
		ScheduledAt:  time.Now(),
		DeadlineAt:   time.Now().Add(72 * time.Hour), // thread.startedAt + max_duration
		ContractName: "product_delivery",
	}

	// monitor.ScheduleTimeout(timeoutEvent)
	_ = timeoutEvent
}

// TestTimeoutEventStructure validates the timeout event structure
func TestTimeoutEventStructure(t *testing.T) {
	event := models.TimeoutEvent{
		ID:          "timeout-123",
		ThreadID:    "thread-456",
		Type:        models.TimeoutTypeTransition,
		FromStep:    "step_a",
		ToStep:      "step_b",
		Timeout:     "5m",
		ScheduledAt: time.Now(),
		DeadlineAt:  time.Now().Add(5 * time.Minute),
	}

	if event.ID == "" {
		t.Error("timeout ID should not be empty")
	}

	if event.Type != models.TimeoutTypeTransition {
		t.Errorf("expected type %s, got %s", models.TimeoutTypeTransition, event.Type)
	}
}

// TestTimeoutCancellationStructure validates the cancellation structure
func TestTimeoutCancellationStructure(t *testing.T) {
	cancellation := models.TimeoutCancellation{
		TimeoutID:   "timeout-123",
		ThreadID:    "thread-456",
		CancelledAt: time.Now(),
		Reason:      "next_step_started",
	}

	if cancellation.TimeoutID == "" {
		t.Error("timeout ID should not be empty")
	}

	if cancellation.Reason == "" {
		t.Error("cancellation reason should not be empty")
	}
}
