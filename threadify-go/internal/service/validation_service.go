package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/threadify/engine/internal/interfaces"
	"github.com/threadify/engine/internal/models"
)

// ValidationService handles synchronous validation checks for thread operations.
// It provides methods to validate step transitions, timeouts, business context,
// and other compliance rules without blocking the main thread execution.
// ValidationService handles all validation logic for threads and steps
type ValidationService struct {
	valkeyClient interfaces.ValkeyClient
}

// NewValidationService creates a new validation service
func NewValidationService(valkeyClient interfaces.ValkeyClient) *ValidationService {
	return &ValidationService{
		valkeyClient: valkeyClient,
	}
}

// GetCurrentSteps retrieves current step names from the sorted set
func (s *ValidationService) GetCurrentSteps(ctx context.Context, threadID string) []string {
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

// CheckInvalidTransition validates if the transition from current step to new step is allowed
// DEPRECATED: This function has race conditions. Use StepStateRepository.ValidateAndUpdateStepState() instead.
// Kept only for backward compatibility with existing tests.
func (s *ValidationService) CheckInvalidTransition(
	ctx context.Context,
	threadID string,
	currentStepName string,
	graph *models.ContractGraph,
) *models.ValidationViolation {
	// Get current steps from sorted set
	currentSteps := s.GetCurrentSteps(ctx, threadID)

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

// CheckStepTimeout validates if the step execution time exceeded the timeout
func (s *ValidationService) CheckStepTimeout(
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

// CheckMaxDuration validates if the thread duration exceeded the max duration
func (s *ValidationService) CheckMaxDuration(
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

// CheckMultipleTerminalStates validates if thread reached multiple terminal states
// DEPRECATED: This function has race conditions. Use StepStateRepository.ValidateAndUpdateStepState() instead.
// Kept only for backward compatibility with existing tests.
func (s *ValidationService) CheckMultipleTerminalStates(
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
	currentSteps := s.GetCurrentSteps(ctx, threadID)

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

// CheckRetryLimit - REMOVED
// Retry limit validation is now handled atomically in the Lua script
// via StepStateRepository.ValidateAndUpdateStepState()
// See: /internal/repository/valkey/lua/validate_and_update_step_state.lua

// CheckMissingOptionalFields checks for missing optional business context fields
func (s *ValidationService) CheckMissingOptionalFields(
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

// CheckExtraFields checks for fields not defined in business context
func (s *ValidationService) CheckExtraFields(
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
