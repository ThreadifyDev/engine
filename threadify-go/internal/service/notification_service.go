package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"threadify-go/shared/rbac"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/workerpool"
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
	validationService   *ValidationService
	activityRepo        interfaces.ActivityRepository
	stepStateRepo       interfaces.StepStateRepository
	cacheManager        interfaces.CacheManager
	natsPublisher       NotificationPublisher
	threadAccessService *ThreadAccessService
	rbacLoader          *rbac.Loader
	validationPool      *workerpool.Pool
	notificationPool    *workerpool.Pool
}

// NewNotificationService creates a new notification service
func NewNotificationService(
	validationService *ValidationService,
	activityRepo interfaces.ActivityRepository,
	stepStateRepo interfaces.StepStateRepository,
	cacheManager interfaces.CacheManager,
	natsPublisher NotificationPublisher,
	threadAccessService *ThreadAccessService,
	rbacLoader *rbac.Loader,
	validationPool *workerpool.Pool,
	notificationPool *workerpool.Pool,
) *NotificationService {
	return &NotificationService{
		validationService:   validationService,
		activityRepo:        activityRepo,
		stepStateRepo:       stepStateRepo,
		cacheManager:        cacheManager,
		natsPublisher:       natsPublisher,
		threadAccessService: threadAccessService,
		rbacLoader:          rbacLoader,
		validationPool:      validationPool,
		notificationPool:    notificationPool,
	}
}

