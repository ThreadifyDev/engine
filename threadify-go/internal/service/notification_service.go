package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/rbac"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/domain"
	"github.com/threadify/engine/internal/perf"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
	"github.com/threadify/engine/internal/workerpool"
	"go.uber.org/zap"
)

// NotificationService orchestrates async validation and notification archival.
// It handles non-blocking validation processing, stores validation results in streams,
// and manages the archival of validation notifications without blocking main execution.
type NotificationService struct {
	waitRepo              *valkey.WaitRepository
	validationService     *ValidationService
	activityRepo          domain.ActivityRepository
	stepStateRepo         domain.StepStateRepository
	threadRepo            domain.ThreadRepository
	cacheManager          domain.CacheManager
	natsPublisher         domain.NotificationPublisher
	natsArchivalPublisher *natsrepo.ArchivalPublisher
	threadAccessService   *ThreadAccessService
	rbacLoader            *rbac.Loader
	validationPool        *workerpool.Pool
	notificationPool      *workerpool.Pool
	timeoutMonitor        domain.TimeoutMonitor
	logger                *zap.Logger
}

// ShouldReceiveNotification exports shouldReceiveNotification for use in tests and other packages.
func (s *NotificationService) ShouldReceiveNotification(
	userPerms, requiredPerms []string,
	userID, stepOwnerID string,
) bool {
	return s.shouldReceiveNotification(userPerms, requiredPerms, userID, stepOwnerID)
}

// ScheduleThreadMaxDurationTimeout exports scheduleThreadMaxDurationTimeout for tests.
func (s *NotificationService) ScheduleThreadMaxDurationTimeout(ctx context.Context, threadID string, graph *domain.ContractGraph, thread *domain.Thread, now time.Time) {
	s.scheduleThreadMaxDurationTimeout(ctx, threadID, graph, thread, now)
}

// CancelThreadMaxDurationTimeout exports cancelThreadMaxDurationTimeout for tests.
func (s *NotificationService) CancelThreadMaxDurationTimeout(ctx context.Context, threadID string) {
	s.cancelThreadMaxDurationTimeout(ctx, threadID)
}

// HandleNoContractStep exports handleNoContractStep for tests.
func (s *NotificationService) HandleNoContractStep(ctx context.Context, threadID, stepID, stepName, ownerID string, req *domain.RecordEventCmd, thread *domain.Thread) {
	s.handleNoContractStep(ctx, threadID, stepID, stepName, ownerID, req, thread)
}

// GetRequiredPermissionsForNotification exports getRequiredPermissionsForNotification.
func GetRequiredPermissionsForNotification(status, stepStatus, severity, violationType string) []string {
	return getRequiredPermissionsForNotification(status, stepStatus, severity, violationType)
}

// NewNotificationService creates a new notification service.
func NewNotificationService(
	validationService *ValidationService,
	activityRepo domain.ActivityRepository,
	stepStateRepo domain.StepStateRepository,
	threadRepo domain.ThreadRepository,
	cacheManager domain.CacheManager,
	natsPublisher domain.NotificationPublisher,
	natsArchivalPublisher *natsrepo.ArchivalPublisher,
	threadAccessService *ThreadAccessService,
	rbacLoader *rbac.Loader,
	validationPool *workerpool.Pool,
	notificationPool *workerpool.Pool,
	timeoutMonitor domain.TimeoutMonitor,
	logger *zap.Logger,
) *NotificationService {
	return &NotificationService{
		validationService:     validationService,
		activityRepo:          activityRepo,
		stepStateRepo:         stepStateRepo,
		threadRepo:            threadRepo,
		cacheManager:          cacheManager,
		natsPublisher:         natsPublisher,
		natsArchivalPublisher: natsArchivalPublisher,
		threadAccessService:   threadAccessService,
		rbacLoader:            rbacLoader,
		validationPool:        validationPool,
		notificationPool:      notificationPool,
		timeoutMonitor:        timeoutMonitor,
		logger:                logger,
	}
}

