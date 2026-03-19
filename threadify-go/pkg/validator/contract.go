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

		// Validate timeout format if present
		if step.Timeout != "" && !v.isValidDuration(step.Timeout) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.timeout", step.ID),
				Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
			})
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

	// Validate max_duration in validation section (optional field)
	if contract.Validation.MaxDuration != "" && !v.isValidDuration(contract.Validation.MaxDuration) {
		errors = append(errors, ValidationError{
			Field:   "validation.max_duration",
			Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
		})
	}

	// ========================================
	// CONTRACT V3 VALIDATIONS
	// ========================================

	// Validate entry points
	errors = append(errors, v.validateEntryPoints(contract, stepIds)...)

	// Validate terminal steps
	errors = append(errors, v.validateTerminalSteps(contract, stepIds)...)

	// Validate terminal steps are reachable
	errors = append(errors, v.validateTerminalStepsReachable(contract)...)

	// Validate transition steps
	errors = append(errors, v.validateTransitionSteps(contract, stepIds)...)

	// Validate no orphaned steps
	errors = append(errors, v.validateNoOrphanedSteps(contract, stepIds)...)

	// Validate business context structure
	errors = append(errors, v.validateBusinessContext(contract)...)

	// Validate validation rules
	errors = append(errors, v.validateValidationRules(contract)...)

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
		EntryPoints:   contract.EntryPoints,
		Parties:       contract.Parties,
		Steps:         contract.Steps,
		Transitions:   contract.Transitions,
		TerminalSteps: contract.TerminalSteps,
		Groups:        contract.Groups,
		Validation:    contract.Validation,
		Versioning:    contract.Versioning,
	}
	contentJSON, err := json.Marshal(contentOnly)
	if err != nil {
		return "", "", err
	}

	return string(fullJSON), string(contentJSON), nil
}

// ========================================
// CONTRACT V3 VALIDATION RULES
// ========================================

// validateEntryPoints validates that entry points exist and are valid
func (v *ContractValidator) validateEntryPoints(contract *Contract, stepIds map[string]bool) []ValidationError {
	errors := []ValidationError{}

	// If no entry points specified, it's optional (backward compatibility)
	if len(contract.EntryPoints) == 0 {
		return errors
	}

	// All entry points must reference valid steps
	for _, entryPoint := range contract.EntryPoints {
		if !stepIds[entryPoint] {
			errors = append(errors, ValidationError{
				Field:   "entry_points",
				Message: fmt.Sprintf("Entry point '%s' is not defined in steps", entryPoint),
			})
		}
	}

	return errors
}

// validateTerminalSteps validates that terminal steps exist
func (v *ContractValidator) validateTerminalSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
	errors := []ValidationError{}

	// If no terminal steps specified, it's optional (backward compatibility)
	if len(contract.TerminalSteps) == 0 {
		return errors
	}

	// All terminal steps must reference valid steps
	for _, terminalStep := range contract.TerminalSteps {
		if !stepIds[terminalStep] {
			errors = append(errors, ValidationError{
				Field:   "terminal_steps",
				Message: fmt.Sprintf("Terminal step '%s' is not defined in steps", terminalStep),
			})
		}
	}

	return errors
}

// validateTerminalStepsReachable validates that terminal steps are reachable from transitions
func (v *ContractValidator) validateTerminalStepsReachable(contract *Contract) []ValidationError {
	errors := []ValidationError{}

	// Skip if no terminal steps or transitions defined
	if len(contract.TerminalSteps) == 0 || len(contract.Transitions) == 0 {
		return errors
	}

	// Build set of steps that appear in transitions' "to" field
	reachableSteps := make(map[string]bool)
	for _, transition := range contract.Transitions {
		for _, toStep := range transition.To {
			reachableSteps[toStep] = true
		}
	}

	// Check each terminal step is reachable
	for _, terminalStep := range contract.TerminalSteps {
		if !reachableSteps[terminalStep] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("terminal_steps.%s", terminalStep),
				Message: fmt.Sprintf("Terminal step '%s' is not reachable from any transition", terminalStep),
			})
		}
	}

	return errors
}

