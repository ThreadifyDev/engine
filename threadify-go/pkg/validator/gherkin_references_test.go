package validator

import (
	"os"
	"strings"
	"testing"
)

func TestGherkinReferences(t *testing.T) {
	source := simpleFeature + ` And content "reference" must equal approval.reference` + "\n"
	c, r := NewContractValidator().Validate(source)
	if !r.IsValid {
		t.Fatal(r.Errors)
	}
	ref := c.Steps[1].ContentRules[0].Reference
	if ref == nil || ref.Step != "approval" || ref.Field != "reference" {
		t.Fatalf("lost reference: %+v", ref)
	}
	_, jsonSource, err := NewContractValidator().SerializeContract(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonSource, `"reference":{"step":"approval","field":"reference"}`) {
		t.Fatal(jsonSource)
	}
	literal := strings.Replace(source, `must equal approval.reference`, `must equal "approval.reference"`, 1)
	c, r = NewContractValidator().Validate(literal)
	if !r.IsValid {
		t.Fatal(r.Errors)
	}
	if c.Steps[1].ContentRules[0].Reference != nil {
		t.Fatal("quoted literal became reference")
	}
	for _, bad := range []string{"missing.reference", "charge.reference", "approval.", ".reference", "approval.reference.extra", "approval.reference trailing"} {
		_, r := NewContractValidator().Validate(strings.Replace(source, "approval.reference", bad, 1))
		if r.IsValid {
			t.Fatalf("accepted %s", bad)
		}
	}
}

func TestReferenceDependenciesAndCycles(t *testing.T) {
	source, err := os.ReadFile("../../examples/shipping.feature")
	if err != nil {
		t.Fatal(err)
	}
	c, result := NewContractValidator().Validate(string(source))
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	if len(c.Steps[1].DependsOn) != 1 || c.Steps[1].DependsOn[0] != "order_shipped" {
		t.Fatal("reference prerequisite not inferred")
	}
	cycle := strings.Replace(string(source), `And content "tracking_number" must not be empty`, `And content "tracking_number" must equal delivery_confirmed.tracking_number`, 1)
	_, result = NewContractValidator().Validate(cycle)
	if result.IsValid {
		t.Fatal("cyclic references accepted")
	}
}