// PerformAsyncValidation runs all non-blocking validations via worker pool.
func (s *NotificationService) PerformAsyncValidation(
	threadID string,
	stepID string,
	stepName string,
	ownerID string,
	req *domain.RecordEventCmd,
	thread *domain.Thread,
	graph *domain.ContractGraph,
	stepNode domain.GraphNode,
) {
	submitted := s.validationPool.Submit(func(ctx context.Context) {
		s.logger.Info("starting validation",
			zap.String("thread", threadID),
			zap.String("step_uuid", stepID),
			zap.String("step_name", stepName),
		)

		// Cancel any pending timeouts waiting for this step to start
		if s.timeoutMonitor != nil && graph != nil {
			s.cancelPendingTimeoutsForStep(ctx, threadID, stepName, graph)
		}

		if graph == nil {
			s.handleNoContractStep(ctx, threadID, stepID, stepName, ownerID, req, thread)
			return
		}

		notifications := s.performNonBlockingValidations(ctx, thread, req, stepNode, graph, stepID, ownerID)

		s.logger.Debug("generated notifications",
			zap.Int("count", len(notifications)),
			zap.String("thread_id", threadID),
		)

		idempKey := req.IdempotencyKey
		if idempKey == "" {
			idempKey = stepID
			s.logger.Debug("no idempotency key provided, using stepID", zap.String("step_id", stepID))
		}

		s.processValidationNotifications(ctx, threadID, stepID, stepName, ownerID, idempKey, notifications, graph, thread, req.Status, req)

		// Schedule transition timeouts AFTER validations complete (inside worker pool)
		// This ensures scheduling happens in the same async context as cancellation
		if req.Status == StepStatusSuccess && s.timeoutMonitor != nil && graph != nil {
			// Check if there are critical violations that would prevent timeout scheduling
			hasCriticalViolation := false
			for _, notif := range notifications {
				if notif.Severity == "critical" {
					hasCriticalViolation = true
					break
				}
			}
			if !hasCriticalViolation {
				s.scheduleTransitionTimeouts(ctx, threadID, stepName, req, graph, thread)
			}
		}

		s.logger.Debug("completed validation", zap.String("thread_id", threadID))
	})

	if !submitted {
		s.finishWait(context.Background(), req, stepID, "unavailable", "Validation queue is full; event has not been validated", nil, false)
		s.logger.Warn("job dropped due to backpressure",
			zap.String("thread_id", threadID),
			zap.String("step", stepName),
		)
	}
}

// handleNoContractStep handles steps on threads with no contract: emits an execution
// notification and archives step state, then returns without validation.
func (s *NotificationService) handleNoContractStep(
	ctx context.Context,
	threadID, stepID, stepName, ownerID string,
	req *domain.RecordEventCmd,
	thread *domain.Thread,
) {
	s.finishWait(ctx, req, stepID, "unvalidated", "Thread has no contract", nil, true)
	contractName := ""
	if thread != nil {
		contractName = thread.ContractName
	}

	var message string
	switch req.Status {
	case StepStatusSuccess:
		message = fmt.Sprintf("Step %q completed successfully (no contract)", stepName)
	case StepStatusFailed:
		message = fmt.Sprintf("Step %q failed (no contract)", stepName)
	case StepStatusError:
		message = fmt.Sprintf("Step %q encountered an error (no contract)", stepName)
	default:
		message = fmt.Sprintf("Step %q recorded with status %q (no contract)", stepName, req.Status)
	}
	idempKey := req.IdempotencyKey
	if idempKey == "" {
		idempKey = stepID
	}

	eventTimestamp := recordEventTimestamp(req)
	executionNotif := domain.ValidationNotification{
		NotificationID: uuid.New().String(),
		ThreadID:       threadID,
		StepID:         stepID,
		StepName:       stepName,
		OwnerID:        ownerID,
		ContractName:   contractName,
		StepStatus:     req.Status,
		Status:         ValidationStatusNone,
		Severity:       string(domain.SeverityInfo),
		Message:        message,
		Details: map[string]interface{}{
			"context":        req.Context,
			"idempotencyKey": idempKey,
		},
		Timestamp: eventTimestamp,
	}

	if s.natsPublisher != nil {
		s.submitNotificationJob(executionNotif)
	}

	if err := s.activityRepo.ArchiveStepState(ctx, buildStepStateSnapshot(
		stepID, threadID, stepName, idempKey, req.Status, ownerID, req, 0, "", "",
	)); err != nil {
		s.logger.Error("failed to archive step state (no contract)", zap.Error(err))
	}

	s.logger.Debug("completed validation (no contract)", zap.String("thread_id", threadID))
}

// performNonBlockingValidations executes all validation checks and returns notifications.
func (s *NotificationService) performNonBlockingValidations(
	ctx context.Context,
	thread *domain.Thread,
	req *domain.RecordEventCmd,
	stepNode domain.GraphNode,
	graph *domain.ContractGraph,
	stepID string,
	ownerID string,
) []domain.ValidationNotification {
	notifications := make([]domain.ValidationNotification, 0, 5)
	now := recordEventTimestamp(req)

	// 1. Step Timeout Exceeded (Critical)
	if violation := s.validationService.CheckStepTimeout(stepNode, req.StartedAt, req.FinishedAt); violation != nil {
		details := ensureDetails(violation.Details)
		details["duration"] = violation.Duration
		details["limit"] = violation.Limit
		notifications = append(notifications, domain.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			ContractName:   thread.ContractName,
			StepStatus:     req.Status,
			Status:         ValidationStatusViolated,
			ViolationType:  string(domain.ViolationStepTimeoutExceeded),
			Severity:       string(domain.SeverityCritical),
			Message:        violation.Message,
			Details:        details,
			Timestamp:      now,
		})
	}

	// 2. Max Duration Exceeded (Critical)
	if violation := s.validationService.CheckMaxDuration(thread, graph, now); violation != nil {
		details := ensureDetails(violation.Details)
		details["duration"] = violation.Duration
		details["limit"] = violation.Limit
		notifications = append(notifications, domain.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			ContractName:   thread.ContractName,
			StepStatus:     req.Status,
			Status:         ValidationStatusViolated,
			ViolationType:  string(domain.ViolationMaxDurationExceeded),
			Severity:       string(domain.SeverityCritical),
			Message:        violation.Message,
			Details:        details,
			Timestamp:      now,
		})
	}

	// 3. Multiple Terminal States — handled atomically by Lua script.
	// 4. Retry Limit Exceeded   — handled atomically by Lua script.
	// 5. Invalid Transition     — handled atomically by Lua script.

	return notifications
}

