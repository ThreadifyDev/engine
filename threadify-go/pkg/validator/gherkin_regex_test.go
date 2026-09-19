package validator

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGherkinRegexRoundTrip(t *testing.T) {
	source := simpleFeature + ` And content "reference" must match regex "^TRK-\\d{8}$"` + "\n"
	v := NewContractValidator()
	c, result := v.Validate(source)
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	rule := c.Steps[1].ContentRules[0]
	if rule.Operator != "matches" || rule.Value != `^TRK-\d{8}$` {
		t.Fatalf("incorrect compiled rule: %+v", rule)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, result := v.Validate(string(data))
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	_, before, err := v.SerializeContract(c)
	if err != nil {
		t.Fatal(err)
	}
	_, after, err := v.SerializeContract(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("regex changed during normalization")
	}
	reloaded.Steps[1].ContentRules[0].Value = "["
	data, err = yaml.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if _, result := v.Validate(string(data)); result.IsValid {
		t.Fatal("invalid YAML regex accepted")
	}
}

func TestGherkinRejectsInvalidRegex(t *testing.T) {
	for _, clause := range []string{
		`content "reference" must match regex "["`,
		`content "reference" must match regex "(?=a)"`,
		`content "reference" must match regex "(a)\\1"`,
		`content "reference" must match regex ""`,
		`content "reference" must match regex "\d+"`,
		`content "reference" must match regex "a" trailing`,
		`content "reference" must match regex approval.reference`,
		`content "reference" must match regex "` + strings.Repeat("a", 4097) + `"`,
	} {
		_, err := ParseGherkin(simpleFeature + " And " + clause + "\n")
		if err == nil || !strings.Contains(err.Error(), "line 15:") {
			t.Fatalf("expected regex error at source line: %v", err)
		}
	}
	for _, source := range []string{
		simpleFeature + " And content \"note\" must match regex \".+\"\n",
		simpleFeature + " And content \"reference\" must match regex \"a\"\n And content \"reference\" must match regex \"b\"\n",
	} {
		if _, result := NewContractValidator().Validate(source); result.IsValid {
			t.Fatal("conflicting regex rule accepted")
		}
	}
}
