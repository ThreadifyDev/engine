package service

import (
	"context"
	"fmt"
	"time"

	"github.com/threadify/engine/internal/domain"
	"go.uber.org/zap"
)

// scheduleThreadMaxDurationTimeout schedules a thread-level max duration timeout
func (s *NotificationService) scheduleThreadMaxDurationTimeout(
	ctx context.Context,
	threadID string,
	graph *domain.ContractGraph,
	thread *domain.Thread,
	createdAt time.Time,
) {
	// Check if timeout monitor is available
	if s.timeoutMonitor == nil {
		s.logger.Debug("timeout monitor not available, skipping thread max_duration timeout",
			zap.String("thread_id", threadID),
		)
		return
	}

	// Check if contract has max_duration validation
	if graph.Validation == nil {
		s.logger.Debug("contract graph has no validation section",
			zap.String("thread_id", threadID),
			zap.String("contract", thread.ContractName),
		)
		return
	}
	if graph.Validation.MaxDuration == "" {
		s.logger.Debug("contract validation has no max_duration",
			zap.String("thread_id", threadID),
			zap.String("contract", thread.ContractName),
		)
		return
	}

	// Parse max duration
	maxDuration, err := time.ParseDuration(graph.Validation.MaxDuration)
	if err != nil {
		s.logger.Warn("invalid max_duration format in contract",
			zap.String("thread_id", threadID),
			zap.String("contract", thread.ContractName),
			zap.String("max_duration", graph.Validation.MaxDuration),
			zap.Error(err),
		)
		return
	}

	// Calculate deadline: thread creation time + max_duration
	deadline := createdAt.Add(maxDuration)

	// Generate timeout ID
	timeoutID := fmt.Sprintf("%s:max_duration", threadID)

	// Create timeout event
	timeoutEvent := domain.TimeoutEvent{
		ID:           timeoutID,
		ThreadID:     threadID,
		Type:         domain.TimeoutTypeMaxDuration,
		Timeout:      graph.Validation.MaxDuration,
		ScheduledAt:  time.Now(),
		DeadlineAt:   deadline,
		ContractName: thread.ContractName,
		Metadata: map[string]interface{}{
			"owner_id":   thread.OwnerID,
			"created_at": createdAt.Format(time.RFC3339),
		},
	}

	// Schedule the timeout
	if err := s.timeoutMonitor.ScheduleTimeout(ctx, timeoutEvent); err != nil {
		s.logger.Error("failed to schedule thread max_duration timeout",
			zap.String("thread_id", threadID),
			zap.String("contract", thread.ContractName),
			zap.Error(err),
		)
	} else {
		s.logger.Info("scheduled thread max_duration timeout",
			zap.String("thread_id", threadID),
			zap.String("contract", thread.ContractName),
			zap.Duration("max_duration", maxDuration),
			zap.Time("deadline", deadline),
		)
	}
}

// cancelThreadMaxDurationTimeout cancels a thread-level max duration timeout
func (s *NotificationService) cancelThreadMaxDurationTimeout(
	ctx context.Context,
	threadID string,
) {
	if s.timeoutMonitor == nil {
		return
	}

	timeoutID := fmt.Sprintf("%s:max_duration", threadID)

	if err := s.timeoutMonitor.CancelTimeout(ctx, timeoutID, threadID, "thread completed"); err != nil {
		s.logger.Warn("failed to cancel thread max_duration timeout",
			zap.String("thread_id", threadID),
			zap.String("timeout_id", timeoutID),
			zap.Error(err),
		)
	} else {
		s.logger.Debug("cancelled thread max_duration timeout",
			zap.String("thread_id", threadID),
			zap.String("timeout_id", timeoutID),
		)
	}
}