// processValidationNotifications stores notifications and updates step state via Lua script.
func (s *NotificationService) processValidationNotifications(
	ctx context.Context,
	threadID, stepID, stepName, ownerID, idempotencyKey string,
	notifications []domain.ValidationNotification,
	graph *domain.ContractGraph,
	thread *domain.Thread,
	originalStatus string,
	req *domain.RecordEventCmd,
) {
	s.logger.Debug("processing Go violations",
		zap.Int("count", len(notifications)),
		zap.String("thread_id", threadID),
		zap.String("step", stepName),
	)

	hasCriticalViolation := false
	var existingViolations []domain.Violation

	for _, notif := range notifications {
		if notif.Severity == string(domain.SeverityCritical) {
			s.logger.Info("found critical violation",
				zap.String("type", notif.ViolationType),
				zap.String("message", notif.Message),
			)
			hasCriticalViolation = true
			existingViolations = append(existingViolations, domain.Violation{
				Type:     notif.ViolationType,
				Severity: notif.Severity,
				Message:  notif.Message,
				Details:  notif.Details,
			})
		}
	}

	isTerminal := slices.Contains(graph.Graph.TerminalSteps, stepName)

	eventTimestamp := recordEventTimestamp(req)
	params := buildValidateStepParams(threadID, stepID, stepName, idempotencyKey, originalStatus, ownerID, isTerminal, existingViolations, graph, eventTimestamp)
	rawContext, err := json.Marshal(req.Context)
	if err != nil {
		s.logger.Error("failed to encode reference context", zap.Error(err))
		return
	}
	params.RawContext = string(rawContext)
	params.InvocationID = req.InvocationID

	luaStart := perf.Now()
	result, err := s.stepStateRepo.ValidateAndUpdateStepState(ctx, params)
	luaDuration := perf.Since(luaStart)

	stepEventID := threadID + ":" + stepName + ":" + idempotencyKey
	perf.LogStructured("Validation LUA_SCRIPT",
		zap.String("step_event_id", stepEventID),
		zap.Duration("duration", luaDuration),
		zap.Bool("success", err == nil),
	)

	if err != nil {
		s.finishWait(ctx, req, stepID, "unavailable", "Validation storage unavailable", nil, false)
		s.logger.Error("error validating and updating step state", zap.Error(err))
		return
	}

	s.logger.Debug("step state updated",
		zap.String("status", result.Status),
		zap.Int("violations", len(result.Violations)),
		zap.Int("retry_count", result.RetryCount),
	)

	// Combine Go and Lua violations.
	allViolations := make([]map[string]interface{}, 0, len(notifications)+len(result.Violations))
	for _, notif := range notifications {
		allViolations = append(allViolations, map[string]interface{}{
			"type": notif.ViolationType, "severity": notif.Severity,
			"message": notif.Message, "details": notif.Details,
		})
	}
	for _, v := range result.Violations {
		allViolations = append(allViolations, map[string]interface{}{
			"type": v.Type, "severity": v.Severity,
			"message": v.Message, "details": v.Details,
		})
	}

	status := result.Status
	if status == "" {
		status = ValidationStatusPassed
	}
	hasCriticalViolation = hasCriticalViolation || result.HasCriticalViolation

	finalDetails := make(map[string]interface{})
	var finalStatus, finalMessage, violationType, severity string

	if len(allViolations) > 0 {
		finalStatus = ValidationStatusViolated
		if len(allViolations) == 1 {
			v := allViolations[0]
			violationType = v["type"].(string)
			severity = v["severity"].(string)
			finalMessage = v["message"].(string)
			if details, ok := v["details"].(map[string]interface{}); ok {
				for k, val := range details {
					finalDetails[k] = val
				}
			}
		} else {
			finalMessage = fmt.Sprintf("Step %q completed with %s violations", stepName, strconv.Itoa(len(allViolations)))
			finalDetails["violations"] = allViolations
		}
	} else {
		finalStatus = ValidationStatusPassed
		finalMessage = fmt.Sprintf("Step %q completed successfully", stepName)
	}

	combinedViolations := result.Violations
	s.finishWait(ctx, req, stepID, finalStatus, fmt.Sprintf("Contract validation %s (reported execution: %s)", finalStatus, originalStatus), combinedViolations, true)
	finalDetails["idempotencyKey"] = idempotencyKey

	executionNotif := domain.ValidationNotification{
		NotificationID: uuid.New().String(),
		ThreadID:       threadID,
		StepID:         stepID,
		StepName:       stepName,
		OwnerID:        ownerID,
		ContractName:   thread.ContractName,
		StepStatus:     originalStatus,
		Status:         ValidationStatusNone,
		Severity:       string(domain.SeverityInfo),
		Message:        fmt.Sprintf("Step %q execution %s", stepName, originalStatus),
		Details: map[string]interface{}{
			"context":        req.Context,
			"idempotencyKey": idempotencyKey,
		},
		Timestamp: eventTimestamp,
	}

	validationNotif := domain.ValidationNotification{
		NotificationID: uuid.New().String(),
		ThreadID:       threadID,
		StepID:         stepID,
		StepName:       stepName,
		OwnerID:        ownerID,
		ContractName:   thread.ContractName,
		StepStatus:     originalStatus,
		Status:         finalStatus,
		ViolationType:  violationType,
		Severity:       severity,
		Message:        finalMessage,
		Details:        finalDetails,
		Timestamp:      eventTimestamp,
	}

	// Intentional detached context: this goroutine outlives the request lifecycle.
	if s.natsPublisher != nil {
		go s.publishDualNotifications(context.Background(), executionNotif, validationNotif)
	}

	// NOTE: Timeout scheduling moved to PerformAsyncValidation worker pool
	// to prevent race conditions with cancellation logic

	// Handle terminal step completion.
	// Lua returns the step outcome (success), while marking thread metadata
	// completed separately. Persist completion for a successful terminal step.
	if isTerminal && result.Status == StepStatusSuccess && !result.HasCriticalViolation {
		s.logger.Info("thread marked as COMPLETED", zap.String("thread_id", threadID))

		// Cancel thread max duration timeout
		s.cancelThreadMaxDurationTimeout(ctx, threadID)

		// Update Valkey status to "completed" SYNCHRONOUSLY.
		// This is critical: it must happen before any WebSocket disconnect can call EndThread.
		if s.threadRepo != nil {
			if err := s.threadRepo.UpdateThreadStatus(ctx, threadID, ThreadStatusCompleted, eventTimestamp); err != nil {
				s.logger.Error("failed to update thread status in Valkey",
					zap.String("thread_id", threadID), zap.Error(err))
			} else {
				s.logger.Info("thread status updated to completed in Valkey",
					zap.String("thread_id", threadID))
			}
		}

		if s.natsPublisher != nil {
			completionNotif := domain.ValidationNotification{
				NotificationID: uuid.New().String(),
				ThreadID:       threadID,
				StepID:         stepID,
				StepName:       stepName,
				OwnerID:        ownerID,
				ContractName:   thread.ContractName,
				StepStatus:     originalStatus,
				Status:         ValidationStatusPassed,
				Severity:       "",
				Message:        fmt.Sprintf("Thread completed successfully at terminal step %q", stepName),
				Timestamp:      eventTimestamp,
			}
			go s.publishToAuthorizedMembers(ctx, completionNotif)
		}

		completedAt := eventTimestamp
		thread.CompletedAt = &completedAt

		if err := s.activityRepo.ArchiveThreadMetadata(ctx, &domain.Thread{
			ID:              threadID,
			OwnerID:         thread.OwnerID,
			CompanyID:       thread.CompanyID,
			ContractID:      thread.ContractID,
			ContractName:    thread.ContractName,
			ContractVersion: thread.ContractVersion,
			LastHash:        thread.LastHash,
			StartedAt:       thread.StartedAt,
			CompletedAt:     thread.CompletedAt,
		}, ThreadStatusCompleted); err != nil {
			s.logger.Error("failed to archive thread metadata", zap.Error(err))
		}
	}

	if err := s.activityRepo.ArchiveValidationResults(ctx, threadID, stepID, stepName, idempotencyKey, notifications, status, hasCriticalViolation); err != nil {
		s.logger.Error("failed to archive validation results", zap.Error(err))
	} else {
		s.logger.Debug("validation results archived", zap.String("step", stepName))
	}

	now := eventTimestamp.Format(time.RFC3339Nano)
	firstSeenAt := result.FirstSeenAt
	if firstSeenAt == "" {
		firstSeenAt = now
	}

	previousStepName := extractStepName(result.PreviousStep)
	convertedCtx := convertContext(req.Context)
	contextJSON := marshalContext(convertedCtx)

	snapshot := buildStepStateSnapshot(stepID, threadID, stepName, idempotencyKey, result.Status, ownerID, req, result.RetryCount, firstSeenAt, previousStepName)
	if result.Status == StepStatusSuccess && !result.HasCriticalViolation {
		snapshot.SuccessOrder = result.SuccessOrder
		snapshot.SuccessContext = params.RawContext
	}
	if err := s.activityRepo.ArchiveStepState(ctx, snapshot); err != nil {
		s.logger.Error("failed to archive step state", zap.Error(err))
	} else {
		s.logger.Debug("step state archived", zap.String("step", stepName))
	}
	_ = contextJSON // used inside buildStepStateSnapshot via req

	s.cacheManager.ClearThreadCache(threadID)
}

