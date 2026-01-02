package tests

import (
	"context"

	"github.com/threadify/engine/internal/models"
	"github.com/threadify/engine/internal/service"
)

// Test helper functions that maintain backward compatibility using unified MockValkeyClient
func checkInvalidTransition(thread *models.Thread, currentStep string, graph *models.ContractGraph) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()

	// Set up mock current steps for testing (since Thread model doesn't have CurrentSteps field)
	// For testing purposes, assume we have a previous step to test transitions
	mockSteps := []string{"previous_step"}
	mockClient.SetCurrentSteps(thread.ID, mockSteps)

	validationService := service.NewValidationService(mockClient)
	return validationService.CheckInvalidTransition(context.Background(), thread.ID, currentStep, graph)
}

func checkStepTimeout(stepNode models.GraphNode, startedAt, finishedAt string) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()
	validationService := service.NewValidationService(mockClient)
	return validationService.CheckStepTimeout(stepNode, startedAt, finishedAt)
}

func checkMaxDuration(thread *models.Thread, graph *models.ContractGraph) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()
	validationService := service.NewValidationService(mockClient)
	return validationService.CheckMaxDuration(thread, graph)
}

func checkMultipleTerminalStates(thread *models.Thread, currentStepName string, graph *models.ContractGraph) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()

	// Set up mock current steps for testing (since Thread model doesn't have CurrentSteps field)
	// For testing purposes, assume we have a current step to test terminal states
	mockSteps := []string{currentStepName}
	mockClient.SetCurrentSteps(thread.ID, mockSteps)

	validationService := service.NewValidationService(mockClient)
	return validationService.CheckMultipleTerminalStates(context.Background(), thread.ID, currentStepName, graph)
}

func checkRetryLimit(thread *models.Thread, stepName, idempotencyKey string, graph *models.ContractGraph) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()

	// Set up retry count if needed
	mockClient.SetStepState(thread.ID, stepName, idempotencyKey, "retryCount", "1")

	validationService := service.NewValidationService(mockClient)
	return validationService.CheckRetryLimit(context.Background(), thread.ID, stepName, idempotencyKey, graph)
}

func checkMissingOptionalFields(stepNode models.GraphNode, context map[string]string) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()
	validationService := service.NewValidationService(mockClient)
	return validationService.CheckMissingOptionalFields(stepNode, context)
}

func checkExtraFields(stepNode models.GraphNode, context map[string]string) *models.ValidationViolation {
	mockClient := NewMockValkeyClient()
	validationService := service.NewValidationService(mockClient)
	return validationService.CheckExtraFields(stepNode, context)
}
