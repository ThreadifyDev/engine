package validator

import (
	"encoding/json"
	"strings"
	"testing"
)

func dependencyContract(steps string) string {
	return "Feature: partial_order\nVersion: 1\nDescription: partial order test\n" + steps
}

func TestContractValidator_AcceptsAndSerializesDependencies(t *testing.T) {
	validator := NewContractValidator()
	contract, result := validator.Validate(dependencyContract(`Rule: Authenticate
  When step "authenticated" is submitted
  Then owner must be "agent"
  And this step is an entry point
Rule: Charge
  When step "charge" is submitted
  Then owner must be "agent"
  And step "authenticated" must have succeeded
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
			steps: `Rule: Charge
  When step "charge" is submitted
  Then owner must be "agent"
  And step "missing" must have succeeded
`,
			message: "is not defined",
		},
		{
			name: "self dependency",
			steps: `Rule: Charge
  When step "charge" is submitted
  Then owner must be "agent"
  And step "charge" must have succeeded
`,
			message: "cannot depend on itself",
		},
		{
			name: "dependency cycle",
			steps: `Rule: First
  When step "first" is submitted
  Then owner must be "agent"
  And step "second" must have succeeded
Rule: Second
  When step "second" is submitted
  Then owner must be "agent"
  And step "first" must have succeeded
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