// getRequiredPermissionsForNotification returns the permission strings needed to receive
// a notification of a given type/status/severity.
func getRequiredPermissionsForNotification(status, stepStatus, severity, violationType string) []string {
	if status == "none" {
		switch stepStatus {
		case "cancelled", "completed":
			return []string{fmt.Sprintf("notification.thread.%s.*", stepStatus)}
		case "failed", "error":
			return []string{"notification.step.failed.*", "notification.step.failed.own"}
		case "success":
			return []string{"notification.step.success.*", "notification.step.success.own"}
		default:
			return nil
		}
	}

	switch status {
	case "violated":
		base := []string{"notification.rule.violated.*", "notification.rule.violated.own"}
		switch violationType {
		case "step_timeout_exceeded":
			return append(base, "notification.rule.violated.timeout.*", "notification.rule.violated.timeout.own")
		case "retry_limit_exceeded":
			return append(base, "notification.rule.violated.retry_limit.*", "notification.rule.violated.retry_limit.own")
		}
		if severity == "critical" {
			return append(base, "notification.rule.violated.critical")
		}
		return base

	case "passed":
		return []string{"notification.rule.passed.*", "notification.rule.passed.own"}

	default:
		return nil
	}
}

// submitNotificationJob submits a notification publishing job to the notification worker pool.
func (s *NotificationService) submitNotificationJob(notification domain.ValidationNotification) {
	s.notificationPool.Submit(func(ctx context.Context) {
		s.publishToAuthorizedMembers(ctx, notification)
	})
}

