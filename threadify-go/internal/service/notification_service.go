package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// NotificationService orchestrates async validation and notification archival.
// It handles non-blocking validation processing, stores validation results in streams,
// and manages the archival of validation notifications without blocking main execution.

// NotificationService handles async validation processing and coordination
type NotificationService struct {
	validationService *ValidationService
	activityRepo      interfaces.ActivityRepository
	stepStateRepo     interfaces.StepStateRepository
	cacheManager      interfaces.CacheManager
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	validationService *ValidationService,
	activityRepo interfaces.ActivityRepository,
	stepStateRepo interfaces.StepStateRepository,
	cacheManager interfaces.CacheManager,
) *NotificationService {
	return &NotificationService{
		validationService: validationService,
		activityRepo:      activityRepo,
		stepStateRepo:     stepStateRepo,
		cacheManager:      cacheManager,
	}
}

// PerformAsyncValidation runs all non-blocking validations in a goroutine
func (s *NotificationService) PerformAsyncValidation(
	threadID string,
	stepID string,
	stepName string,
	ownerID string,
	req *models.RecordEventRequest,
	thread *models.Thread,
	graph *models.ContractGraph,
	stepNode models.GraphNode,
) {
	go func() {
		ctx := context.Background()
		fmt.Printf("[ASYNC-VALIDATION] Starting validation for thread=%s, step=%s, stepName=%s\n", threadID, stepID, stepName)

		// Perform all non-blocking validations (contract-specific if graph exists)
		var notifications []models.ValidationNotification
		if graph != nil {
			notifications = s.performNonBlockingValidations(
				ctx,
				thread,
				req,
				stepNode,
				graph,
				stepID,
				ownerID,
			)
		}

		fmt.Printf("[ASYNC-VALIDATION] Generated %d notifications for thread=%s\n", len(notifications), threadID)

		// Use stepID as default idempotency key if not provided
		idempKey := req.IdempotencyKey
		if idempKey == "" {
			idempKey = stepID
			fmt.Printf("[ASYNC-VALIDATION] No idempotency key provided, using stepID: %s\n", stepID)
		}

		// Process notifications and update step state
		s.processValidationNotifications(ctx, threadID, stepID, stepName, ownerID, idempKey, notifications, graph, thread, req.Status)

		fmt.Printf("[ASYNC-VALIDATION] Completed validation for thread=%s\n", threadID)
	}()
}

// performNonBlockingValidations executes all validation checks
func (s *NotificationService) performNonBlockingValidations(
	ctx context.Context,
	thread *models.Thread,
	req *models.RecordEventRequest,
	stepNode models.GraphNode,
	graph *models.ContractGraph,
	stepID string,
	ownerID string,
) []models.ValidationNotification {
	notifications := []models.ValidationNotification{}
	now := time.Now()

	// === STRUCTURAL/TIME-BASED VALIDATIONS (Run for ALL statuses) ===

	// 1. Step Timeout Exceeded (Critical)
	if violation := s.validationService.CheckStepTimeout(stepNode, req.StartedAt, req.FinishedAt); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationStepTimeoutExceeded,
			Severity:       models.SeverityCritical,
			Message:        violation.Message,
			Duration:       violation.Duration,
			Limit:          violation.Limit,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 2. Max Duration Exceeded (Critical)
	if violation := s.validationService.CheckMaxDuration(thread, graph); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationMaxDurationExceeded,
			Severity:       models.SeverityCritical,
			Message:        violation.Message,
			Duration:       violation.Duration,
			Limit:          violation.Limit,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 3. Multiple Terminal States (Critical/Configurable) - Handled by Lua script atomically
	// The Lua script checks for multiple terminal states and adds violation if found

	// 4. Retry Limit Exceeded (Critical) - Handled by Lua script atomically
	// The Lua script checks retry count and adds violation if exceeded

	// 5. Invalid Transition (Critical) - Handled by Lua script atomically
	// The Lua script checks allowed transitions and adds violation if invalid

	// === BUSINESS/DATA QUALITY VALIDATIONS (Only run for successful steps) ===
	if req.Status == "success" {
		// 6. Missing Optional Fields (Info)
		if violation := s.validationService.CheckMissingOptionalFields(stepNode, req.Context); violation != nil {
			notifications = append(notifications, models.ValidationNotification{
				NotificationID: uuid.New().String(),
				ThreadID:       req.ThreadID,
				StepID:         stepID,
				StepName:       req.StepName,
				OwnerID:        ownerID,
				Status:         models.NotificationStatusCompleted,
				ViolationType:  models.ViolationMissingOptionalField,
				Severity:       models.SeverityInfo,
				Message:        violation.Message,
				MissingFields:  violation.MissingFields,
				Details:        violation.Details,
				Timestamp:      now,
			})
		}

		// Note: Extra Undocumented Fields check skipped for now
	}

	// 8. Step Status Notifications
	hasCriticalViolations := false
	for _, notif := range notifications {
		if notif.Severity == models.SeverityCritical {
			hasCriticalViolations = true
			break
		}
	}

	// Add appropriate status notification based on step outcome
	if req.Status == "failed" || req.Status == "error" {
		// Step Failed - Always notify on failure
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			Message:        fmt.Sprintf("Step '%s' failed with status: %s", req.StepName, req.Status),
			Timestamp:      now,
		})
	} else if !hasCriticalViolations {
		// Step Completed Successfully - Only if no critical violations
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusCompleted,
			Message:        fmt.Sprintf("Step '%s' completed successfully", req.StepName),
			Timestamp:      now,
		})
	}

	return notifications
}

