package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/threadify/engine/internal/models"
)

// getCurrentSteps retrieves current step names from the sorted set
func (s *ThreadService) getCurrentSteps(ctx context.Context, threadID string) []string {
	currentStepsKey := fmt.Sprintf("thread:%s:current_steps", threadID)

	// Get all members from sorted set (ordered by score/timestamp)
	steps, err := s.valkeyClient.ZRange(ctx, currentStepsKey, 0, -1)
	if err != nil {
		return []string{}
	}

	// Extract step names from stepKey format (stepName:idempKey)
	stepNames := make([]string, 0, len(steps))
	for _, stepKey := range steps {
		// Split by : to get stepName
		parts := strings.Split(stepKey, ":")
		if len(parts) > 0 {
			stepNames = append(stepNames, parts[0])
		}
	}

	return stepNames
}

// checkInvalidTransition validates if the transition from current step to new step is allowed
// Uses current_steps sorted set to determine the previous step
func (s *ThreadService) checkInvalidTransition(
	ctx context.Context,
	threadID string,
	currentStepName string,
	graph *models.ContractGraph,
) *models.ValidationViolation {
	// Get current steps from sorted set
	currentSteps := s.getCurrentSteps(ctx, threadID)

	// If no current steps, this is the first step (entry point already validated in blocking)
	if len(currentSteps) == 0 {
		return nil
	}

	// Get the last current step (most recent)
	previousStep := currentSteps[len(currentSteps)-1]

	// Find transition from previous step
	var transition *models.Transition
	for i := range graph.Transitions {
		if graph.Transitions[i].From == previousStep {
			transition = &graph.Transitions[i]
			break
		}
	}

	if transition == nil {
		return &models.ValidationViolation{
			Message:  fmt.Sprintf("No transitions defined from step '%s'", previousStep),
			FromStep: previousStep,
			ToStep:   currentStepName,
			Details: map[string]interface{}{
				"previous_step": previousStep,
				"current_step":  currentStepName,
			},
		}
	}

	// Check if current step is in allowed transitions
	for _, allowedStep := range transition.To {
		if allowedStep == currentStepName {
			return nil // Valid transition
		}
	}

	return &models.ValidationViolation{
		Message:  fmt.Sprintf("Invalid transition from '%s' to '%s'", previousStep, currentStepName),
		FromStep: previousStep,
		ToStep:   currentStepName,
		Details: map[string]interface{}{
			"previous_step": previousStep,
			"current_step":  currentStepName,
			"allowed_steps": transition.To,
		},
	}
}

// checkStepTimeout validates if the step execution time exceeded the timeout
func checkStepTimeout(
	stepNode models.GraphNode,
	startedAt string,
	finishedAt string,
) *models.ValidationViolation {
	// No timeout defined
	if stepNode.Timeout == "" {
		return nil
	}

	// Parse timeout duration
	timeout, err := time.ParseDuration(stepNode.Timeout)
	if err != nil {
		return nil // Invalid timeout format, skip validation
	}

	// Parse timestamps
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return nil
	}

	finish, err := time.Parse(time.RFC3339, finishedAt)
	if err != nil {
		return nil
	}

	// Calculate actual duration
	duration := finish.Sub(start)

	if duration > timeout {
		return &models.ValidationViolation{
			Message:  fmt.Sprintf("Step exceeded timeout of %s (took %s)", stepNode.Timeout, duration),
			Duration: duration.String(),
			Limit:    stepNode.Timeout,
			Details: map[string]interface{}{
				"timeout":  stepNode.Timeout,
				"duration": duration.String(),
			},
		}
	}

	return nil
}

// checkMaxDuration validates if the thread duration exceeded the max duration
func checkMaxDuration(
	thread *models.Thread,
	graph *models.ContractGraph,
) *models.ValidationViolation {
	// No validation rules or max duration defined
	if graph.Validation == nil || graph.Validation.MaxDuration == "" {
		return nil
	}

	// Parse max duration
	maxDuration, err := time.ParseDuration(graph.Validation.MaxDuration)
	if err != nil {
		return nil // Invalid format, skip validation
	}

	// Calculate elapsed time
	elapsed := time.Since(thread.StartedAt)

	if elapsed > maxDuration {
		return &models.ValidationViolation{
			Message:  fmt.Sprintf("Thread exceeded maximum duration of %s (running for %s)", graph.Validation.MaxDuration, elapsed),
			Duration: elapsed.String(),
			Limit:    graph.Validation.MaxDuration,
			Details: map[string]interface{}{
				"thread_started_at": thread.StartedAt,
				"elapsed":           elapsed.String(),
				"max_duration":      graph.Validation.MaxDuration,
			},
		}
	}

	return nil
}