// publishToAuthorizedMembers publishes a notification to all thread members with appropriate permissions.
func (s *NotificationService) publishToAuthorizedMembers(ctx context.Context, notification domain.ValidationNotification) {
	if notification.Source == "" {
		if notification.Status == ValidationStatusNone {
			notification.Source = domain.NotificationSourceStep
		} else {
			notification.Source = domain.NotificationSourceRule
		}
	}

	if notification.NotificationType == "" {
		if notification.Status == ValidationStatusNone {
			// Map StepStatus to domain constants
			switch notification.StepStatus {
			case "started":
				notification.NotificationType = domain.NotificationTypeStepStarted
			case "success":
				notification.NotificationType = domain.NotificationTypeStepSuccess
			case "failed":
				notification.NotificationType = domain.NotificationTypeStepFailed
			case "error":
				notification.NotificationType = domain.NotificationTypeStepError
			case "cancelled":
				notification.NotificationType = domain.NotificationTypeStepCancelled
			case "skipped":
				notification.NotificationType = domain.NotificationTypeStepSkipped
			default:
				notification.NotificationType = domain.NotificationType(fmt.Sprintf("step.%s", notification.StepStatus))
			}
		} else {
			// Map Status to domain constants
			switch notification.Status {
			case "passed":
				notification.NotificationType = domain.NotificationTypeRulePassed
			case "violated":
				notification.NotificationType = domain.NotificationTypeRuleViolated
			default:
				notification.NotificationType = domain.NotificationType(fmt.Sprintf("rule.%s", notification.Status))
			}
		}
	}

	requiredPerms := getRequiredPermissionsForNotification(
		notification.Status, notification.StepStatus, notification.Severity, notification.ViolationType,
	)
	if len(requiredPerms) == 0 {
		s.logger.Debug("no permissions required for notification type")
		return
	}

	users, err := s.threadAccessService.GetUsersByPermissions(ctx, notification.ThreadID, requiredPerms)
	if err != nil {
		s.logger.Error("failed to get authorized users", zap.Error(err))
		return
	}
	if len(users) == 0 {
		s.logger.Debug("no users found with required permissions")
		return
	}

	publishedCount := 0
	for _, user := range users {
		if !s.shouldReceiveNotification(user.Permissions, requiredPerms, user.UserID, notification.OwnerID) {
			continue
		}
		userNotif := notification
		userNotif.OwnerID = user.UserID
		if err := s.natsPublisher.PublishNotification(ctx, userNotif); err != nil {
			s.logger.Error("failed to publish to user", zap.String("user_id", user.UserID), zap.Error(err))
			continue
		}
		publishedCount++
	}

	s.logger.Debug("published to users",
		zap.Int("published_count", publishedCount),
		zap.Int("total_users", len(users)),
		zap.Strings("perms", requiredPerms),
	)

	shouldArchive := notification.Source == domain.NotificationSourceRule &&
		(notification.Status == "violated" || notification.Severity == "warning")

	if s.natsArchivalPublisher != nil && publishedCount > 0 && shouldArchive {
		if err := s.archiveNotification(ctx, notification); err != nil {
			s.logger.Error("failed to archive validation result", zap.Error(err))
		}
	}
}