// validateTransitionSteps validates that all transition steps exist
func (v *ContractValidator) validateTransitionSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
	errors := []ValidationError{}

	for i, transition := range contract.Transitions {
		// Validate "from" step exists
		if !stepIds[transition.From] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("transitions[%d].from", i),
				Message: fmt.Sprintf("Step '%s' is not defined in steps", transition.From),
			})
		}

		// Validate all "to" steps exist
		for _, toStep := range transition.To {
			if !stepIds[toStep] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("transitions[%d].to", i),
					Message: fmt.Sprintf("Step '%s' is not defined in steps", toStep),
				})
			}
		}
	}

	return errors
}

// validateNoOrphanedSteps validates that all steps are connected via transitions or are entry/terminal steps
func (v *ContractValidator) validateNoOrphanedSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
	errors := []ValidationError{}

	// Skip if no transitions defined (backward compatibility)
	if len(contract.Transitions) == 0 {
		return errors
	}

	// Build sets
	entryPointSet := make(map[string]bool)
	for _, ep := range contract.EntryPoints {
		entryPointSet[ep] = true
	}

	terminalStepSet := make(map[string]bool)
	for _, ts := range contract.TerminalSteps {
		terminalStepSet[ts] = true
	}

	// Build incoming and outgoing transition maps
	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)

	for _, transition := range contract.Transitions {
		hasOutgoing[transition.From] = true
		for _, toStep := range transition.To {
			hasIncoming[toStep] = true
		}
	}

	// Check each step
	for stepId := range stepIds {
		isEntryPoint := entryPointSet[stepId]
		isTerminal := terminalStepSet[stepId]
		incoming := hasIncoming[stepId]
		outgoing := hasOutgoing[stepId]

		// Entry points don't need incoming, terminals don't need outgoing
		// But all other steps need at least one of each
		if !isEntryPoint && !incoming {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepId),
				Message: fmt.Sprintf("Step '%s' has no incoming transitions and is not an entry point", stepId),
			})
		}

		if !isTerminal && !outgoing {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepId),
				Message: fmt.Sprintf("Step '%s' has no outgoing transitions and is not a terminal step", stepId),
			})
		}
	}

	return errors
}

// validateBusinessContext validates the business_context structure
func (v *ContractValidator) validateBusinessContext(contract *Contract) []ValidationError {
	errors := []ValidationError{}

	for _, step := range contract.Steps {
		if step.BusinessContext == nil {
			continue
		}

		bc := step.BusinessContext

		// Must have at least required or optional
		if len(bc.Required) == 0 && len(bc.Optional) == 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.business_context", step.ID),
				Message: "business_context must have at least 'required' or 'optional' fields",
			})
		}

		// Check for duplicates between required and optional
		requiredSet := make(map[string]bool)
		for _, field := range bc.Required {
			requiredSet[field] = true
		}

		for _, field := range bc.Optional {
			if requiredSet[field] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.business_context", step.ID),
					Message: fmt.Sprintf("Field '%s' appears in both required and optional", field),
				})
			}
		}
	}

	return errors
}

// validateValidationRules validates the validation rules structure
func (v *ContractValidator) validateValidationRules(contract *Contract) []ValidationError {
	errors := []ValidationError{}

	// Validate multiple_terminals_severity if present
	if contract.Validation.MultipleTerminalsSeverity != "" {
		validSeverities := map[string]bool{
			"minor":   true,
			"warning": true,
			"major":   true,
			"error":   true,
		}

		if !validSeverities[contract.Validation.MultipleTerminalsSeverity] {
			errors = append(errors, ValidationError{
				Field:   "validation.multiple_terminals_severity",
				Message: "Must be one of: minor, warning, major, error",
			})
		}
	}

	return errors
}
