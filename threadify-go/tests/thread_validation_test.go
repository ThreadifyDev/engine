package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/threadify/engine/internal/models"
)

// Test checkInvalidTransition
func TestCheckInvalidTransition(t *testing.T) {
	tests := []struct {
		name            string
		thread          *models.Thread
		currentStep     string
		transitions     []models.Transition
		expectViolation bool
		expectedFrom    string
		expectedTo      string
	}{
		{
			name: "valid transition",
			thread: &models.Thread{
				CurrentSteps: []string{"order_placed"},
			},
			currentStep: "payment_validated",
			transitions: []models.Transition{
				{From: "order_placed", To: []string{"payment_validated", "order_cancelled"}},
			},
			expectViolation: false,
		},
		{
			name: "invalid transition",
			thread: &models.Thread{
				CurrentSteps: []string{"order_placed"},
			},
			currentStep: "package_shipped",
			transitions: []models.Transition{
				{From: "order_placed", To: []string{"payment_validated"}},
			},
			expectViolation: true,
			expectedFrom:    "order_placed",
			expectedTo:      "package_shipped",
		},
		{
			name: "first step - no previous step",
			thread: &models.Thread{
				CurrentSteps: []string{},
			},
			currentStep:     "order_placed",
			transitions:     []models.Transition{},
			expectViolation: false,
		},
		{
			name: "no transition defined",
			thread: &models.Thread{
				CurrentSteps: []string{"order_placed"},
			},
			currentStep:     "payment_validated",
			transitions:     []models.Transition{},
			expectViolation: true,
			expectedFrom:    "order_placed",
			expectedTo:      "payment_validated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := &models.ContractGraph{
				Transitions: tt.transitions,
			}

			violation := checkInvalidTransition(tt.thread, tt.currentStep, graph)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
				if violation != nil {
					assert.Equal(t, tt.expectedFrom, violation.FromStep)
					assert.Equal(t, tt.expectedTo, violation.ToStep)
				}
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkStepTimeout
func TestCheckStepTimeout(t *testing.T) {
	tests := []struct {
		name            string
		stepNode        models.GraphNode
		startedAt       string
		finishedAt      string
		expectViolation bool
	}{
		{
			name: "within timeout",
			stepNode: models.GraphNode{
				Timeout: "10s",
			},
			startedAt:       time.Now().Add(-5 * time.Second).Format(time.RFC3339),
			finishedAt:      time.Now().Format(time.RFC3339),
			expectViolation: false,
		},
		{
			name: "exceeded timeout",
			stepNode: models.GraphNode{
				Timeout: "5s",
			},
			startedAt:       time.Now().Add(-10 * time.Second).Format(time.RFC3339),
			finishedAt:      time.Now().Format(time.RFC3339),
			expectViolation: true,
		},
		{
			name:            "no timeout defined",
			stepNode:        models.GraphNode{},
			startedAt:       time.Now().Add(-100 * time.Second).Format(time.RFC3339),
			finishedAt:      time.Now().Format(time.RFC3339),
			expectViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation := checkStepTimeout(tt.stepNode, tt.startedAt, tt.finishedAt)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkMaxDuration
func TestCheckMaxDuration(t *testing.T) {
	tests := []struct {
		name            string
		threadStartedAt time.Time
		maxDuration     string
		expectViolation bool
	}{
		{
			name:            "within max duration",
			threadStartedAt: time.Now().Add(-1 * time.Hour),
			maxDuration:     "2h",
			expectViolation: false,
		},
		{
			name:            "exceeded max duration",
			threadStartedAt: time.Now().Add(-3 * time.Hour),
			maxDuration:     "2h",
			expectViolation: true,
		},
		{
			name:            "no max duration defined",
			threadStartedAt: time.Now().Add(-100 * time.Hour),
			maxDuration:     "",
			expectViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &models.Thread{
				StartedAt: tt.threadStartedAt,
			}
			graph := &models.ContractGraph{
				Validation: &models.Validation{
					MaxDuration: tt.maxDuration,
				},
			}

			violation := checkMaxDuration(thread, graph)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkMultipleTerminalStates
func TestCheckMultipleTerminalStates(t *testing.T) {
	tests := []struct {
		name                      string
		currentSteps              []string
		currentStepName           string
		terminalSteps             []string
		allowMultipleTerminals    bool
		multipleTerminalsSeverity string
		expectViolation           bool
		expectedSeverity          models.ViolationSeverity
	}{
		{
			name:                   "first terminal state",
			currentSteps:           []string{"order_placed", "payment_validated"},
			currentStepName:        "delivered",
			terminalSteps:          []string{"delivered", "order_cancelled"},
			allowMultipleTerminals: false,
			expectViolation:        false,
		},
		{
			name:                   "second terminal state - not allowed",
			currentSteps:           []string{"delivered"},
			currentStepName:        "order_cancelled",
			terminalSteps:          []string{"delivered", "order_cancelled"},
			allowMultipleTerminals: false,
			expectViolation:        true,
			expectedSeverity:       models.SeverityCritical,
		},
		{
			name:                   "second terminal state - allowed",
			currentSteps:           []string{"delivered"},
			currentStepName:        "order_cancelled",
			terminalSteps:          []string{"delivered", "order_cancelled"},
			allowMultipleTerminals: true,
			expectViolation:        false,
		},
		{
			name:                      "second terminal state - warning severity",
			currentSteps:              []string{"delivered"},
			currentStepName:           "order_cancelled",
			terminalSteps:             []string{"delivered", "order_cancelled"},
			allowMultipleTerminals:    false,
			multipleTerminalsSeverity: "warning",
			expectViolation:           true,
			expectedSeverity:          models.SeverityWarning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &models.Thread{
				CurrentSteps: tt.currentSteps,
			}
			graph := &models.ContractGraph{
				Graph: models.Graph{
					TerminalSteps: tt.terminalSteps,
				},
				Validation: &models.Validation{
					AllowMultipleTerminals:    tt.allowMultipleTerminals,
					MultipleTerminalsSeverity: tt.multipleTerminalsSeverity,
				},
			}

			violation := checkMultipleTerminalStates(thread, tt.currentStepName, graph)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
				if violation != nil && tt.expectedSeverity != "" {
					assert.Equal(t, tt.expectedSeverity, violation.Severity)
				}
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkRetryLimit
func TestCheckRetryLimit(t *testing.T) {
	tests := []struct {
		name            string
		thread          *models.Thread
		stepName        string
		idempotencyKey  string
		retryCount      int
		maxRetries      int
		expectViolation bool
	}{
		{
			name:            "within retry limit",
			stepName:        "payment_validated",
			idempotencyKey:  "key123",
			retryCount:      2,
			maxRetries:      3,
			expectViolation: false,
		},
		{
			name:            "exceeded retry limit",
			stepName:        "payment_validated",
			idempotencyKey:  "key123",
			retryCount:      3,
			maxRetries:      3,
			expectViolation: true,
		},
		{
			name:            "no idempotency key",
			stepName:        "payment_validated",
			idempotencyKey:  "",
			retryCount:      5,
			maxRetries:      3,
			expectViolation: false,
		},
		{
			name:            "no max retries defined",
			stepName:        "payment_validated",
			idempotencyKey:  "key123",
			retryCount:      10,
			maxRetries:      0,
			expectViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := &models.Thread{
				CurrentSteps: []string{"order_placed"},
				Steps:        make(map[string]*models.StepState),
			}

			if tt.idempotencyKey != "" {
				storageKey := tt.stepName + ":" + tt.idempotencyKey
				thread.Steps[storageKey] = &models.StepState{
					StepName:       tt.stepName,
					IdempotencyKey: tt.idempotencyKey,
					RetryCount:     tt.retryCount,
				}
			}

			graph := &models.ContractGraph{
				Transitions: []models.Transition{
					{
						From:       "order_placed",
						To:         []string{tt.stepName},
						MaxRetries: tt.maxRetries,
					},
				},
			}

			violation := checkRetryLimit(thread, tt.stepName, tt.idempotencyKey, graph)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkMissingOptionalFields
func TestCheckMissingOptionalFields(t *testing.T) {
	tests := []struct {
		name            string
		stepNode        models.GraphNode
		context         map[string]string
		expectViolation bool
		expectedMissing []string
	}{
		{
			name: "all optional fields present",
			stepNode: models.GraphNode{
				BusinessContext: &models.BusinessContext{
					Optional: []string{"notes", "reference"},
				},
			},
			context: map[string]string{
				"notes":     "test",
				"reference": "ref123",
			},
			expectViolation: false,
		},
		{
			name: "some optional fields missing",
			stepNode: models.GraphNode{
				BusinessContext: &models.BusinessContext{
					Optional: []string{"notes", "reference", "metadata"},
				},
			},
			context: map[string]string{
				"notes": "test",
			},
			expectViolation: true,
			expectedMissing: []string{"reference", "metadata"},
		},
		{
			name: "no optional fields defined",
			stepNode: models.GraphNode{
				BusinessContext: &models.BusinessContext{
					Required: []string{"order_id"},
				},
			},
			context: map[string]string{
				"order_id": "123",
			},
			expectViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation := checkMissingOptionalFields(tt.stepNode, tt.context)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
				if violation != nil {
					assert.ElementsMatch(t, tt.expectedMissing, violation.MissingFields)
				}
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}

// Test checkExtraFields
func TestCheckExtraFields(t *testing.T) {
	tests := []struct {
		name            string
		stepNode        models.GraphNode
		context         map[string]string
		expectViolation bool
		expectedExtra   []string
	}{
		{
			name: "no extra fields",
			stepNode: models.GraphNode{
				BusinessContext: &models.BusinessContext{
					Required: []string{"order_id"},
					Optional: []string{"notes"},
				},
			},
			context: map[string]string{
				"order_id": "123",
				"notes":    "test",
			},
			expectViolation: false,
		},
		{
			name: "extra fields present",
			stepNode: models.GraphNode{
				BusinessContext: &models.BusinessContext{
					Required: []string{"order_id"},
				},
			},
			context: map[string]string{
				"order_id":    "123",
				"extra_field": "value",
				"another":     "value2",
			},
			expectViolation: true,
			expectedExtra:   []string{"extra_field", "another"},
		},
		{
			name:     "no business context defined",
			stepNode: models.GraphNode{},
			context: map[string]string{
				"order_id": "123",
			},
			expectViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violation := checkExtraFields(tt.stepNode, tt.context)

			if tt.expectViolation {
				assert.NotNil(t, violation, "Expected violation but got none")
				if violation != nil {
					assert.ElementsMatch(t, tt.expectedExtra, violation.ExtraFields)
				}
			} else {
				assert.Nil(t, violation, "Expected no violation but got one")
			}
		})
	}
}