// publishDualNotifications sends both execution and validation notifications in a single member query.
func (s *NotificationService) publishDualNotifications(
	ctx context.Context,
	executionNotif, validationNotif domain.ValidationNotification,
) {
	executionNotif.Source = domain.NotificationSourceStep
	executionNotif.NotificationType = domain.NotificationType(fmt.Sprintf("step.%s", executionNotif.StepStatus))
	validationNotif.Source = domain.NotificationSourceRule
	validationNotif.NotificationType = domain.NotificationType(fmt.Sprintf("rule.%s", validationNotif.Status))

	executionPerms := getRequiredPermissionsForNotification(
		executionNotif.Status, executionNotif.StepStatus, executionNotif.Severity, executionNotif.ViolationType,
	)
	validationPerms := getRequiredPermissionsForNotification(
		validationNotif.Status, validationNotif.StepStatus, validationNotif.Severity, validationNotif.ViolationType,
	)

	if len(executionPerms) == 0 && len(validationPerms) == 0 {
		s.logger.Debug("no permissions required for either notification")
		return
	}

	// Build combined perm list without mutating executionPerms.
	allPerms := make([]string, 0, len(executionPerms)+len(validationPerms))
	allPerms = append(allPerms, executionPerms...)
	allPerms = append(allPerms, validationPerms...)

	users, err := s.threadAccessService.GetUsersByPermissions(ctx, executionNotif.ThreadID, allPerms)
	if err != nil {
		s.logger.Error("failed to get authorized users", zap.Error(err))
		return
	}
	if len(users) == 0 {
		s.logger.Debug("no users found with required permissions")
		return
	}

	executionCount, validationCount := 0, 0

	for _, user := range users {
		if len(executionPerms) > 0 && s.shouldReceiveNotification(user.Permissions, executionPerms, user.UserID, executionNotif.OwnerID) {
			n := executionNotif
			n.OwnerID = user.UserID
			if err := s.natsPublisher.PublishNotification(ctx, n); err != nil {
				s.logger.Error("failed to publish execution notif", zap.String("user_id", user.UserID), zap.Error(err))
			} else {
				executionCount++
			}
		}

		if len(validationPerms) > 0 && s.shouldReceiveNotification(user.Permissions, validationPerms, user.UserID, validationNotif.OwnerID) {
			n := validationNotif
			n.OwnerID = user.UserID
			if err := s.natsPublisher.PublishNotification(ctx, n); err != nil {
				s.logger.Error("failed to publish validation notif", zap.String("user_id", user.UserID), zap.Error(err))
			} else {
				validationCount++
			}
		}
	}

	s.logger.Debug("published dual notifications",
		zap.Int("execution_count", executionCount),
		zap.Int("validation_count", validationCount),
		zap.Int("total_users", len(users)),
	)

	if s.natsArchivalPublisher == nil {
		return
	}
	if executionCount > 0 && executionNotif.Severity == "warning" {
		if err := s.archiveNotification(ctx, executionNotif); err != nil {
			s.logger.Error("failed to archive execution warning", zap.Error(err))
		}
	}
	if validationCount > 0 && (validationNotif.Status == "violated" || validationNotif.Severity == "warning") {
		if err := s.archiveNotification(ctx, validationNotif); err != nil {
			s.logger.Error("failed to archive validation result", zap.Error(err))
		}
	}
}

// shouldReceiveNotification reports whether a user should receive a notification
// based on their permissions, handling wildcard and .own permission logic.
func (s *NotificationService) shouldReceiveNotification(
	userPerms, requiredPerms []string,
	userID, stepOwnerID string,
) bool {
	for _, userPerm := range userPerms {
		hasOwn := strings.HasSuffix(userPerm, ".own")
		isOwner := userID == stepOwnerID

		for _, reqPerm := range requiredPerms {
			// If user has .own, they MUST be the owner for this perm to match.
			if hasOwn {
				if !isOwner {
					continue
				}
				ownBase := strings.TrimSuffix(userPerm, ".own")
				if ownBase == reqPerm || ownBase == strings.TrimSuffix(reqPerm, ".*") || ownBase == strings.TrimSuffix(reqPerm, ".own") {
					return true
				}
				continue
			}

			// Global match (exact or wildcard)
			if userPerm == reqPerm {
				return true
			}
			if strings.HasSuffix(userPerm, ".*") {
				prefix := strings.TrimSuffix(userPerm, ".*")
				if strings.HasPrefix(reqPerm, prefix) {
					return true
				}
			}
		}
	}
	return false
}

// archiveNotification publishes a notification as an activity log event to NATS for archival.
func (s *NotificationService) archiveNotification(ctx context.Context, n domain.ValidationNotification) error {
	detailsJSON, _ := json.Marshal(n.Details)

	idempotencyKey := ""
	if n.Details != nil {
		if v, ok := n.Details["idempotencyKey"].(string); ok {
			idempotencyKey = v
		}
	}

	stepID := n.StepName
	if idempotencyKey != "" {
		stepID = n.StepName + ":" + idempotencyKey
	}

	return s.natsArchivalPublisher.PublishActivityLog(ctx, map[string]interface{}{
		"threadId":         n.ThreadID,
		"type":             ActivityTypeValidationResult,
		"stepId":           stepID,
		"actor":            n.OwnerID,
		"actorService":     ActorServiceRuleEngine,
		"timestamp":        n.Timestamp.Format(time.RFC3339),
		"status":           n.Status,
		"notificationId":   n.NotificationID,
		"source":           n.Source,
		"notificationType": n.NotificationType,
		"stepStatus":       n.StepStatus,
		"violationType":    n.ViolationType,
		"severity":         n.Severity,
		"message":          n.Message,
		"details":          string(detailsJSON),
	})
}