// checkMultipleTerminalStates validates if thread reached multiple terminal states
func (s *ThreadService) checkMultipleTerminalStates(
	ctx context.Context,
	threadID string,
	currentStepName string,
	graph *models.ContractGraph,
) *models.ValidationViolation {
	// Check if current step is a terminal step
	isCurrentTerminal := false
	for _, terminalStep := range graph.Graph.TerminalSteps {
		if terminalStep == currentStepName {
			isCurrentTerminal = true
			break
		}
	}

	if !isCurrentTerminal {
		return nil // Current step is not terminal
	}

	// Get current steps from sorted set
	currentSteps := s.getCurrentSteps(ctx, threadID)

	// Count how many terminal steps are already in current steps
	terminalCount := 0
	for _, currentStep := range currentSteps {
		for _, terminalStep := range graph.Graph.TerminalSteps {
			if currentStep == terminalStep {
				terminalCount++
				break
			}
		}
	}

	// If we already have a terminal step and this is another one
	if terminalCount > 0 {
		// Check if multiple terminals are allowed
		if graph.Validation != nil && graph.Validation.AllowMultipleTerminals {
			return nil // Allowed
		}

		// Determine severity
		severity := models.SeverityCritical
		if graph.Validation != nil && graph.Validation.MultipleTerminalsSeverity != "" {
			switch graph.Validation.MultipleTerminalsSeverity {
			case "warning":
				severity = models.SeverityWarning
			case "minor":
				severity = models.SeverityMinor
			case "info":
				severity = models.SeverityInfo
			}
		}

		return &models.ValidationViolation{
			Message:  fmt.Sprintf("Thread reached multiple terminal states (already at terminal, attempting '%s')", currentStepName),
			Severity: severity,
			Details: map[string]interface{}{
				"current_terminal_steps": currentSteps,
				"new_terminal_step":      currentStepName,
			},
		}
	}

	return nil
}

// checkRetryLimit validates if the step retry count exceeded the max retries
func (s *ThreadService) checkRetryLimit(
	ctx context.Context,
	threadID string,
	stepName string,
	idempotencyKey string,
	graph *models.ContractGraph,
) *models.ValidationViolation {
	// Find the transition that allows retries for this step
	// Retry configuration is on the transition FROM the step
	var maxRetries int

	for _, transition := range graph.Transitions {
		// Check if the "from" step matches our current step
		// If MaxRetries is set, retries are enabled (CanRetry is optional)
		if transition.From == stepName && (transition.CanRetry || transition.MaxRetries > 0) {
			maxRetries = transition.MaxRetries
			fmt.Printf("[RETRY-CHECK] Found retry config for step=%s: max_retries=%d, canRetry=%v\n", stepName, maxRetries, transition.CanRetry)
			break
		}
	}

	// If retries are not configured for this step, skip validation
	if maxRetries == 0 {
		fmt.Printf("[RETRY-CHECK] No retry config for step=%s (maxRetries=%d)\n", stepName, maxRetries)
		return nil
	}

	// Get step state hash to check retry count
	stepHashKey := fmt.Sprintf("thread:%s:steps:%s:%s", threadID, stepName, idempotencyKey)
	fmt.Printf("[RETRY-CHECK] Checking key: %s\n", stepHashKey)
	retryCountStr, err := s.valkeyClient.HGet(ctx, stepHashKey, "retryCount")
	if err != nil {
		fmt.Printf("[RETRY-CHECK] Error getting retry count: %v\n", err)
		return nil // Step state not found, skip validation
	}

	// Parse retry count
	var retryCount int
	if _, err := fmt.Sscanf(retryCountStr, "%d", &retryCount); err != nil {
		fmt.Printf("[RETRY-CHECK] Error parsing retry count '%s': %v\n", retryCountStr, err)
		return nil // Invalid retry count format
	}

	fmt.Printf("[RETRY-CHECK] step=%s, retryCount=%d, maxRetries=%d\n", stepName, retryCount, maxRetries)

	// Check if retry limit exceeded
	// Note: We check >= because this validation runs BEFORE the Lua script increments the count
	// So if retryCount is already at maxRetries, the next increment will exceed it
	if retryCount >= maxRetries {
		fmt.Printf("[RETRY-CHECK] VIOLATION: Retry limit exceeded!\n")
		return &models.ValidationViolation{
			Message: fmt.Sprintf("Step '%s' exceeded retry limit of %d (current: %d retries, next would be %d)", stepName, maxRetries, retryCount, retryCount+1),
			Details: map[string]interface{}{
				"step_name":   stepName,
				"retry_count": retryCount,
				"max_retries": maxRetries,
			},
		}
	}

	fmt.Printf("[RETRY-CHECK] No violation (within limit)\n")
	return nil
}

