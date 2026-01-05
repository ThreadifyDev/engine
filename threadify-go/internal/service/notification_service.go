package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	luaScripts        *LuaScriptManager
	cacheManager      interfaces.CacheManager
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	validationService *ValidationService,
	activityRepo interfaces.ActivityRepository,
	luaScripts *LuaScriptManager,
	cacheManager interfaces.CacheManager,
) *NotificationService {
	return &NotificationService{
		validationService: validationService,
		activityRepo:      activityRepo,
		luaScripts:        luaScripts,
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

	// 1. Invalid Transition (Critical)
	if violation := s.validationService.CheckInvalidTransition(ctx, req.ThreadID, req.StepName, graph); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationInvalidTransition,
			Severity:       models.SeverityCritical,
			Message:        violation.Message,
			FromStep:       violation.FromStep,
			ToStep:         violation.ToStep,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 2. Step Timeout Exceeded (Critical)
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

	// 3. Max Duration Exceeded (Critical)
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

	// 4. Multiple Terminal States (Critical/Configurable)
	if violation := s.validationService.CheckMultipleTerminalStates(ctx, req.ThreadID, req.StepName, graph); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationMultipleTerminalStates,
			Severity:       violation.Severity,
			Message:        violation.Message,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 5. Retry Limit Exceeded (Critical) - Now handled by Lua script atomically
	// The Lua script will check retry count and add violation if exceeded

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

	// 7. Extra Undocumented Fields (Info)
	if violation := s.validationService.CheckExtraFields(stepNode, req.Context); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusCompleted,
			ViolationType:  models.ViolationExtraUndocumentedField,
			Severity:       models.SeverityInfo,
			Message:        violation.Message,
			ExtraFields:    violation.ExtraFields,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 8. Step Completed Successfully (Info) - Only add if no critical violations
	hasCriticalViolations := false
	for _, notif := range notifications {
		if notif.Severity == models.SeverityCritical {
			hasCriticalViolations = true
			break
		}
	}

	if !hasCriticalViolations {
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

	// Determine final step status and violation JSON
	// Start with the original step status from the request (success, failed, error, etc.)
	status := originalStatus
	if status == "success" {
		status = "completed" // Map success to completed for internal state
	}

	violationJSON := ""
	if hasCriticalViolation {
		status = "violated"
		violationData, _ := json.Marshal(criticalViolations)
		violationJSON = string(violationData)
		fmt.Printf("[PROCESS-NOTIFICATIONS] Marking step as VIOLATED due to %d critical violations\n", len(criticalViolations))
	} else {
		fmt.Printf("[PROCESS-NOTIFICATIONS] Marking step as %s (no critical violations)\n", strings.ToUpper(status))
	}

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

	// Get maxRetries from contract graph for retry limit validation in Lua
	maxRetries := 0
	if graph != nil {
		for _, transition := range graph.Transitions {
			if transition.From == stepName && transition.MaxRetries > 0 {
				maxRetries = transition.MaxRetries
				break
			}
		}
	}

	// Update step state via Lua script (atomic operation)
	// Lua script will handle retry limit validation atomically
	if s.luaScripts != nil {
		luaResponse, err := s.luaScripts.UpdateStepState(
			ctx,
			threadID,
			stepID,
			stepName,
			idempotencyKey,
			status,
			violationJSON,
			isTerminal,
			time.Now().Format(time.RFC3339),
			maxRetries,
		)
		if err != nil {
			fmt.Printf("[LUA-ERROR] Error updating step state via Lua: %v\n", err)
		} else {
			// Parse Lua response: "status" or "status|retry_violated"
			parts := strings.Split(luaResponse, "|")
			finalStatus := parts[0]
			retryViolated := len(parts) > 1 && parts[1] == "retry_violated"

			fmt.Printf("[LUA-SUCCESS] Step state updated, final status: %s, retry violated: %v\n", finalStatus, retryViolated)

			// If Lua detected retry limit violation, add it to notifications
			if retryViolated {
				fmt.Printf("[CRITICAL-VIOLATION] Retry limit exceeded detected by Lua script\n")

				// Get current retry count from Redis for the violation message
				stepHashKey := fmt.Sprintf("thread:%s:steps:%s:%s", threadID, stepName, idempotencyKey)
				retryCountStr, _ := s.validationService.valkeyClient.HGet(ctx, stepHashKey, "retryCount")
				retryCount := 0
				fmt.Sscanf(retryCountStr, "%d", &retryCount)

				notifications = append(notifications, models.ValidationNotification{
					NotificationID: uuid.New().String(),
					ThreadID:       threadID,
					StepID:         stepID,
					StepName:       stepName,
					OwnerID:        ownerID,
					Status:         models.NotificationStatusFailed,
					ViolationType:  models.ViolationRetryLimitExceeded,
					Severity:       models.SeverityCritical,
					Message:        fmt.Sprintf("Step '%s' exceeded retry limit of %d (current: %d retries)", stepName, maxRetries, retryCount),
					Details: map[string]interface{}{
						"step_name":   stepName,
						"retry_count": retryCount,
						"max_retries": maxRetries,
					},
					Timestamp: time.Now(),
				})

				// Update hasCriticalViolation flag
				hasCriticalViolation = true
			}

			// Note: Lua script now writes step state change to activity log stream atomically

			if isTerminal && finalStatus == "completed" {
				fmt.Printf("[TERMINAL-STEP] Thread marked as COMPLETED\n")
				// Archive completed thread metadata via ActivityRepository
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
		}
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