// --- package-level helpers ---

// buildValidateStepParams constructs the params struct for ValidateAndUpdateStepState.
func buildValidateStepParams(
	threadID, stepID, stepName, idempotencyKey, status, ownerID string,
	isTerminal bool,
	existingViolations []domain.Violation,
	graph *domain.ContractGraph,
	timestamp time.Time,
) domain.ValidateStepParams {
	maxRetries := 0
	transitionsMap := make(map[string][]string)
	freshSteps := []string{}
	requiredSteps := []string{}
	terminalSteps := []string{}
	allowMultipleTerminals := false

	if graph != nil {
		for _, t := range graph.Transitions {
			transitionsMap[t.From] = t.To
			if t.From == stepName && t.MaxRetries > 0 {
				maxRetries = t.MaxRetries
			}
		}
		if node, ok := graph.Graph.Nodes[stepName]; ok {
			requiredSteps = slices.Clone(node.DependsOn)
			freshSteps = slices.Clone(node.FreshDependsOn)
		}
		terminalSteps = graph.Graph.TerminalSteps
		if graph.Validation != nil {
			allowMultipleTerminals = graph.Validation.AllowMultipleTerminals
		}
	}

	return domain.ValidateStepParams{
		ThreadID:               threadID,
		StepID:                 stepID,
		StepName:               stepName,
		IdempotencyKey:         idempotencyKey,
		Status:                 status,
		ExistingViolations:     existingViolations,
		IsTerminalStep:         isTerminal,
		Timestamp:              timestamp.Format(time.RFC3339Nano),
		MaxRetries:             maxRetries,
		TransitionsMap:         transitionsMap,
		RequiredSteps:          requiredSteps,
		FreshRequiredSteps:     freshSteps,
		TerminalSteps:          terminalSteps,
		AllowMultipleTerminals: allowMultipleTerminals,
		Actor:                  ownerID,
	}
}

// buildStepStateSnapshot constructs a StepStateSnapshot for archival.
func buildStepStateSnapshot(
	stepID, threadID, stepName, idempotencyKey, status, ownerID string,
	req *domain.RecordEventCmd,
	retryCount int,
	firstSeenAt, previousStep string,
) *domain.StepStateSnapshot {
	eventTimestamp := recordEventTimestamp(req)
	now := eventTimestamp.Format(time.RFC3339Nano)
	if firstSeenAt == "" {
		firstSeenAt = now
	}
	parsedFirstSeen, _ := time.Parse(time.RFC3339Nano, firstSeenAt)
	parsedLastUpdated := eventTimestamp
	var startedAt, finishedAt *time.Time
	if req.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.StartedAt); err == nil {
			startedAt = &t
		}
	}
	if req.FinishedAt != "" {
		if t, err := time.Parse(time.RFC3339, req.FinishedAt); err == nil {
			finishedAt = &t
		}
	}

	return &domain.StepStateSnapshot{
		ID:             stepID,
		ThreadID:       threadID,
		StepName:       stepName,
		IdempotencyKey: idempotencyKey,
		Status:         status,
		RetryCount:     retryCount,
		FirstSeenAt:    parsedFirstSeen,
		LastUpdatedAt:  parsedLastUpdated,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
		PreviousStep:   previousStep,
		Actor:          ownerID,
		ActorService:   req.ServiceName,
		LatestContext:  marshalContext(convertContext(req.Context)),
	}
}

// recordEventTimestamp returns the producer's authoritative event time when it
// is available (including an OTLP span's end time). Receipt time is only a
// fallback for internal callers that omit or corrupt FinishedAt.
func recordEventTimestamp(req *domain.RecordEventCmd) time.Time {
	if req != nil && req.FinishedAt != "" {
		if timestamp, err := time.Parse(time.RFC3339Nano, req.FinishedAt); err == nil {
			return timestamp
		}
	}
	return time.Now()
}

// extractStepName extracts the step name from a "stepName:idempotencyKey" composite key.
func extractStepName(previousStep string) string {
	if idx := strings.Index(previousStep, ":"); idx > 0 {
		return previousStep[:idx]
	}
	return ""
}

// convertContext converts a map[string]string (SDK wire format) to map[string]any by
// attempting to JSON-parse each string value. Values that are valid JSON become nested
// objects/arrays; values that are not valid JSON are kept as plain strings.
func convertContext(raw map[string]string) map[string]any {
	out := make(map[string]any, len(raw))
	parsedCount := 0
	for k, v := range raw {
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			out[k] = parsed
			parsedCount++
		} else {
			out[k] = v
		}
	}
	return out
}

