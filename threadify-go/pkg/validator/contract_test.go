package validator

import (
	"testing"
)

func TestValidateContract_ValidContract(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test contract description
parties:
  - party_a
  - party_b
steps:
  - id: step1
    owner: party_a
    type: managed
    timeout: 2s
  - id: step2
    owner: party_b
    depends_on:
      - step1
validation:
  max_duration: 5m
`

	contract, result := validator.Validate(yamlContent)
	
	if !result.IsValid {
		t.Errorf("Expected valid contract, got errors: %v", result.Errors)
	}
	
	if contract == nil {
		t.Fatal("Expected contract to be parsed")
	}
	
	if contract.ContractName != "test_contract" {
		t.Errorf("Expected contract_name 'test_contract', got '%s'", contract.ContractName)
	}
	
	if len(contract.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(contract.Steps))
	}
}

func TestValidateContract_InvalidYAML(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `invalid: yaml: content:`
	
	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected invalid YAML to fail validation")
	}
	
	if len(result.Errors) == 0 {
		t.Error("Expected validation errors")
	}
}

func TestValidateContract_InvalidContractName(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: invalid-name-with-dashes
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid contract name")
	}
	
	hasNameError := false
	for _, err := range result.Errors {
		if err.Field == "contract_name" {
			hasNameError = true
			break
		}
	}
	
	if !hasNameError {
		t.Error("Expected contract_name validation error")
	}
}

func TestValidateContract_InvalidVersion(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 0
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for version 0")
	}
	
	hasVersionError := false
	for _, err := range result.Errors {
		if err.Field == "version" {
			hasVersionError = true
			break
		}
	}
	
	if !hasVersionError {
		t.Error("Expected version validation error")
	}
}

func TestValidateContract_EmptyDescription(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: ""
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for empty description")
	}
}

func TestValidateContract_DuplicateStepIDs(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
  - id: step1
    owner: party_a
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for duplicate step IDs")
	}
	
	hasDuplicateError := false
	for _, err := range result.Errors {
		if err.Field == "steps.step1" {
			hasDuplicateError = true
			break
		}
	}
	
	if !hasDuplicateError {
		t.Error("Expected duplicate step ID error")
	}
}

func TestValidateContract_InvalidStepOwner(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_b
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid step owner")
	}
	
	hasOwnerError := false
	for _, err := range result.Errors {
		if err.Field == "steps.step1.owner" {
			hasOwnerError = true
			break
		}
	}
	
	if !hasOwnerError {
		t.Error("Expected step owner validation error")
	}
}

func TestValidateContract_InvalidStepType(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
    type: invalid_type
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid step type")
	}
}

func TestValidateContract_InvalidDependsOn(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
    depends_on:
      - nonexistent_step
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid depends_on reference")
	}
}

func TestValidateContract_InvalidTimeout(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
    timeout: invalid
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid timeout format")
	}
}

func TestValidateContract_UnassignedParty(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
  - party_b
steps:
  - id: step1
    owner: party_a
validation:
  max_duration: 5m
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for unassigned party")
	}
	
	hasPartyError := false
	for _, err := range result.Errors {
		if err.Field == "parties.party_b" {
			hasPartyError = true
			break
		}
	}
	
	if !hasPartyError {
		t.Error("Expected unassigned party validation error")
	}
}

func TestValidateContract_InvalidMaxDuration(t *testing.T) {
	validator := NewContractValidator()
	
	yamlContent := `
contract_name: test_contract
version: 1
description: Test
parties:
  - party_a
steps:
  - id: step1
    owner: party_a
validation:
  max_duration: invalid
`

	_, result := validator.Validate(yamlContent)
	
	if result.IsValid {
		t.Error("Expected validation to fail for invalid max_duration")
	}
}

func TestValidateContract_ValidDurationFormats(t *testing.T) {
	validator := NewContractValidator()
	
	validDurations := []string{"2s", "100ms", "50us", "5m", "3d", "2h", "1.5s"}
	
	for _, duration := range validDurations {
		if !validator.isValidDuration(duration) {
			t.Errorf("Expected duration '%s' to be valid", duration)
		}
	}
}

func TestValidateContract_InvalidDurationFormats(t *testing.T) {
	validator := NewContractValidator()
	
	invalidDurations := []string{"2", "100", "5minutes", "invalid", "2x"}
	
	for _, duration := range invalidDurations {
		if validator.isValidDuration(duration) {
			t.Errorf("Expected duration '%s' to be invalid", duration)
		}
	}
}

func TestValidateContract_ValidFieldTypes(t *testing.T) {
	validator := NewContractValidator()
	
	validTypes := []string{"string", "number", "boolean", "object", "array"}
	
	for _, fieldType := range validTypes {
		if !validator.isValidFieldType(fieldType) {
			t.Errorf("Expected field type '%s' to be valid", fieldType)
		}
	}
}

func TestValidateContract_InvalidFieldTypes(t *testing.T) {
	validator := NewContractValidator()
	
	invalidTypes := []string{"int", "float", "list", "dict", "invalid"}
	
	for _, fieldType := range invalidTypes {
		if validator.isValidFieldType(fieldType) {
			t.Errorf("Expected field type '%s' to be invalid", fieldType)
		}
	}
}

func TestSerializeContract(t *testing.T) {
	validator := NewContractValidator()
	
	contract := &Contract{
		ContractName: "test",
		Version:      1,
		Description:  "Test contract",
		Parties:      []string{"party_a"},
		Steps: []Step{
			{ID: "step1", Owner: "party_a"},
		},
		Validation: ValidationRules{MaxDuration: "5m"},
	}
	
	fullJSON, contentJSON, err := validator.SerializeContract(contract)
	
	if err != nil {
		t.Fatalf("Failed to serialize contract: %v", err)
	}
	
	if fullJSON == "" {
		t.Error("Expected full JSON to be non-empty")
	}
	
	if contentJSON == "" {
		t.Error("Expected content JSON to be non-empty")
	}
	
	// Full JSON should contain metadata
	if len(fullJSON) <= len(contentJSON) {
		t.Error("Expected full JSON to be larger than content-only JSON")
	}
}