// PerformAsyncValidation runs all non-blocking validations via worker pool
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
	// Submit to validation worker pool instead of spawning unbounded goroutines
	// The pool provides timeout via job context
	submitted := s.validationPool.Submit(func(ctx context.Context) {
		fmt.Printf("[ASYNC-VALIDATION] Starting validation for thread=%s, step=%s, stepName=%s\n", threadID, stepID, stepName)

		// Perform all non-blocking validations (contract-specific if graph exists)
		var notifications []models.ValidationNotification

		// If no contract, send execution notification only (no validation possible)
		if graph == nil {
			contractName := ""
			if thread != nil {
				contractName = thread.ContractName
			}

			// Create execution notification based on SDK status
			var message string
			if req.Status == "success" {
				message = "Step '" + stepName + "' completed successfully (no contract)"
			} else if req.Status == "failed" {
				message = "Step '" + stepName + "' failed (no contract)"
			} else if req.Status == "error" {
				message = "Step '" + stepName + "' encountered an error (no contract)"
			} else {
				message = "Step '" + stepName + "' recorded with status '" + req.Status + "' (no contract)"
			}

			executionNotif := models.ValidationNotification{
				NotificationID: uuid.New().String(),
				ThreadID:       threadID,
				StepID:         stepID,
				StepName:       stepName,
				OwnerID:        ownerID,
				ContractName:   contractName,
				StepStatus:     req.Status,
				Status:         "none", // No validation status
				ViolationType:  "",
				Severity:       string(models.SeverityInfo),
				Message:        message,
				Details:        nil, // Use nil for empty map to avoid allocation
				Timestamp:      time.Now(),
			}

			// Publish execution notification via notification worker pool
			if s.natsPublisher != nil {
				s.submitNotificationJob(executionNotif)
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
	})

	if !submitted {
		fmt.Printf("[ASYNC-VALIDATION] Job dropped due to backpressure for thread=%s, step=%s\n", threadID, stepName)
	}
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
	// Pre-allocate with capacity to avoid reallocation (typical: 0-3 violations)
	notifications := make([]models.ValidationNotification, 0, 5)
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

	// Call repository to validate and update atomically with timing
	luaStart := time.Now()
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
		Actor:                  ownerID, // User who recorded this step (for .own permission filtering)
	})

	luaDuration := time.Since(luaStart)
	stepEventID := fmt.Sprintf("%s:%s:%s", threadID, stepName, idempotencyKey)
	log.Printf("[PERF] Validation LUA_SCRIPT: %s | duration=%v | success=%t", stepEventID, luaDuration, err == nil)

	if err != nil {
		fmt.Printf("[REPO-ERROR] Error validating and updating step state: %v\n", err)
		return
	}

	fmt.Printf("[REPO-SUCCESS] Step state updated, status: %s, violations: %d, retry count: %d\n",
		result.Status, len(result.Violations), result.RetryCount)

	// Combine Go and Lua violations
	// Pre-allocate with known size to avoid reallocation
	allViolations := make([]map[string]interface{}, 0, len(notifications)+len(result.Violations))

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
			finalMessage = "Step '" + stepName + "' completed with " + strconv.Itoa(totalViolations) + " violations"
			finalDetails["violations"] = allViolations
		}
	} else {
		finalStatus = "passed"
		finalMessage = "Step '" + stepName + "' completed successfully"
	}

	// Send both execution and validation notifications in a single pass
	executionMessage := "Step '" + stepName + "' execution " + originalStatus
	executionNotif := models.ValidationNotification{
		NotificationID: uuid.New().String(),
		ThreadID:       threadID,
		StepID:         stepID,
		StepName:       stepName,
		OwnerID:        ownerID,
		ContractName:   thread.ContractName,
		StepStatus:     originalStatus,
		Status:         "none", // Execution notification has no validation status
		ViolationType:  "",
		Severity:       string(models.SeverityInfo),
		Message:        executionMessage,
		Details:        nil, // Use nil for empty map to avoid allocation
		Timestamp:      time.Now(),
	}

	validationNotif := models.ValidationNotification{
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

	// Publish both notifications efficiently (single member query)
	// Use background context to avoid cancellation when parent goroutine finishes
	if s.natsPublisher != nil {
		go s.publishDualNotifications(context.Background(), executionNotif, validationNotif)
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
			Message:        "Thread completed successfully at terminal step '" + stepName + "'",
			Details:        nil, // Use nil for empty map to avoid allocation
			Timestamp:      time.Now(),
		}

		// Publish thread completion notification to all authorized members
		if s.natsPublisher != nil {
			go s.publishToAuthorizedMembers(ctx, threadCompletionNotif)
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

// getRequiredPermissionsForNotification determines which permissions are needed
// to receive a notification based on its type, status, and severity
// Returns both wildcard and .own versions so users with either can receive notifications
func getRequiredPermissionsForNotification(
	status string, // "violated", "passed", "none"
	stepStatus string, // "success", "failed", "error"
	severity string, // "critical", "major", "minor", "warning", "info"
	violationType string, // e.g., "step_timeout_exceeded", "retry_limit_exceeded"
) []string {
	// No contract validation - use step status (execution notifications)
	if status == "none" {
		switch stepStatus {
		case "failed", "error":
			// Step execution failed - both wildcard and .own
			return []string{"notification.execution.failed.*", "notification.execution.failed.own"}
		case "success":
			// Step execution succeeded - both wildcard and .own
			return []string{"notification.execution.success.*", "notification.execution.success.own"}
		default:
			return []string{}
		}
	}

	// Contract-based validation notifications
	switch status {
	case "violated":
		// Check for specific violation types (timeout, retry_limit)
		if violationType == "step_timeout_exceeded" {
			return []string{
				"notification.validation.violated.*",
				"notification.validation.violated.timeout.*",
				"notification.validation.violated.timeout.own",
				"notification.validation.violated.own",
			}
		}
		if violationType == "retry_limit_exceeded" {
			return []string{
				"notification.validation.violated.*",
				"notification.validation.violated.retry_limit.*",
				"notification.validation.violated.retry_limit.own",
				"notification.validation.violated.own",
			}
		}

		// General contract violation
		if severity == "critical" {
			// Critical violations: wildcard, critical-specific, or .own
			return []string{
				"notification.validation.violated.*",
				"notification.validation.violated.critical",
				"notification.validation.violated.own",
			}
		}
		// Non-critical violations: wildcard or .own
		return []string{"notification.validation.violated.*", "notification.validation.violated.own"}

	case "passed":
		// Contract validation passed - both wildcard and .own
		return []string{"notification.validation.passed.*", "notification.validation.passed.own"}

	default:
		return []string{}
	}
}

// submitNotificationJob submits a notification publishing job to the notification worker pool
func (s *NotificationService) submitNotificationJob(notification models.ValidationNotification) {
	s.notificationPool.Submit(func(ctx context.Context) {
		s.publishToAuthorizedMembers(ctx, notification)
	})
}

// publishToAuthorizedMembers publishes notification to all thread members with appropriate permissions
// Uses permission-based filtering with .own logic: users with wildcard permissions get all notifications,
// users with .own permissions only get notifications for steps they own
func (s *NotificationService) publishToAuthorizedMembers(
	ctx context.Context,
	notification models.ValidationNotification,
) {
	// Set notification source and type based on status
	if notification.Status == "none" {
		notification.Source = models.NotificationSourceExecution
		notification.NotificationType = fmt.Sprintf("execution.%s", notification.StepStatus)
	} else {
		notification.Source = models.NotificationSourceValidation
		notification.NotificationType = fmt.Sprintf("validation.%s", notification.Status)
	}

	// 1. Determine required permissions for this notification
	requiredPerms := getRequiredPermissionsForNotification(
		notification.Status,
		notification.StepStatus,
		notification.Severity,
		notification.ViolationType,
	)

	if len(requiredPerms) == 0 {
		log.Printf("[NOTIF-SKIP] No permissions required for notification type")
		return
	}

	// 2. Get users with required permissions via ThreadAccessService
	// Returns users with their actual permissions for .own filtering
	users, err := s.threadAccessService.GetUsersByPermissions(
		ctx,
		notification.ThreadID,
		requiredPerms,
	)
	if err != nil {
		log.Printf("[NOTIF-ERROR] Failed to get authorized users: %v", err)
		return
	}

	if len(users) == 0 {
		log.Printf("[NOTIF-SKIP] No users found with required permissions")
		return
	}

	// 3. Filter users based on .own permissions and publish
	publishedCount := 0
	for _, user := range users {
		// Check if user should receive based on their actual permissions
		if !s.shouldReceiveNotification(user.Permissions, requiredPerms, user.UserID, notification.OwnerID) {
			continue
		}

		userNotif := notification
		userNotif.OwnerID = user.UserID

		if err := s.natsPublisher.PublishNotification(ctx, userNotif); err != nil {
			log.Printf("[NOTIF-ERROR] Failed to publish to user %s: %v", user.UserID, err)
			continue
		}
		publishedCount++
	}

	log.Printf("[NOTIF-SUCCESS] Published to %d/%d users with permissions %v", publishedCount, len(users), requiredPerms)
}

// publishDualNotifications sends both execution and validation notifications efficiently
// Queries thread members once and filters for each notification type
func (s *NotificationService) publishDualNotifications(
	ctx context.Context,
	executionNotif models.ValidationNotification,
	validationNotif models.ValidationNotification,
) {
	// Set notification sources
	executionNotif.Source = models.NotificationSourceExecution
	executionNotif.NotificationType = fmt.Sprintf("execution.%s", executionNotif.StepStatus)

	validationNotif.Source = models.NotificationSourceValidation
	validationNotif.NotificationType = fmt.Sprintf("validation.%s", validationNotif.Status)

	// Get permissions for both notification types
	executionPerms := getRequiredPermissionsForNotification(
		executionNotif.Status,
		executionNotif.StepStatus,
		executionNotif.Severity,
		executionNotif.ViolationType,
	)

	validationPerms := getRequiredPermissionsForNotification(
		validationNotif.Status,
		validationNotif.StepStatus,
		validationNotif.Severity,
		validationNotif.ViolationType,
	)

	if len(executionPerms) == 0 && len(validationPerms) == 0 {
		log.Printf("[NOTIF-SKIP] No permissions required for either notification")
		return
	}

	// Combine permissions to get all users who need at least one notification
	allPerms := append(executionPerms, validationPerms...)

	// Get users with any of the required permissions (single query)
	users, err := s.threadAccessService.GetUsersByPermissions(
		ctx,
		executionNotif.ThreadID,
		allPerms,
	)
	if err != nil {
		log.Printf("[NOTIF-ERROR] Failed to get authorized users: %v", err)
		return
	}

	if len(users) == 0 {
		log.Printf("[NOTIF-SKIP] No users found with required permissions")
		return
	}

	// For each user, check which notifications they should receive
	executionCount := 0
	validationCount := 0

	for _, user := range users {
		// Check execution notification
		if len(executionPerms) > 0 && s.shouldReceiveNotification(user.Permissions, executionPerms, user.UserID, executionNotif.OwnerID) {
			userExecNotif := executionNotif
			userExecNotif.OwnerID = user.UserID

			if err := s.natsPublisher.PublishNotification(ctx, userExecNotif); err != nil {
				log.Printf("[NOTIF-ERROR] Failed to publish execution notif to %s: %v", user.UserID, err)
			} else {
				executionCount++
			}
		}

		// Check validation notification
		if len(validationPerms) > 0 && s.shouldReceiveNotification(user.Permissions, validationPerms, user.UserID, validationNotif.OwnerID) {
			userValNotif := validationNotif
			userValNotif.OwnerID = user.UserID

			if err := s.natsPublisher.PublishNotification(ctx, userValNotif); err != nil {
				log.Printf("[NOTIF-ERROR] Failed to publish validation notif to %s: %v", user.UserID, err)
			} else {
				validationCount++
			}
		}
	}

	log.Printf("[NOTIF-SUCCESS] Published execution:%d validation:%d to %d users", executionCount, validationCount, len(users))
}

// shouldReceiveNotification checks if a user should receive a notification based on their actual permissions
// Handles .own permissions: user must own the step to receive .own-only notifications
func (s *NotificationService) shouldReceiveNotification(
	userPerms []string,
	requiredPerms []string,
	userID string,
	stepOwnerID string,
) bool {
	// Check if user has any matching permission
	for _, userPerm := range userPerms {
		for _, reqPerm := range requiredPerms {
			// Exact match (e.g., "notification.violations.critical")
			if userPerm == reqPerm {
				return true
			}

			// Wildcard match (e.g., user has "notification.violations.*", req is "notification.violations.own")
			if strings.HasSuffix(userPerm, ".*") {
				prefix := strings.TrimSuffix(userPerm, ".*")
				if strings.HasPrefix(reqPerm, prefix) {
					return true
				}
			}

			// .own match (user has .own permission and is the owner)
			if strings.HasSuffix(userPerm, ".own") && userID == stepOwnerID {
				// Check if .own permission matches required permission base
				ownBase := strings.TrimSuffix(userPerm, ".own")
				reqBase := strings.TrimSuffix(reqPerm, ".*")
				reqBaseOwn := strings.TrimSuffix(reqPerm, ".own")
				if ownBase == reqBase || ownBase == reqBaseOwn {
					return true
				}
			}
		}
	}

	return false
}
