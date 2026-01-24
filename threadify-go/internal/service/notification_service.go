package service

import (
	"context"
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

// NotificationPublisher defines the interface for publishing notifications
type NotificationPublisher interface {
	PublishNotification(ctx context.Context, notification models.ValidationNotification) error
}

// NotificationService handles async validation processing and coordination
type NotificationService struct {
	validationService *ValidationService
	activityRepo      interfaces.ActivityRepository
	stepStateRepo     interfaces.StepStateRepository
	cacheManager      interfaces.CacheManager
	natsPublisher     NotificationPublisher
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	validationService *ValidationService,
	activityRepo interfaces.ActivityRepository,
	stepStateRepo interfaces.StepStateRepository,
	cacheManager interfaces.CacheManager,
	natsPublisher NotificationPublisher,
) *NotificationService {
	return &NotificationService{
		validationService: validationService,
		activityRepo:      activityRepo,
		stepStateRepo:     stepStateRepo,
		cacheManager:      cacheManager,
		natsPublisher:     natsPublisher,
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
		// Use configurable timeout to prevent goroutine leaks
		// Default: 60 seconds (from config.Timeouts.ValidationSeconds)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		fmt.Printf("[ASYNC-VALIDATION] Starting validation for thread=%s, step=%s, stepName=%s (timeout: 60s)\n", threadID, stepID, stepName)

		// Perform all non-blocking validations (contract-specific if graph exists)
		var notifications []models.ValidationNotification

		// If no contract, send immediate success notification (can't validate)
		if graph == nil {
			var notifStatus, message string
			contractName := ""

			// Get contract name from thread if available
			if thread != nil {
				contractName = thread.ContractName
			}

			if req.Status == "success" {
				notifStatus = "passed"
				if contractName != "" {
					message = fmt.Sprintf("Step '%s' completed successfully", stepName)
				} else {
					message = fmt.Sprintf("Step '%s' completed successfully (no contract)", stepName)
				}
			} else if req.Status == "failed" {
				notifStatus = "failed"
				if contractName != "" {
					message = fmt.Sprintf("Step '%s' failed", stepName)
				} else {
					message = fmt.Sprintf("Step '%s' failed (no contract)", stepName)
				}
			} else if req.Status == "error" {
				notifStatus = "error"
				if contractName != "" {
					message = fmt.Sprintf("Step '%s' encountered an error", stepName)
				} else {
					message = fmt.Sprintf("Step '%s' encountered an error (no contract)", stepName)
				}
			} else {
				notifStatus = "none"
				if contractName != "" {
					message = fmt.Sprintf("Step '%s' recorded with status '%s'", stepName, req.Status)
				} else {
					message = fmt.Sprintf("Step '%s' recorded with status '%s' (no contract)", stepName, req.Status)
				}
			}

			immediateNotif := models.ValidationNotification{
				NotificationID: uuid.New().String(),
				ThreadID:       threadID,
				StepID:         stepID,
				StepName:       stepName,
				OwnerID:        ownerID,
				ContractName:   contractName,
				StepStatus:     req.Status,
				Status:         notifStatus,
				ViolationType:  "",
				Severity:       string(models.SeverityInfo),
				Message:        message,
				Details:        make(map[string]interface{}),
				Timestamp:      time.Now(),
			}

			// Publish immediately (async, non-blocking)
			if s.natsPublisher != nil {
				go func(notif models.ValidationNotification) {
					if err := s.natsPublisher.PublishNotification(ctx, notif); err != nil {
						fmt.Printf("[NATS-ERROR] Error publishing immediate notification: %v\n", err)
					} else {
						fmt.Printf("[IMMEDIATE-NOTIF] Published for step=%s, status=%s (no contract)\n",
							stepName, req.Status)
					}
				}(immediateNotif)
			}

			// Archive step state even for threads without contracts
			idempKey := req.IdempotencyKey
			if idempKey == "" {
				idempKey = stepID
			}

			now := time.Now().Format(time.RFC3339)
			stepStateSnapshot := &interfaces.StepStateSnapshot{
				ID:             stepID,
				ThreadID:       threadID,
				StepName:       stepName,
				IdempotencyKey: idempKey,
				Status:         req.Status,
				RetryCount:     0, // No retry tracking without contract
				FirstSeenAt:    now,
				LastUpdatedAt:  now,
				PreviousStep:   "",
			}

			if err := s.activityRepo.ArchiveStepState(ctx, stepStateSnapshot); err != nil {
				fmt.Printf("[ARCHIVE-ERROR] Failed to archive step state (no contract): %v\n", err)
			}

			// Return early - no need to process validations
			fmt.Printf("[ASYNC-VALIDATION] Completed validation for thread=%s (no contract)\n", threadID)
			return
		}

		// Perform validations (graph is guaranteed to be non-nil here)
		notifications = s.performNonBlockingValidations(
			ctx,
			thread,
			req,
			stepNode,
			graph,
			stepID,
			ownerID,
		)

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
		details := violation.Details
		if details == nil {
			details = make(map[string]interface{})
		}
		details["duration"] = violation.Duration
		details["limit"] = violation.Limit

		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			ContractName:   thread.ContractName,
			StepStatus:     req.Status,
			Status:         "violated",
			ViolationType:  string(models.ViolationStepTimeoutExceeded),
			Severity:       string(models.SeverityCritical),
			Message:        violation.Message,
			Details:        details,
			Timestamp:      now,
		})
	}

	// 2. Max Duration Exceeded (Critical)
	if violation := s.validationService.CheckMaxDuration(thread, graph); violation != nil {
		details := violation.Details
		if details == nil {
			details = make(map[string]interface{})
		}
		details["duration"] = violation.Duration
		details["limit"] = violation.Limit

		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			ContractName:   thread.ContractName,
			StepStatus:     req.Status,
			Status:         "violated",
			ViolationType:  string(models.ViolationMaxDurationExceeded),
			Severity:       string(models.SeverityCritical),
			Message:        violation.Message,
			Details:        details,
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
			details := violation.Details
			if details == nil {
				details = make(map[string]interface{})
			}
			// Convert array to comma-separated string to avoid JSON parsing issues in Lua
			details["missingFields"] = strings.Join(violation.MissingFields, ",")

			notifications = append(notifications, models.ValidationNotification{
				NotificationID: uuid.New().String(),
				ThreadID:       req.ThreadID,
				StepID:         stepID,
				StepName:       req.StepName,
				OwnerID:        ownerID,
				ContractName:   thread.ContractName,
				StepStatus:     req.Status,
				Status:         "violated",
				ViolationType:  string(models.ViolationMissingOptionalField),
				Severity:       string(models.SeverityInfo),
				Message:        violation.Message,
				Details:        details,
				Timestamp:      now,
			})
		}
	}

	// Return only violation notifications; Lua script determines final status
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
	// Don't publish Go violations individually - will be included in final notification
	fmt.Printf("[PROCESS-NOTIFICATIONS] Processing %d Go violations for thread=%s, step=%s\n", len(notifications), threadID, stepName)

	// Check for critical violations
	hasCriticalViolation := false
	var criticalViolations []models.StepViolation

	for _, notif := range notifications {
		if notif.Severity == string(models.SeverityCritical) {
			fmt.Printf("[CRITICAL-VIOLATION] Found violation: type=%s, message=%s\n", notif.ViolationType, notif.Message)
			hasCriticalViolation = true
			criticalViolations = append(criticalViolations, models.StepViolation{
				StepID:        stepID,
				StepName:      stepName,
				OwnerID:       ownerID,
				ViolationType: models.ViolationType(notif.ViolationType),
				Severity:      models.ViolationSeverity(notif.Severity),
				Message:       notif.Message,
				Details:       notif.Details,
				ViolatedAt:    notif.Timestamp,
			})
		}
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
	transitionsMap := make(map[string][]string)
	terminalSteps := []string{}
	allowMultipleTerminals := false

	if graph != nil {
		// Build transitions map: stepName -> allowed next steps
		for _, transition := range graph.Transitions {
			transitionsMap[transition.From] = transition.To

			// Also get max retries for current step
			if transition.From == stepName && transition.MaxRetries > 0 {
				maxRetries = transition.MaxRetries
			}
		}

		// Get terminal steps
		terminalSteps = graph.Graph.TerminalSteps

		// Check if multiple terminals allowed
		if graph.Validation != nil {
			allowMultipleTerminals = graph.Validation.AllowMultipleTerminals
		}
	}

	// DEBUG: Log what we're passing to Lua
	fmt.Printf("[DEBUG-LUA-PARAMS] thread=%s, step=%s, transitionsMap=%v, maxRetries=%d\n",
		threadID, stepName, transitionsMap, maxRetries)

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
		TransitionsMap:         transitionsMap,
		TerminalSteps:          terminalSteps,
		AllowMultipleTerminals: allowMultipleTerminals,
	})

	if err != nil {
		fmt.Printf("[REPO-ERROR] Error validating and updating step state: %v\n", err)
		return
	}

	fmt.Printf("[REPO-SUCCESS] Step state updated, status: %s, violations: %d, retry count: %d\n",
		result.Status, len(result.Violations), result.RetryCount)

	// Combine Go and Lua violations
	var allViolations []map[string]interface{}

	// Add Go violations
	for _, notif := range notifications {
		violationData := map[string]interface{}{
			"type":     notif.ViolationType,
			"severity": notif.Severity,
			"message":  notif.Message,
			"details":  notif.Details,
		}
		allViolations = append(allViolations, violationData)
	}

	// Add Lua violations
	for _, v := range result.Violations {
		violationData := map[string]interface{}{
			"type":     v.Type,
			"severity": v.Severity,
			"message":  v.Message,
			"details":  v.Details,
		}
		allViolations = append(allViolations, violationData)
	}

	// Update final status and critical violation flag from repository result
	status := result.Status
	hasCriticalViolation = result.HasCriticalViolation || hasCriticalViolation // Combine Go + Lua critical flags

	// Send final status notification based on overall validation result
	finalDetails := make(map[string]interface{})
	var finalStatus, finalMessage, violationType, severity string

	totalViolations := len(allViolations)
	if totalViolations > 0 {
		finalStatus = "violated"

		// If there's exactly one violation, use its details directly
		if totalViolations == 1 {
			violation := allViolations[0]
			violationType = violation["type"].(string)
			severity = violation["severity"].(string)
			finalMessage = violation["message"].(string)
			// Merge violation details into top level
			if details, ok := violation["details"].(map[string]interface{}); ok {
				for k, v := range details {
					finalDetails[k] = v
				}
			}
		} else {
			// Multiple violations - include all in details
			finalMessage = fmt.Sprintf("Step '%s' completed with %d violations", stepName, totalViolations)
			finalDetails["violations"] = allViolations
		}
	} else {
		finalStatus = "passed"
		finalMessage = fmt.Sprintf("Step '%s' completed successfully", stepName)
	}

	finalStatusNotif := models.ValidationNotification{
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
		Timestamp:      time.Now(),
	}

	// Publish final status notification (async, non-blocking)
	if s.natsPublisher != nil {
		go func(notif models.ValidationNotification) {
			if err := s.natsPublisher.PublishNotification(ctx, notif); err != nil {
				fmt.Printf("[NATS-ERROR] Error publishing final status notification: %v\n", err)
			}
		}(finalStatusNotif)
	}

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
			ContractName:   thread.ContractName,
			StepStatus:     originalStatus,
			Status:         "passed",
			ViolationType:  "",
			Severity:       "",
			Message:        fmt.Sprintf("Thread completed successfully at terminal step '%s'", stepName),
			Details:        make(map[string]interface{}),
			Timestamp:      time.Now(),
		}

		// Publish thread completion notification to NATS (async, non-blocking)
		if s.natsPublisher != nil {
			go func(notif models.ValidationNotification) {
				if err := s.natsPublisher.PublishNotification(ctx, notif); err != nil {
					fmt.Printf("[NATS-ERROR] Error publishing thread completion notification: %v\n", err)
				} else {
					fmt.Printf("[THREAD-COMPLETE] Thread completion notification published\n")
				}
			}(threadCompletionNotif)
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

	now := time.Now().Format(time.RFC3339)

	// Use firstSeenAt from Redis if available, otherwise use current time
	firstSeenAt := result.FirstSeenAt
	if firstSeenAt == "" {
		firstSeenAt = now
	}

	// Extract step name from previousStep key (format: stepName:idempKey)
	previousStepName := ""
	if result.PreviousStep != "" {
		// Extract just the step name part before the colon
		if idx := strings.Index(result.PreviousStep, ":"); idx > 0 {
			previousStepName = result.PreviousStep[:idx]
		}
	}

	stepStateSnapshot := &interfaces.StepStateSnapshot{
		ID:             stepID,
		ThreadID:       threadID,
		StepName:       stepName,
		IdempotencyKey: idempotencyKey,
		Status:         result.Status,
		RetryCount:     result.RetryCount,
		FirstSeenAt:    firstSeenAt,
		LastUpdatedAt:  now,
		PreviousStep:   previousStepName,
	}

	if err := s.activityRepo.ArchiveStepState(ctx, stepStateSnapshot); err != nil {
		fmt.Printf("[ARCHIVE-ERROR] Failed to archive step state: %v\n", err)
	} else {
		fmt.Printf("[ARCHIVE-SUCCESS] Step state archived for step=%s\n", stepName)
	}

	// Invalidate cache to force reload on next access
	s.cacheManager.ClearThreadCache(threadID)
}
