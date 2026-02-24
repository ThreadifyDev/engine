package tests

import (
	"encoding/json"
	"fmt"
	"regexp"

	pkgvalidator "github.com/threadify/engine/pkg/validator"
	"gopkg.in/yaml.v3"
)

type Contract = pkgvalidator.Contract
type Step = pkgvalidator.Step
type Group = pkgvalidator.Group
type GroupRules = pkgvalidator.GroupRules
type Transition = pkgvalidator.Transition
type BusinessContext = pkgvalidator.BusinessContext
type ValidationRules = pkgvalidator.ValidationRules
type VersioningRules = pkgvalidator.VersioningRules
type ValidationError = pkgvalidator.ValidationError
type ValidationResult = pkgvalidator.ValidationResult
type ContractContent = pkgvalidator.ContractContent

type ContractValidator struct{}

func NewContractValidator() *ContractValidator {
	return &ContractValidator{}
}

func (v *ContractValidator) Validate(yamlString string) (*Contract, *ValidationResult) {
	errors := []ValidationError{}

	contract, err := v.parseContract(yamlString)
	if err != nil {
		return nil, &ValidationResult{
			IsValid: false,
			Errors: []ValidationError{
				{Field: "yaml", Message: fmt.Sprintf("Failed to parse YAML: %v", err)},
			},
		}
	}

	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(contract.ContractName) {
		errors = append(errors, ValidationError{
			Field:   "contract_name",
			Message: "Must contain only alphanumeric characters and underscores",
		})
	}

	if contract.Version < 1 {
		errors = append(errors, ValidationError{
			Field:   "version",
			Message: "Must be a positive integer",
		})
	}

	if contract.Description == "" {
		errors = append(errors, ValidationError{
			Field:   "description",
			Message: "Cannot be empty",
		})
	}

	partyIDs := make(map[string]bool)
	for _, party := range contract.Parties {
		partyIDs[party] = true
	}

	stepIDs := make(map[string]bool)
	stepIDCount := make(map[string]int)
	for _, step := range contract.Steps {
		stepIDs[step.ID] = true
		stepIDCount[step.ID]++
	}

	for stepID, count := range stepIDCount {
		if count > 1 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepID),
				Message: "Duplicate step ID found. Each step must have a unique ID",
			})
		}
	}

	for _, step := range contract.Steps {
		if !partyIDs[step.Owner] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.owner", step.ID),
				Message: fmt.Sprintf("Owner '%s' is not a defined party", step.Owner),
			})
		}

		if step.Type != "" {
			validTypes := map[string]bool{"managed": true, "human_in_loop": true, "external": true}
			if !validTypes[step.Type] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("steps.%s.type", step.ID),
					Message: fmt.Sprintf("Invalid type '%s'. Must be: managed, human_in_loop, or external", step.Type),
				})
			}
		}

		if step.Timeout != "" && !v.isValidDuration(step.Timeout) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.timeout", step.ID),
				Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
			})
		}
	}

	assignedParties := make(map[string]bool)
	for _, step := range contract.Steps {
		assignedParties[step.Owner] = true
	}
	for partyID := range partyIDs {
		if !assignedParties[partyID] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("parties.%s", partyID),
				Message: "Party is not assigned to any step",
			})
		}
	}

	for _, group := range contract.Groups {
		for _, stepID := range group.Steps {
			if !stepIDs[stepID] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("groups.%s.steps", group.ID),
					Message: fmt.Sprintf("Referenced step '%s' does not exist", stepID),
				})
			}
		}

		if group.Rules.MaxCombinedDuration != "" && !v.isValidDuration(group.Rules.MaxCombinedDuration) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("groups.%s.rules.max_combined_duration", group.ID),
				Message: "Invalid duration format",
			})
		}
	}

	if !v.isValidDuration(contract.Validation.MaxDuration) {
		errors = append(errors, ValidationError{
			Field:   "validation.max_duration",
			Message: "Invalid duration format. Use: s, ms, us, m, h, or d (e.g., '2s', '3d')",
		})
	}

	errors = append(errors, v.validateEntryPoints(contract, stepIDs)...)
	errors = append(errors, v.validateTerminalSteps(contract, stepIDs)...)
	errors = append(errors, v.validateTerminalStepsReachable(contract)...)
	errors = append(errors, v.validateTransitionSteps(contract, stepIDs)...)
	errors = append(errors, v.validateNoOrphanedSteps(contract, stepIDs)...)
	errors = append(errors, v.validateBusinessContext(contract)...)
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
	matched, _ := regexp.MatchString(`^\d+(\.\d+)?(s|ms|us|m|d|h)$`, duration)
	return matched
}

