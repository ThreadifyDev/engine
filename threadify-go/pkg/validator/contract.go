package validator

import (
	"encoding/json"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

type ContractValidator struct{}

func NewContractValidator() *ContractValidator {
	return &ContractValidator{}
}

func (v *ContractValidator) Validate(yamlString string) (*Contract, *ValidationResult) {
	errors := []ValidationError{}

	// Parse YAML
	contract, err := v.parseContract(yamlString)
	if err != nil {
		return nil, &ValidationResult{
			IsValid: false,
			Errors: []ValidationError{
				{Field: "yaml", Message: fmt.Sprintf("Failed to parse YAML: %v", err)},
			},
		}
	}

	// Validate contract_name format (alphanumeric with underscores)
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(contract.ContractName) {
		errors = append(errors, ValidationError{
			Field:   "contract_name",
			Message: "Must contain only alphanumeric characters and underscores",
		})
	}

	// Validate version is positive
	if contract.Version < 1 {
		errors = append(errors, ValidationError{
			Field:   "version",
			Message: "Must be a positive integer",
		})
	}

	// Validate description is not empty
	if contract.Description == "" {
		errors = append(errors, ValidationError{
			Field:   "description",
			Message: "Cannot be empty",
		})
	}

	// Get all party IDs
	partyIds := make(map[string]bool)
	for _, party := range contract.Parties {
		partyIds[party] = true
	}

	// Get all step IDs and check for duplicates
	stepIds := make(map[string]bool)
	stepIdCount := make(map[string]int)
	for _, step := range contract.Steps {
		stepIds[step.ID] = true
		stepIdCount[step.ID]++
	}

	// Validate no duplicate step IDs
	for stepId, count := range stepIdCount {
		if count > 1 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepId),
				Message: "Duplicate step ID found. Each step must have a unique ID",
			})
		}
	}

	// Validate steps
	for _, step := range contract.Steps {
		// Validate owner is a defined party
		if !partyIds[step.Owner] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.owner", step.ID),
				Message: fmt.Sprintf("Owner '%s' is not a defined party", step.Owner),
			})
		}

		// Validate type field if present
		if step.Type != "" {
			validTypes := map[string]bool{"managed": true, "human_in_loop": true, "external": true}
			if !validTypes[step.Type] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.type", step.ID),
					Message: fmt.Sprintf("Invalid type '%s'. Must be: managed, human_in_loop, or external", step.Type),
				})
			}
		}

		// Validate depends_on references exist
		for _, depId := range step.DependsOn {
			if !stepIds[depId] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.depends_on", step.ID),
					Message: fmt.Sprintf("Referenced step '%s' does not exist", depId),
				})
			}
		}

		// Validate timeout format if present
		if step.Timeout != "" && !v.isValidDuration(step.Timeout) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.timeout", step.ID),
				Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
			})
		}

		// Validate business_context field types
		for field, fieldType := range step.BusinessContext {
			if !v.isValidFieldType(fieldType) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.business_context.%s", step.ID, field),
					Message: fmt.Sprintf("Invalid type '%s'. Must be: string, number, boolean, object, or array", fieldType),
				})
			}
		}
	}

	// Validate all party members are assigned to at least one step
	assignedParties := make(map[string]bool)
	for _, step := range contract.Steps {
		assignedParties[step.Owner] = true
	}
	for partyId := range partyIds {
		if !assignedParties[partyId] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("parties.%s", partyId),
				Message: "Party is not assigned to any step",
			})
		}
	}

	// Validate groups if present
	for _, group := range contract.Groups {
		for _, stepId := range group.Steps {
			if !stepIds[stepId] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("groups.%s.steps", group.ID),
					Message: fmt.Sprintf("Referenced step '%s' does not exist", stepId),
				})
			}
		}

		// Validate max_combined_duration format if present
		if group.Rules.MaxCombinedDuration != "" && !v.isValidDuration(group.Rules.MaxCombinedDuration) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("groups.%s.rules.max_combined_duration", group.ID),
				Message: "Invalid duration format",
			})
		}
	}

	// Validate max_duration in validation section
	if !v.isValidDuration(contract.Validation.MaxDuration) {
		errors = append(errors, ValidationError{
			Field:   "validation.max_duration",
			Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
		})
	}

	return contract, &ValidationResult{
		IsValid: len(errors) == 0,
		Errors:  errors,
	}
}

func (v *ContractValidator) parseContract(yamlString string) (*Contract, error) {
	var contract Contract
	if err := yaml.Unmarshal([]byte(yamlString), &contract); err != nil {
		return nil, err
	}
	return &contract, nil
}

func (v *ContractValidator) isValidDuration(duration string) bool {
	// Matches patterns like: 2s, 100ms, 50us, 5m, 3d, 2h
	matched, _ := regexp.MatchString(`^\d+(\.\d+)?(s|ms|us|m|d|h)$`, duration)
	return matched
}

func (v *ContractValidator) isValidFieldType(fieldType string) bool {
	validTypes := map[string]bool{
		"string":  true,
		"number":  true,
		"boolean": true,
		"object":  true,
		"array":   true,
	}
	return validTypes[fieldType]
}

func (v *ContractValidator) SerializeContract(contract *Contract) (string, string, error) {
	// Full contract as JSON
	fullJSON, err := json.Marshal(contract)
	if err != nil {
		return "", "", err
	}

	// Content only (without metadata)
	contentOnly := ContractContent{
		Parties:    contract.Parties,
		Steps:      contract.Steps,
		Groups:     contract.Groups,
		Validation: contract.Validation,
	}
	contentJSON, err := json.Marshal(contentOnly)
	if err != nil {
		return "", "", err
	}

	return string(fullJSON), string(contentJSON), nil
}