// marshalContext serialises a context map to a JSON string, returning "{}" on empty input or error.
func marshalContext(ctx map[string]any) string {
	if len(ctx) == 0 {
		return "{}"
	}
	b, err := json.Marshal(ctx)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// ensureDetails returns d if non-nil, otherwise allocates a fresh map.
func ensureDetails(d map[string]interface{}) map[string]interface{} {
	if d != nil {
		return d
	}
	return make(map[string]interface{})
}

// cancelPendingTimeoutsForStep cancels any pending timeouts waiting for this step to start
func (s *NotificationService) cancelPendingTimeoutsForStep(
	ctx context.Context,
	threadID string,
	stepName string,
	graph *domain.ContractGraph,
) {
	// Find all transitions that have this step as a target
	var incomingTransitions []domain.Transition
	for _, transition := range graph.Transitions {
		for _, toStep := range transition.To {
			if toStep == stepName {
				incomingTransitions = append(incomingTransitions, transition)
				break
			}
		}
	}

	if len(incomingTransitions) == 0 {
		return
	}

	// Cancel timeout for each incoming transition
	for _, transition := range incomingTransitions {
		// Reconstruct the timeout ID that was used when scheduling
		timeoutID := fmt.Sprintf("%s:%s:%s:transition", threadID, transition.From, strings.Join(transition.To, ","))

		if err := s.timeoutMonitor.CancelTimeout(ctx, timeoutID, threadID, fmt.Sprintf("step '%s' started", stepName)); err != nil {
			s.logger.Warn("failed to cancel timeout",
				zap.String("timeout_id", timeoutID),
				zap.String("thread_id", threadID),
				zap.String("from", transition.From),
				zap.String("to_step", stepName),
				zap.Error(err),
			)
		} else {
			s.logger.Debug("cancelled timeout",
				zap.String("timeout_id", timeoutID),
				zap.String("thread_id", threadID),
				zap.String("from", transition.From),
				zap.String("to_step", stepName),
			)
		}
	}
}

// scheduleTransitionTimeouts schedules timeout events for all outgoing transitions from the current step
func (s *NotificationService) scheduleTransitionTimeouts(
	ctx context.Context,
	threadID string,
	stepName string,
	req *domain.RecordEventCmd,
	graph *domain.ContractGraph,
	thread *domain.Thread,
) {
	// Find transitions from this step
	var outgoingTransitions []domain.Transition
	for _, transition := range graph.Transitions {
		if transition.From == stepName {
			outgoingTransitions = append(outgoingTransitions, transition)
		}
	}

	if len(outgoingTransitions) == 0 {
		return
	}

	// Parse startedAt timestamp
	startedAt, err := time.Parse(time.RFC3339, req.StartedAt)
	if err != nil {
		s.logger.Warn("failed to parse startedAt for timeout scheduling",
			zap.String("thread_id", threadID),
			zap.String("step_name", stepName),
			zap.Error(err),
		)
		return
	}

	// Schedule timeout for each outgoing transition that has a timeout defined
	for _, transition := range outgoingTransitions {
		if transition.Timeout == "" {
			continue
		}

		// Parse timeout duration
		timeout, err := time.ParseDuration(transition.Timeout)
		if err != nil {
			s.logger.Warn("invalid timeout duration format",
				zap.String("thread_id", threadID),
				zap.String("from", transition.From),
				zap.String("timeout", transition.Timeout),
				zap.Error(err),
			)
			continue
		}

		// Calculate deadline: startedAt + timeout
		deadline := startedAt.Add(timeout)

		// Generate timeout ID for deduplication and cancellation
		timeoutID := fmt.Sprintf("%s:%s:%s:transition", threadID, transition.From, strings.Join(transition.To, ","))

		// Create timeout event
		timeoutEvent := domain.TimeoutEvent{
			ID:           timeoutID,
			ThreadID:     threadID,
			Type:         domain.TimeoutTypeTransition,
			FromStep:     transition.From,
			ToStep:       strings.Join(transition.To, ","), // Store as comma-separated for multiple targets
			Timeout:      transition.Timeout,
			ScheduledAt:  time.Now(),
			DeadlineAt:   deadline,
			ContractName: thread.ContractName,
			Metadata: map[string]interface{}{
				"owner_id":        thread.OwnerID,
				"step_name":       req.StepName,
				"idempotency_key": req.IdempotencyKey,
			},
		}

		// Schedule the timeout
		if err := s.timeoutMonitor.ScheduleTimeout(ctx, timeoutEvent); err != nil {
			s.logger.Error("failed to schedule transition timeout",
				zap.String("thread_id", threadID),
				zap.String("from", transition.From),
				zap.String("to", strings.Join(transition.To, ",")),
				zap.Error(err),
			)
		} else {
			s.logger.Debug("scheduled transition timeout",
				zap.String("thread_id", threadID),
				zap.String("from", transition.From),
				zap.String("to", strings.Join(transition.To, ",")),
				zap.Duration("timeout", timeout),
				zap.Time("deadline", deadline),
			)
		}
	}
}

func (s *NotificationService) finishWait(ctx context.Context, req *domain.RecordEventCmd, stepID, decision, message string, violations []domain.Violation, clear bool) {
	if s.waitRepo == nil {
		return
	}
	err := s.waitRepo.Complete(ctx, domain.WaitResult{ThreadID: req.ThreadID, StepName: req.StepName, StepID: stepID, InvocationID: req.InvocationID, Decision: decision, Message: message, Violations: violations}, clear)
	if err != nil {
		s.logger.Error("failed to publish invocation validation result", zap.Error(err), zap.String("step_id", stepID))
	}
}