func (v *ContractValidator) SerializeContract(contract *Contract) (string, string, error) {
	fullJSON, err := json.Marshal(contract)
	if err != nil {
		return "", "", err
	}

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

func (v *ContractValidator) validateEntryPoints(contract *Contract, stepIDs map[string]bool) []ValidationError {
	errors := []ValidationError{}
	if len(contract.EntryPoints) == 0 {
		return errors
	}

	for _, entryPoint := range contract.EntryPoints {
		if !stepIDs[entryPoint] {
			errors = append(errors, ValidationError{
				Field:   "entry_points",
				Message: fmt.Sprintf("Entry point '%s' is not defined in steps", entryPoint),
			})
		}
	}

	return errors
}

func (v *ContractValidator) validateTerminalSteps(contract *Contract, stepIDs map[string]bool) []ValidationError {
	errors := []ValidationError{}
	if len(contract.TerminalSteps) == 0 {
		return errors
	}

	for _, terminalStep := range contract.TerminalSteps {
		if !stepIDs[terminalStep] {
			errors = append(errors, ValidationError{
				Field:   "terminal_steps",
				Message: fmt.Sprintf("Terminal step '%s' is not defined in steps", terminalStep),
			})
		}
	}

	return errors
}

func (v *ContractValidator) validateTerminalStepsReachable(contract *Contract) []ValidationError {
	errors := []ValidationError{}
	if len(contract.TerminalSteps) == 0 || len(contract.Transitions) == 0 {
		return errors
	}

	reachableSteps := make(map[string]bool)
	for _, transition := range contract.Transitions {
		for _, toStep := range transition.To {
			reachableSteps[toStep] = true
		}
	}

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

func (v *ContractValidator) validateTransitionSteps(contract *Contract, stepIDs map[string]bool) []ValidationError {
	errors := []ValidationError{}

	for i, transition := range contract.Transitions {
		if !stepIDs[transition.From] {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("transitions[%d].from", i),
				Message: fmt.Sprintf("Step '%s' is not defined in steps", transition.From),
			})
		}

		for _, toStep := range transition.To {
			if !stepIDs[toStep] {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("transitions[%d].to", i),
					Message: fmt.Sprintf("Step '%s' is not defined in steps", toStep),
				})
			}
		}
	}

	return errors
}

func (v *ContractValidator) validateNoOrphanedSteps(contract *Contract, stepIDs map[string]bool) []ValidationError {
	errors := []ValidationError{}
	if len(contract.Transitions) == 0 {
		return errors
	}

	entryPointSet := make(map[string]bool)
	for _, entryPoint := range contract.EntryPoints {
		entryPointSet[entryPoint] = true
	}

	terminalStepSet := make(map[string]bool)
	for _, terminalStep := range contract.TerminalSteps {
		terminalStepSet[terminalStep] = true
	}

	hasIncoming := make(map[string]bool)
	hasOutgoing := make(map[string]bool)

	for _, transition := range contract.Transitions {
		hasOutgoing[transition.From] = true
		for _, toStep := range transition.To {
			hasIncoming[toStep] = true
		}
	}

	for stepID := range stepIDs {
		isEntryPoint := entryPointSet[stepID]
		isTerminal := terminalStepSet[stepID]
		incoming := hasIncoming[stepID]
		outgoing := hasOutgoing[stepID]

		if !isEntryPoint && !incoming {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepID),
				Message: fmt.Sprintf("Step '%s' has no incoming transitions and is not an entry point", stepID),
			})
		}

		if !isTerminal && !outgoing {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s", stepID),
				Message: fmt.Sprintf("Step '%s' has no outgoing transitions and is not a terminal step", stepID),
			})
		}
	}

	return errors
}

func (v *ContractValidator) validateBusinessContext(contract *Contract) []ValidationError {
	errors := []ValidationError{}

	for _, step := range contract.Steps {
		if step.BusinessContext == nil {
			continue
		}

		bc := step.BusinessContext
		if len(bc.Required) == 0 && len(bc.Optional) == 0 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("steps.%s.business_context", step.ID),
				Message: "business_context must have at least 'required' or 'optional' fields",
			})
		}

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

func (v *ContractValidator) validateValidationRules(contract *Contract) []ValidationError {
	errors := []ValidationError{}

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