// checkMissingOptionalFields checks for missing optional business context fields
func checkMissingOptionalFields(
	stepNode models.GraphNode,
	context map[string]string,
) *models.ValidationViolation {
	// No business context defined
	if stepNode.BusinessContext == nil {
		return nil
	}

	// Type assert to BusinessContext
	bc, ok := stepNode.BusinessContext.(*models.BusinessContext)
	if !ok {
		return nil
	}

	// No optional fields defined
	if len(bc.Optional) == 0 {
		return nil
	}

	// Check which optional fields are missing
	var missingFields []string
	for _, optField := range bc.Optional {
		if _, exists := context[optField]; !exists {
			missingFields = append(missingFields, optField)
		}
	}

	if len(missingFields) > 0 {
		return &models.ValidationViolation{
			Message:       fmt.Sprintf("Missing %d optional field(s)", len(missingFields)),
			MissingFields: missingFields,
			Details: map[string]interface{}{
				"missing_optional_fields": missingFields,
			},
		}
	}

	return nil
}

// checkExtraFields checks for fields not defined in business context
func checkExtraFields(
	stepNode models.GraphNode,
	context map[string]string,
) *models.ValidationViolation {
	// No business context defined - all fields are allowed
	if stepNode.BusinessContext == nil {
		return nil
	}

	// Type assert to BusinessContext
	bc, ok := stepNode.BusinessContext.(*models.BusinessContext)
	if !ok {
		return nil
	}

	// Build set of allowed fields
	allowedFields := make(map[string]bool)
	for _, field := range bc.Required {
		allowedFields[field] = true
	}
	for _, field := range bc.Optional {
		allowedFields[field] = true
	}

	// Check for extra fields
	var extraFields []string
	for field := range context {
		if !allowedFields[field] {
			extraFields = append(extraFields, field)
		}
	}

	if len(extraFields) > 0 {
		return &models.ValidationViolation{
			Message:     fmt.Sprintf("Found %d undocumented field(s)", len(extraFields)),
			ExtraFields: extraFields,
			Details: map[string]interface{}{
				"extra_fields": extraFields,
			},
		}
	}

	return nil
}

// performAsyncValidation runs all non-blocking validations in a goroutine
// Called from HandleRecordEvent after step is successfully recorded
func (s *ThreadService) performAsyncValidation(
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
		s.processValidationNotifications(ctx, threadID, stepID, stepName, ownerID, idempKey, notifications, graph)

		fmt.Printf("[ASYNC-VALIDATION] Completed validation for thread=%s\n", threadID)
	}()
}

