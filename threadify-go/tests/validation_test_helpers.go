package tests

import (
	"fmt"
	"slices"
	"time"

	"github.com/threadify/engine/internal/models"
)

func checkInvalidTransition(currentSteps []string, currentStep string, graph *models.ContractGraph) *models.ValidationViolation {
	if len(currentSteps) == 0 {
		return nil
	}

	fromStep := currentSteps[len(currentSteps)-1]
	for _, transition := range graph.Transitions {
		if transition.From == fromStep {
			if slices.Contains(transition.To, currentStep) {
				return nil
			}
			return &models.ValidationViolation{
				FromStep: fromStep,
				ToStep:   currentStep,
				Message:  fmt.Sprintf("invalid transition from %s to %s", fromStep, currentStep),
			}
		}
	}

	return &models.ValidationViolation{
		FromStep: fromStep,
		ToStep:   currentStep,
		Message:  fmt.Sprintf("no transition defined from %s to %s", fromStep, currentStep),
	}
}

func checkStepTimeout(stepNode models.GraphNode, startedAt, finishedAt string) *models.ValidationViolation {
	if stepNode.Timeout == "" {
		return nil
	}

	timeout, err := time.ParseDuration(stepNode.Timeout)
	if err != nil {
		return nil
	}

	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return nil
	}

	finish, err := time.Parse(time.RFC3339, finishedAt)
	if err != nil {
		return nil
	}

	duration := finish.Sub(start)
	if duration <= timeout {
		return nil
	}

	return &models.ValidationViolation{
		Message:  fmt.Sprintf("step exceeded timeout %s", stepNode.Timeout),
		Duration: duration.String(),
		Limit:    stepNode.Timeout,
	}
}

func checkMaxDuration(thread *models.Thread, graph *models.ContractGraph) *models.ValidationViolation {
	if graph.Validation == nil || graph.Validation.MaxDuration == "" {
		return nil
	}

	maxDuration, err := time.ParseDuration(graph.Validation.MaxDuration)
	if err != nil {
		return nil
	}

	elapsed := time.Since(thread.StartedAt)
	if elapsed <= maxDuration {
		return nil
	}

	return &models.ValidationViolation{
		Message:  fmt.Sprintf("thread exceeded maximum duration %s", graph.Validation.MaxDuration),
		Duration: elapsed.String(),
		Limit:    graph.Validation.MaxDuration,
	}
}

func checkMultipleTerminalStates(currentSteps []string, currentStepName string, graph *models.ContractGraph) *models.ValidationViolation {
	if graph.Validation == nil || graph.Validation.AllowMultipleTerminals {
		return nil
	}

	if !slices.Contains(graph.Graph.TerminalSteps, currentStepName) {
		return nil
	}

	for _, step := range currentSteps {
		if slices.Contains(graph.Graph.TerminalSteps, step) {
			severity := models.SeverityCritical
			switch graph.Validation.MultipleTerminalsSeverity {
			case "warning":
				severity = models.SeverityWarning
			case "major":
				severity = models.SeverityMajor
			case "minor":
				severity = models.SeverityMinor
			case "info":
				severity = models.SeverityInfo
			}
			return &models.ValidationViolation{
				Message:  "multiple terminal states detected",
				Severity: severity,
			}
		}
	}

	return nil
}

func checkRetryLimit(retryCount int, stepName, idempotencyKey string, graph *models.ContractGraph) *models.ValidationViolation {
	if idempotencyKey == "" {
		return nil
	}

	maxRetries := 0
	for _, transition := range graph.Transitions {
		if slices.Contains(transition.To, stepName) {
			maxRetries = transition.MaxRetries
			break
		}
	}
	if maxRetries <= 0 {
		return nil
	}

	if retryCount < maxRetries {
		return nil
	}

	return &models.ValidationViolation{
		Message: fmt.Sprintf("retry limit exceeded for %s:%s", stepName, idempotencyKey),
		Limit:   fmt.Sprintf("%d", maxRetries),
	}
}

func checkMissingOptionalFields(stepNode models.GraphNode, context map[string]string) *models.ValidationViolation {
	bc, ok := stepNode.BusinessContext.(*models.BusinessContext)
	if !ok || len(bc.Optional) == 0 {
		return nil
	}

	missing := make([]string, 0)
	for _, field := range bc.Optional {
		if _, exists := context[field]; !exists {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return &models.ValidationViolation{
		Message:       "missing optional fields",
		MissingFields: missing,
	}
}

func checkExtraFields(stepNode models.GraphNode, context map[string]string) *models.ValidationViolation {
	bc, ok := stepNode.BusinessContext.(*models.BusinessContext)
	if !ok {
		return nil
	}

	allowed := make(map[string]struct{}, len(bc.Required)+len(bc.Optional))
	for _, field := range bc.Required {
		allowed[field] = struct{}{}
	}
	for _, field := range bc.Optional {
		allowed[field] = struct{}{}
	}

	extra := make([]string, 0)
	for field := range context {
		if _, exists := allowed[field]; !exists {
			extra = append(extra, field)
		}
	}
	if len(extra) == 0 {
		return nil
	}

	return &models.ValidationViolation{
		Message:     "extra undocumented fields",
		ExtraFields: extra,
	}
}
