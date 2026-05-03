package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/threadify/engine/internal/domain"
)

// ValidationService handles synchronous validation checks for thread operations.
// It provides methods to validate step transitions, timeouts, business context,
// and other compliance rules without blocking the main thread execution.
// ValidationService handles all validation logic for threads and steps
type ValidationService struct {
	threadRepo domain.ThreadRepository
}

// NewValidationService creates a new validation service
func NewValidationService(threadRepo domain.ThreadRepository) *ValidationService {
	return &ValidationService{
		threadRepo: threadRepo,
	}
}

// GetCurrentSteps retrieves current step names from the sorted set
func (s *ValidationService) GetCurrentSteps(ctx context.Context, threadID string) []string {
	// Use repository method instead of direct Valkey call
	steps, err := s.threadRepo.GetCompletedSteps(ctx, threadID, domain.ThreadReadOptions{WriteBack: true})
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
	stepNode domain.GraphNode,
	startedAt string,
	finishedAt string,
) *domain.ValidationViolation {
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
		return &domain.ValidationViolation{
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
	thread *domain.Thread,
	graph *domain.ContractGraph,
) *domain.ValidationViolation {
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
		return &domain.ValidationViolation{
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
	stepNode domain.GraphNode,
	context map[string]string,
) *domain.ValidationViolation {
	if stepNode.BusinessContext == nil {
		return nil
	}

	bc := stepNode.BusinessContext

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
		return &domain.ValidationViolation{
			Message:       fmt.Sprintf("Missing %d optional field(s)", len(missingFields)),
			MissingFields: missingFields,
			Details: map[string]interface{}{
				"missing_optional_fields": missingFields,
			},
		}
	}

	return nil
}
