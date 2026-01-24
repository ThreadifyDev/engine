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
	threadRepo   interfaces.ThreadRepository
}

// NewValidationService creates a new validation service
func NewValidationService(valkeyClient interfaces.ValkeyClient, threadRepo interfaces.ThreadRepository) *ValidationService {
	return &ValidationService{
		valkeyClient: valkeyClient,
		threadRepo:   threadRepo,
	}
}

// GetCurrentSteps retrieves current step names from the sorted set
func (s *ValidationService) GetCurrentSteps(ctx context.Context, threadID string) []string {
	// Use repository method instead of direct Valkey call
	steps, err := s.threadRepo.GetCompletedSteps(ctx, threadID, true)
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
