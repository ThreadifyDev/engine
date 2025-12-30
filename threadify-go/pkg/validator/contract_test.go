package validator

import (
	"testing"
)

func TestValidateEntryPoints_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		EntryPoints: []string{"order_placed"},
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
		},
	}

	stepIds := map[string]bool{"order_placed": true}
	errors := validator.validateEntryPoints(contract, stepIds)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestValidateEntryPoints_NotDefined(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		EntryPoints: []string{"nonexistent_step"},
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
		},
	}

	stepIds := map[string]bool{"order_placed": true}
	errors := validator.validateEntryPoints(contract, stepIds)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}

	if len(errors) > 0 && errors[0].Field != "entry_points" {
		t.Errorf("Expected error field 'entry_points', got '%s'", errors[0].Field)
	}
}

func TestValidateTerminalSteps_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		TerminalSteps: []string{"delivered"},
		Steps: []Step{
			{ID: "delivered", Owner: "logistics"},
		},
	}

	stepIds := map[string]bool{"delivered": true}
	errors := validator.validateTerminalSteps(contract, stepIds)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestValidateTerminalSteps_NotDefined(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		TerminalSteps: []string{"nonexistent_step"},
		Steps: []Step{
			{ID: "delivered", Owner: "logistics"},
		},
	}

	stepIds := map[string]bool{"delivered": true}
	errors := validator.validateTerminalSteps(contract, stepIds)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestValidateTransitionSteps_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
			{ID: "delivered", Owner: "logistics"},
		},
		Transitions: []Transition{
			{From: "order_placed", To: []string{"delivered"}},
		},
	}

	stepIds := map[string]bool{
		"order_placed": true,
		"delivered":    true,
	}
	errors := validator.validateTransitionSteps(contract, stepIds)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestValidateTransitionSteps_InvalidFrom(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{ID: "delivered", Owner: "logistics"},
		},
		Transitions: []Transition{
			{From: "nonexistent", To: []string{"delivered"}},
		},
	}

	stepIds := map[string]bool{"delivered": true}
	errors := validator.validateTransitionSteps(contract, stepIds)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestValidateTransitionSteps_InvalidTo(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
		},
		Transitions: []Transition{
			{From: "order_placed", To: []string{"nonexistent"}},
		},
	}

	stepIds := map[string]bool{"order_placed": true}
	errors := validator.validateTransitionSteps(contract, stepIds)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestValidateNoOrphanedSteps_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		EntryPoints:   []string{"order_placed"},
		TerminalSteps: []string{"delivered"},
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
			{ID: "delivered", Owner: "logistics"},
		},
		Transitions: []Transition{
			{From: "order_placed", To: []string{"delivered"}},
		},
	}

	stepIds := map[string]bool{
		"order_placed": true,
		"delivered":    true,
	}

	errors := validator.validateNoOrphanedSteps(contract, stepIds)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errors), errors)
	}
}

func TestValidateNoOrphanedSteps_Orphaned(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		EntryPoints:   []string{"order_placed"},
		TerminalSteps: []string{"delivered"},
		Steps: []Step{
			{ID: "order_placed", Owner: "merchant"},
			{ID: "orphaned", Owner: "merchant"},
			{ID: "delivered", Owner: "logistics"},
		},
		Transitions: []Transition{
			{From: "order_placed", To: []string{"delivered"}},
		},
	}

	stepIds := map[string]bool{
		"order_placed": true,
		"orphaned":     true,
		"delivered":    true,
	}

	errors := validator.validateNoOrphanedSteps(contract, stepIds)

	// orphaned has no incoming and no outgoing
	if len(errors) != 2 {
		t.Errorf("Expected 2 errors, got %d: %v", len(errors), errors)
	}
}

func TestValidateBusinessContext_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{
				ID:    "order_placed",
				Owner: "merchant",
				BusinessContext: &BusinessContext{
					Required: []string{"order_id", "customer_id"},
					Optional: []string{"notes"},
				},
			},
		},
	}

	errors := validator.validateBusinessContext(contract)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestValidateBusinessContext_Empty(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{
				ID:              "order_placed",
				Owner:           "merchant",
				BusinessContext: &BusinessContext{},
			},
		},
	}

	errors := validator.validateBusinessContext(contract)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestValidateBusinessContext_Duplicate(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Steps: []Step{
			{
				ID:    "order_placed",
				Owner: "merchant",
				BusinessContext: &BusinessContext{
					Required: []string{"order_id"},
					Optional: []string{"order_id"}, // Duplicate
				},
			},
		},
	}

	errors := validator.validateBusinessContext(contract)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestValidateValidationRules_Valid(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Validation: ValidationRules{
			MaxDuration:               "72h",
			AllowMultipleTerminals:    false,
			MultipleTerminalsSeverity: "major",
		},
	}

	errors := validator.validateValidationRules(contract)

	if len(errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(errors))
	}
}

func TestValidateValidationRules_InvalidSeverity(t *testing.T) {
	validator := NewContractValidator()

	contract := &Contract{
		Validation: ValidationRules{
			MaxDuration:               "72h",
			MultipleTerminalsSeverity: "invalid",
		},
	}

	errors := validator.validateValidationRules(contract)

	if len(errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errors))
	}
}

func TestFullContractV3_Valid(t *testing.T) {
	validator := NewContractValidator()

	yamlContent := `
contract_name: product_delivery
version: 3
description: Product delivery workflow
entry_points:
  - order_placed
parties:
  - merchant
  - logistics_carrier
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - notes
  - id: delivered
    owner: logistics_carrier
transitions:
  - from: order_placed
    to:
      - delivered
terminal_steps:
  - delivered
validation:
  max_duration: 72h
  allow_multiple_terminals: false
  multiple_terminals_severity: major
versioning:
  threads_lock_to_version: true
`

	contract, result := validator.Validate(yamlContent)

	if !result.IsValid {
		t.Errorf("Expected valid contract, got errors: %v", result.Errors)
	}

	if contract == nil {
		t.Fatal("Expected contract to be parsed")
	}

	// Verify fields were parsed correctly
	if len(contract.EntryPoints) != 1 || contract.EntryPoints[0] != "order_placed" {
		t.Errorf("Entry points not parsed correctly")
	}

	if len(contract.TerminalSteps) != 1 || contract.TerminalSteps[0] != "delivered" {
		t.Errorf("Terminal steps not parsed correctly")
	}

	if len(contract.Transitions) != 1 {
		t.Errorf("Transitions not parsed correctly")
	}

	if contract.Steps[0].BusinessContext == nil {
		t.Fatal("Business context not parsed")
	}

	if len(contract.Steps[0].BusinessContext.Required) != 2 {
		t.Errorf("Required fields not parsed correctly")
	}

	if !contract.Versioning.ThreadsLockToVersion {
		t.Errorf("Versioning not parsed correctly")
	}
}
