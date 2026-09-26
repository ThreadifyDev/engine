package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	shderrors "threadify-go/shared/errors"

	"github.com/threadify/engine/pkg/validator"
)

var includedContractName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
var includeDeclaration = regexp.MustCompile(`(?m)^\s*Include:`)

func sourceDeclaresIncludes(source string) bool {
	return includeDeclaration.MatchString(source)
}

func (s *ContractService) expandContractSource(ctx context.Context, companyID, source string) (string, *validator.ValidationResult, error) {
	if !sourceDeclaresIncludes(source) {
		return source, nil, nil
	}
	return s.resolveContractIncludes(ctx, companyID, source)
}

// resolveContractIncludes composes exact versions of contracts owned by the
// caller's company. Stored Content is already compiled, so nested includes are
// frozen at their original publication and do not need another lookup.
func (s *ContractService) resolveContractIncludes(ctx context.Context, companyID, source string) (string, *validator.ValidationResult, error) {
	root, err := validator.ParseSource(source)
	if err != nil {
		return "", &validator.ValidationResult{Errors: []validator.ValidationError{{Field: "source", Message: fmt.Sprintf("Failed to parse contract: %v", err)}}}, nil
	}
	if len(root.Includes) == 0 {
		return source, nil, nil
	}
	invalid := func(message string) (string, *validator.ValidationResult, error) {
		return "", &validator.ValidationResult{Errors: []validator.ValidationError{{Field: "includes", Message: message}}}, nil
	}
	if len(root.Includes) > 16 {
		return invalid("A contract may include at most 16 other contracts")
	}
	if companyID == "" || s.repo == nil {
		return invalid("Company-scoped contract lookup is unavailable")
	}
	composed := *root
	composed.Steps = append([]validator.Step(nil), root.Steps...)
	composed.Parties = append([]string(nil), root.Parties...)
	composed.Transitions = append([]validator.Transition(nil), root.Transitions...)
	composed.Groups = append([]validator.Group(nil), root.Groups...)
	composed.EntryPoints = append([]string(nil), root.EntryPoints...)
	composed.TerminalSteps = append([]string(nil), root.TerminalSteps...)
	// Validation is performed on the expanded content. The original source
	// retains the exact include references for later inspection.
	composed.Includes = nil
	seen := map[string]bool{}
	groupIDs := map[string]bool{}
	var inheritedEntries, inheritedTerminals []string
	for _, group := range composed.Groups {
		groupIDs[group.ID] = true
	}
	for _, include := range root.Includes {
		if !includedContractName.MatchString(include.Name) || include.Version < 1 {
			return invalid("Include must name a contract and a positive version")
		}
		if seen[include.Name] {
			return invalid(fmt.Sprintf("Contract %q may be included only once", include.Name))
		}
		seen[include.Name] = true
		contract, err := s.repo.GetByNameAndCompany(ctx, include.Name, companyID)
		if err != nil {
			if errors.Is(err, shderrors.ErrContractNotFound) {
				return invalid(fmt.Sprintf("Included contract %q was not found in this company", include.Name))
			}
			return "", nil, fmt.Errorf("lookup included contract %q: %w", include.Name, err)
		}
		version, err := s.repo.GetVersion(ctx, contract.ID, include.Version)
		if err != nil {
			if errors.Is(err, shderrors.ErrContractNotFound) {
				return invalid(fmt.Sprintf("Included contract %q version %d was not found", include.Name, include.Version))
			}
			return "", nil, fmt.Errorf("lookup included contract %q version %d: %w", include.Name, include.Version, err)
		}
		var compiled validator.Contract
		if version.Content == "" || json.Unmarshal([]byte(version.Content), &compiled) != nil || compiled.ContractName != include.Name || compiled.Version != include.Version || len(compiled.Includes) != 0 {
			return invalid(fmt.Sprintf("Included contract %q version %d has no valid compiled content", include.Name, include.Version))
		}
		if len(composed.Steps)+len(compiled.Steps) > 512 {
			return invalid("Expanded contract exceeds 512 steps")
		}
		composed.Steps = append(composed.Steps, compiled.Steps...)
		for _, party := range compiled.Parties {
			if !containsString(composed.Parties, party) {
				composed.Parties = append(composed.Parties, party)
			}
		}
		composed.Transitions = append(composed.Transitions, compiled.Transitions...)
		if composed.Validation.MaxDuration != "" && compiled.Validation.MaxDuration != "" && !sameContractDuration(composed.Validation.MaxDuration, compiled.Validation.MaxDuration) {
			return invalid("Included contracts have conflicting thread durations")
		}
		if composed.Validation.MaxDuration == "" {
			composed.Validation.MaxDuration = compiled.Validation.MaxDuration
		}
		if composed.Validation.MultipleTerminalsSeverity != "" && compiled.Validation.MultipleTerminalsSeverity != "" && composed.Validation.MultipleTerminalsSeverity != compiled.Validation.MultipleTerminalsSeverity {
			return invalid("Included contracts have conflicting multiple-terminal severities")
		}
		if composed.Validation.MultipleTerminalsSeverity == "" {
			composed.Validation.MultipleTerminalsSeverity = compiled.Validation.MultipleTerminalsSeverity
		}
		composed.Validation.AllowMultipleTerminals = composed.Validation.AllowMultipleTerminals || compiled.Validation.AllowMultipleTerminals
		composed.Versioning.ThreadsLockToVersion = composed.Versioning.ThreadsLockToVersion || compiled.Versioning.ThreadsLockToVersion
		for _, group := range compiled.Groups {
			if groupIDs[group.ID] {
				return invalid(fmt.Sprintf("Duplicate group ID %q in combined contract", group.ID))
			}
			groupIDs[group.ID] = true
			composed.Groups = append(composed.Groups, group)
		}
		for _, entry := range compiled.EntryPoints {
			if !containsString(inheritedEntries, entry) {
				inheritedEntries = append(inheritedEntries, entry)
			}
		}
		for _, terminal := range compiled.TerminalSteps {
			if !containsString(inheritedTerminals, terminal) {
				inheritedTerminals = append(inheritedTerminals, terminal)
			}
		}
	}
	if len(composed.EntryPoints) == 0 {
		composed.EntryPoints = inheritedEntries
	}
	if len(composed.TerminalSteps) == 0 {
		composed.TerminalSteps = inheritedTerminals
	}
	validated, result := validator.NewContractValidator().ValidateCompiled(&composed)
	if !result.IsValid {
		return "", result, nil
	}
	data, err := json.Marshal(validated)
	if err != nil {
		return "", nil, fmt.Errorf("serialize expanded contract: %w", err)
	}
	graph, err := NewGraphBuilder().BuildGraph(data)
	if err != nil {
		return "", nil, fmt.Errorf("build expanded contract graph: %w", err)
	}
	if contradictions := validateComposition(validated, graph); len(contradictions) > 0 {
		return "", &validator.ValidationResult{Errors: contradictions}, nil
	}
	return string(data), nil, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
