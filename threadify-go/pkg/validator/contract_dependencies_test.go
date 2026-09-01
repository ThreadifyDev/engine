package validator

import (
	"encoding/json"
	"strings"
	"testing"
)

func dependencyContract(steps string) string {
	return `
contract_name: partial_order
version: 1
description: partial order test
parties: [agent]
steps:
` + steps
}

func TestContractValidator_AcceptsAndSerializesDependencies(t *testing.T) {
	validator := NewContractValidator()
	contract, result := validator.Validate(dependencyContract(`
  - id: authenticated
    owner: agent
  - id: charge
    owner: agent
    depends_on: [authenticated]
`))

	if !result.IsValid {
		t.Fatalf("expected valid contract, got errors: %#v", result.Errors)
	}
	_, content, err := validator.SerializeContract(contract)
	if err != nil {
		t.Fatal(err)
	}
	var serialized map[string]any
	if err := json.Unmarshal([]byte(content), &serialized); err != nil {
		t.Fatal(err)
	}
	serializedSteps := serialized["steps"].([]any)
	charge := serializedSteps[1].(map[string]any)
	if _, ok := charge["DependsOn"]; !ok {
		t.Fatalf("serialized contract dropped DependsOn: %s", content)
	}
}

func TestContractValidator_RejectsInvalidDependencies(t *testing.T) {
	tests := []struct {
		name    string
		steps   string
		message string
	}{
		{
			name: "unknown dependency",
			steps: `
  - id: charge
    owner: agent
    depends_on: [missing]
`,
			message: "is not defined",
		},
		{
			name: "self dependency",
			steps: `
  - id: charge
    owner: agent
    depends_on: [charge]
`,
			message: "cannot depend on itself",
		},
		{
			name: "dependency cycle",
			steps: `
  - id: first
    owner: agent
    depends_on: [second]
  - id: second
    owner: agent
    depends_on: [first]
`,
			message: "must not contain a cycle",
		},
	}

	validator := NewContractValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, result := validator.Validate(dependencyContract(tt.steps))
			if result.IsValid {
				t.Fatal("expected invalid contract")
			}
			for _, validationError := range result.Errors {
				if strings.Contains(validationError.Message, tt.message) {
					return
				}
			}
			t.Fatalf("expected error containing %q, got %#v", tt.message, result.Errors)
		})
	}
}
