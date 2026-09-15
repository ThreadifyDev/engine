package validator

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const simpleFeature = `Feature: example
Version: 2
Description: Example contract.
Rule: Record approval
 When step "approval" is submitted
 Then owner must be "reviewer"
 And this step is an entry point
Rule: Record charge
 When step "charge" is submitted
 Then owner must be "processor"
 And step "approval" must have succeeded
 And content "reference" must be present
 And content "note" is optional
 And this step is terminal
`

func TestGherkinCompilesToEquivalentYAML(t *testing.T) {
	v := NewContractValidator()
	c, result := v.Validate("\ufeff# comment\r\n" + simpleFeature)
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	if c.Version != 2 || len(c.Transitions) != 0 || c.Steps[1].DependsOn[0] != "approval" {
		t.Fatalf("wrong semantics: %+v", c)
	}
	yamlSource, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	other, result := v.Validate(string(yamlSource))
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	full, content, err := v.SerializeContract(c)
	if err != nil {
		t.Fatal(err)
	}
	otherFull, otherContent, err := v.SerializeContract(other)
	if err != nil {
		t.Fatal(err)
	}
	if full != otherFull || content != otherContent {
		t.Fatal("source formats produce different persisted semantics")
	}
}

func TestGherkinExample(t *testing.T) {
	source, err := os.ReadFile("../../examples/payment.feature")
	if err != nil {
		t.Fatal(err)
	}
	c, result := NewContractValidator().Validate(string(source))
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	if len(c.Steps[1].ContentRules) != 2 {
		t.Fatalf("content checks lost: %+v", c.Steps[1])
	}
}

func TestGherkinRejectsUnsupportedAndAmbiguousSource(t *testing.T) {
	tests := map[string]string{
		"optional checked field": simpleFeature + " And content \"note\" must not be empty\n",
		"unknown sentence":       simpleFeature + " And the payment should be sensible\n",
		"unsupported scenario":   simpleFeature + "Scenario: charge\n",
		"missing owner":          strings.Replace(simpleFeature, `Then owner must be "processor"`, `Then this step is terminal`, 1),
		"duplicate owner":        simpleFeature + " And owner must be \"another\"\n",
		"unknown prerequisite":   strings.Replace(simpleFeature, `step "approval" must have succeeded`, `step "missing" must have succeeded`, 1),
		"cycle":                  strings.Replace(simpleFeature, `And this step is an entry point`, `And step "charge" must have succeeded`, 1),
		"no steps":               "Feature: empty\n",
		"empty rule":             simpleFeature + "Rule: empty\n",
		"duplicate version":      strings.Replace(simpleFeature, "Version: 2", "Version: 2\nVersion: 3", 1),
		"metadata late":          simpleFeature + "Version: 3\n",
		"empty quoted":           strings.Replace(simpleFeature, `owner must be "processor"`, `owner must be ""`, 1),
		"malformed quotes":       simpleFeature + " And content \"x must be present\n",
		"invalid duration":       simpleFeature + " And this step must finish within \"yesterday\"\n",
		"bad number":             simpleFeature + " And content \"amount\" must be a number greater than NaN\n",
		"empty enum":             simpleFeature + " And content \"currency\" must be one of \n",
		"trailing enum":          simpleFeature + " And content \"currency\" must be one of \"GBP\",\n",
		"conflicting presence":   simpleFeature + " And content \"reference\" is optional\n",
		"duplicate step":         simpleFeature + "Rule: duplicate\n When step \"charge\" is submitted\n Then owner must be \"processor\"\n",
	}
	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			_, result := NewContractValidator().Validate(source)
			if result.IsValid {
				t.Fatal("invalid contract accepted")
			}
			if len(result.Errors) == 0 {
				t.Fatal("missing diagnostics")
			}
		})
	}
	_, err := ParseGherkin(simpleFeature + " And nonsense\n")
	if err == nil || !strings.Contains(err.Error(), "line ") {
		t.Fatalf("missing source location: %v", err)
	}
}

func TestGherkinStrictTransitionsAndContentVocabulary(t *testing.T) {
	source := strings.Replace(simpleFeature, `And this step is an entry point`, `And this step is an entry point
 And next step must be one of "charge"
 And the next step must start within "5m"
 And this step may be retried at most 2 times`, 1) + ` And content "amount" must be a number greater than or equal to 0
 And content "amount" must be a number less than 100
 And content "balance" must be a number less than or equal to 1000
 And content "count" must be a number
 And content "confirmed" must be a boolean
 And content "status" must equal "approved"
 And content "reference" must not be empty
 And content "currency" must be one of "GBP", "USD"
`
	c, result := NewContractValidator().Validate(source)
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	if len(c.Transitions) != 1 || c.Transitions[0].MaxRetries != 2 || c.Transitions[0].Timeout != "5m" || len(c.Steps[1].ContentRules) != 8 {
		t.Fatalf("lost rules: %+v", c)
	}
}