// processValidationNotifications stores notifications and updates step state via Lua script
func (s *NotificationService) processValidationNotifications(
	ctx context.Context,
	threadID string,
	stepID string,
	stepName string,
	ownerID string,
	idempotencyKey string,
	notifications []models.ValidationNotification,
	graph *models.ContractGraph,
	thread *models.Thread,
	originalStatus string,
) {
	// Store all notifications in stream via ActivityRepository
	for _, notif := range notifications {
		if err := s.activityRepo.StoreValidationNotification(ctx, notif); err != nil {
			fmt.Printf("Error storing notification: %v\n", err)
		}
	}

	fmt.Printf("[PROCESS-NOTIFICATIONS] Processing %d notifications for thread=%s, step=%s\n", len(notifications), threadID, stepName)

	// Check for critical violations
	hasCriticalViolation := false
	var criticalViolations []models.StepViolation

	for _, notif := range notifications {
		if notif.Severity == models.SeverityCritical {
			fmt.Printf("[CRITICAL-VIOLATION] Found violation: type=%s, message=%s\n", notif.ViolationType, notif.Message)
			hasCriticalViolation = true
			criticalViolations = append(criticalViolations, models.StepViolation{
				StepID:        stepID,
				StepName:      stepName,
				OwnerID:       ownerID,
				ViolationType: notif.ViolationType,
				Severity:      notif.Severity,
				Message:       notif.Message,
				Details:       notif.Details,
				ViolatedAt:    notif.Timestamp,
			})
		}
	}

	// Note: Repository will handle status determination based on violations

	// Check if step is terminal
	isTerminal := false
	if graph != nil {
		for _, terminalStep := range graph.Graph.TerminalSteps {
			if terminalStep == stepName {
				isTerminal = true
				break
			}
		}
	}

	// Prepare parameters for repository
	var existingViolations []interfaces.Violation
	for _, v := range criticalViolations {
		existingViolations = append(existingViolations, interfaces.Violation{
			Type:     string(v.ViolationType),
			Severity: string(v.Severity),
			Message:  v.Message,
			Details:  v.Details,
		})
	}

	// Get contract parameters
	maxRetries := 0
	allowedTransitions := []string{}
	terminalSteps := []string{}
	allowMultipleTerminals := false

	if graph != nil {
		// Get max retries and allowed transitions
		for _, transition := range graph.Transitions {
			if transition.From == stepName {
				if transition.MaxRetries > 0 {
					maxRetries = transition.MaxRetries
				}
				allowedTransitions = transition.To
				break
			}
		}

		// Get terminal steps
		terminalSteps = graph.Graph.TerminalSteps

		// Check if multiple terminals allowed
		if graph.Validation != nil {
			allowMultipleTerminals = graph.Validation.AllowMultipleTerminals
		}
	}

	// Call repository to validate and update atomically
	result, err := s.stepStateRepo.ValidateAndUpdateStepState(ctx, interfaces.ValidateStepParams{
		ThreadID:               threadID,
		StepID:                 stepID,
		StepName:               stepName,
		IdempotencyKey:         idempotencyKey,
		Status:                 originalStatus,
		ExistingViolations:     existingViolations,
		IsTerminalStep:         isTerminal,
		Timestamp:              time.Now().Format(time.RFC3339),
		MaxRetries:             maxRetries,
		AllowedTransitions:     allowedTransitions,
		TerminalSteps:          terminalSteps,
		AllowMultipleTerminals: allowMultipleTerminals,
	})

	if err != nil {
		fmt.Printf("[REPO-ERROR] Error validating and updating step state: %v\n", err)
		return
	}

	fmt.Printf("[REPO-SUCCESS] Step state updated, status: %s, violations: %d, retry count: %d\n",
		result.Status, len(result.Violations), result.RetryCount)

	// Convert repository violations to notifications
	for _, v := range result.Violations {
		notification := models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       threadID,
			StepID:         stepID,
			StepName:       stepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationType(v.Type),
			Severity:       models.ViolationSeverity(v.Severity),
			Message:        v.Message,
			Details:        v.Details,
			Timestamp:      time.Now(),
		}

		// Store notification immediately
		if err := s.activityRepo.StoreValidationNotification(ctx, notification); err != nil {
			fmt.Printf("[STORE-ERROR] Error storing notification: %v\n", err)
		}
		notifications = append(notifications, notification)
	}

	// Update final status and critical violation flag from repository result
	status := result.Status
	hasCriticalViolation = result.HasCriticalViolation

	// Handle terminal step completion
	if isTerminal && result.Status == "completed" && !result.HasCriticalViolation {
		fmt.Printf("[TERMINAL-STEP] Thread marked as COMPLETED\n")

		// Create thread completion notification
		threadCompletionNotif := models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       threadID,
			StepID:         stepID,
			StepName:       stepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusCompleted,
			Message:        fmt.Sprintf("Thread completed successfully at terminal step '%s'", stepName),
			Timestamp:      time.Now(),
		}

		// Store thread completion notification
		if err := s.activityRepo.StoreValidationNotification(ctx, threadCompletionNotif); err != nil {
			fmt.Printf("[STORE-ERROR] Error storing thread completion notification: %v\n", err)
		} else {
			fmt.Printf("[THREAD-COMPLETE] Thread completion notification created\n")
		}

		// Archive thread metadata
		s.activityRepo.ArchiveThreadMetadata(ctx, &models.Thread{
			ID:              threadID,
			OwnerID:         thread.OwnerID,
			CompanyID:       thread.CompanyID,
			ContractID:      thread.ContractID,
			ContractName:    thread.ContractName,
			ContractVersion: thread.ContractVersion,
			LastHash:        thread.LastHash,
			StartedAt:       thread.StartedAt,
			CompletedAt:     thread.CompletedAt,
		}, "completed")
	}

	// Archive validation results via ActivityRepository
	if err := s.activityRepo.ArchiveValidationResults(ctx, threadID, stepID, stepName, idempotencyKey, notifications, status, hasCriticalViolation); err != nil {
		fmt.Printf("[ARCHIVE-ERROR] Failed to archive validation results: %v\n", err)
	} else {
		fmt.Printf("[ARCHIVE-SUCCESS] Validation results archived for step=%s\n", stepName)
	}

	// Invalidate cache to force reload on next access
	s.cacheManager.ClearThreadCache(threadID)
}