// performNonBlockingValidations executes all validation checks
func (s *ThreadService) performNonBlockingValidations(
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
	if violation := s.checkInvalidTransition(ctx, req.ThreadID, req.StepName, graph); violation != nil {
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
	if violation := checkStepTimeout(stepNode, req.StartedAt, req.FinishedAt); violation != nil {
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
	if violation := checkMaxDuration(thread, graph); violation != nil {
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
	if violation := s.checkMultipleTerminalStates(ctx, req.ThreadID, req.StepName, graph); violation != nil {
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

	// 5. Retry Limit Exceeded (Critical)
	// Use stepID as idempotency key if not provided
	idempKey := req.IdempotencyKey
	if idempKey == "" {
		idempKey = stepID
	}
	if violation := s.checkRetryLimit(ctx, req.ThreadID, req.StepName, idempKey, graph); violation != nil {
		notifications = append(notifications, models.ValidationNotification{
			NotificationID: uuid.New().String(),
			ThreadID:       req.ThreadID,
			StepID:         stepID,
			StepName:       req.StepName,
			OwnerID:        ownerID,
			Status:         models.NotificationStatusFailed,
			ViolationType:  models.ViolationRetryLimitExceeded,
			Severity:       models.SeverityCritical,
			Message:        violation.Message,
			Details:        violation.Details,
			Timestamp:      now,
		})
	}

	// 6. Missing Optional Fields (Info)
	if violation := checkMissingOptionalFields(stepNode, req.Context); violation != nil {
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
	if violation := checkExtraFields(stepNode, req.Context); violation != nil {
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
func (s *ThreadService) processValidationNotifications(
	ctx context.Context,
	threadID string,
	stepID string,
	stepName string,
	ownerID string,
	idempotencyKey string,
	notifications []models.ValidationNotification,
	graph *models.ContractGraph,
) {
	// Store all notifications in stream
	for _, notif := range notifications {
		if err := s.storeNotificationInStream(ctx, notif); err != nil {
			fmt.Printf("Error storing notification: %v\n", err)
		}
	}

	fmt.Printf("[PROCESS-NOTIFICATIONS] Processing %d notifications for thread=%s, step=%s\n", len(notifications), threadID, stepID)

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
	status := "completed"
	violationJSON := ""
	if hasCriticalViolation {
		status = "violated"
		violationData, _ := json.Marshal(criticalViolations)
		violationJSON = string(violationData)
		fmt.Printf("[PROCESS-NOTIFICATIONS] Marking step as VIOLATED due to %d critical violations\n", len(criticalViolations))
	} else {
		fmt.Printf("[PROCESS-NOTIFICATIONS] Marking step as COMPLETED (no critical violations)\n")
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

	// Update step state via Lua script (atomic operation)
	if s.luaScripts != nil {
		finalStatus, err := s.luaScripts.UpdateStepState(
			ctx,
			threadID,
			stepID,
			stepName,
			idempotencyKey,
			status,
			violationJSON,
			isTerminal,
			time.Now().Format(time.RFC3339),
		)
		if err != nil {
			fmt.Printf("[LUA-ERROR] Error updating step state via Lua: %v\n", err)
		} else {
			fmt.Printf("[LUA-SUCCESS] Step state updated, final status: %s\n", finalStatus)
			if isTerminal && status == "completed" {
				fmt.Printf("[TERMINAL-STEP] Thread marked as COMPLETED\n")
				// Archive completed thread metadata
				s.archiveThreadMetadata(ctx, threadID, "completed")
			}
		}
	}

	// Archive validation results to persistent storage
	if err := s.archiveValidationResults(ctx, threadID, stepID, stepName, idempotencyKey, notifications, status, hasCriticalViolation); err != nil {
		fmt.Printf("[ARCHIVE-ERROR] Failed to archive validation results: %v\n", err)
	} else {
		fmt.Printf("[ARCHIVE-SUCCESS] Validation results archived for step=%s\n", stepID)
	}

	// Invalidate cache to force reload on next access
	s.cacheManager.ClearThreadCache(threadID)
}

// storeNotificationInStream stores validation notification in Valkey stream
func (s *ThreadService) storeNotificationInStream(
	ctx context.Context,
	notif models.ValidationNotification,
) error {
	// Build stream values
	streamValues := map[string]interface{}{
		"notificationId": notif.NotificationID,
		"threadId":       notif.ThreadID,
		"stepId":         notif.StepID,
		"stepName":       notif.StepName,
		"ownerId":        notif.OwnerID,
		"status":         string(notif.Status),
		"message":        notif.Message,
		"timestamp":      notif.Timestamp.Format(time.RFC3339),
		"maxlen":         "~",
		"limit":          100000,
	}

	// Add optional fields
	if notif.ViolationType != "" {
		streamValues["violationType"] = string(notif.ViolationType)
	}
	if notif.Severity != "" {
		streamValues["severity"] = string(notif.Severity)
	}
	if notif.FromStep != "" {
		streamValues["fromStep"] = notif.FromStep
		streamValues["toStep"] = notif.ToStep
	}
	if notif.Duration != "" {
		streamValues["duration"] = notif.Duration
		streamValues["limit"] = notif.Limit
	}
	if len(notif.MissingFields) > 0 {
		streamValues["missingFields"] = strings.Join(notif.MissingFields, ",")
	}
	if len(notif.ExtraFields) > 0 {
		streamValues["extraFields"] = strings.Join(notif.ExtraFields, ",")
	}
	if notif.Details != nil {
		detailsJSON, _ := json.Marshal(notif.Details)
		streamValues["details"] = string(detailsJSON)
	}

	_, err := s.valkeyClient.XAdd(ctx, "streams:validation_notifications", streamValues)
	return err
}

// archiveValidationResults writes validation results to archival stream for persistent storage
func (s *ThreadService) archiveValidationResults(
	ctx context.Context,
	threadID string,
	stepID string,
	stepName string,
	idempotencyKey string,
	notifications []models.ValidationNotification,
	finalStatus string,
	hasCriticalViolation bool,
) error {
	// Build validation summary
	validationSummary := make([]map[string]interface{}, 0, len(notifications))
	criticalCount := 0
	warningCount := 0
	minorCount := 0
	infoCount := 0

	for _, notif := range notifications {
		validation := map[string]interface{}{
			"type":     string(notif.ViolationType),
			"severity": string(notif.Severity),
			"message":  notif.Message,
			"passed":   notif.Status == models.NotificationStatusCompleted,
		}

		// Add optional fields if present
		if notif.Duration != "" {
			validation["duration"] = notif.Duration
		}
		if notif.Limit != "" {
			validation["limit"] = notif.Limit
		}
		if len(notif.MissingFields) > 0 {
			validation["missingFields"] = notif.MissingFields
		}
		if notif.Details != nil {
			validation["details"] = notif.Details
		}

		validationSummary = append(validationSummary, validation)

		// Count by severity
		switch notif.Severity {
		case models.SeverityCritical:
			criticalCount++
		case models.SeverityWarning:
			warningCount++
		case models.SeverityMinor:
			minorCount++
		case models.SeverityInfo:
			infoCount++
		}
	}

	// Marshal validations to JSON
	validationsJSON, err := json.Marshal(validationSummary)
	if err != nil {
		return fmt.Errorf("failed to marshal validations: %w", err)
	}

	// Build stream values for archival
	streamValues := map[string]interface{}{
		"validationID":         uuid.New().String(),
		"threadID":             threadID,
		"stepID":               stepID,
		"stepName":             stepName,
		"idempotencyKey":       idempotencyKey,
		"timestamp":            time.Now().Format(time.RFC3339),
		"validations":          string(validationsJSON),
		"overallStatus":        finalStatus,
		"hasCriticalViolation": hasCriticalViolation,
		"criticalCount":        criticalCount,
		"warningCount":         warningCount,
		"minorCount":           minorCount,
		"infoCount":            infoCount,
		"totalValidations":     len(notifications),
		"maxlen":               "~",
		"limit":                100000,
	}

	// Write to validation_results stream for archival
	_, err = s.valkeyClient.XAdd(ctx, "streams:validation_results", streamValues)
	if err != nil {
		return fmt.Errorf("failed to write to validation_results stream: %w", err)
	}

	return nil
}

// archiveThreadMetadata writes thread metadata to archival stream for persistent storage
func (s *ThreadService) archiveThreadMetadata(ctx context.Context, threadID string, status string) {
	// Get thread from cache or Valkey
	thread, err := s.repo.Get(ctx, threadID)
	if err != nil {
		fmt.Printf("[ARCHIVE-THREAD-ERROR] Failed to get thread %s: %v\n", threadID, err)
		return
	}

	// Convert contract version to string (handle nil pointer)
	contractVersion := "0"
	if thread.ContractVersion != nil {
		contractVersion = fmt.Sprintf("%d", *thread.ContractVersion)
	}

	// Handle nil contract ID
	contractID := ""
	if thread.ContractID != nil {
		contractID = *thread.ContractID
	}

	// Format completed timestamp
	completedAt := ""
	if thread.CompletedAt != nil {
		completedAt = thread.CompletedAt.Format(time.RFC3339)
	} else if status == "completed" {
		completedAt = time.Now().Format(time.RFC3339)
	}

	streamValues := map[string]interface{}{
		"id":              threadID,
		"ownerId":         thread.OwnerID,
		"companyId":       thread.CompanyID,
		"contractId":      contractID,
		"contractVersion": contractVersion,
		"contractName":    thread.ContractName,
		"status":          status,
		"lastHash":        thread.LastHash,
		"startedAt":       thread.StartedAt.Format(time.RFC3339),
		"completedAt":     completedAt,
		"maxlen":          "~",
		"limit":           100000,
	}

	_, err = s.valkeyClient.XAdd(ctx, "streams:thread_metadata", streamValues)
	if err != nil {
		fmt.Printf("[ARCHIVE-THREAD-ERROR] Failed to write thread metadata to stream: %v\n", err)
	} else {
		fmt.Printf("[ARCHIVE-THREAD-SUCCESS] Thread metadata archived: thread=%s, status=%s\n", threadID, status)
	}
}
