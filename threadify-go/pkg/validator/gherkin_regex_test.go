package validator

import (
	"encoding/json"
	"strings"
	"testing"
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
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded Contract
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	_, result = v.ValidateCompiled(&reloaded)
	if !result.IsValid {
		t.Fatal(result.Errors)
	}
	_, before, err := v.SerializeContract(c)
	if err != nil {
		t.Fatal(err)
	}
	_, after, err := v.SerializeContract(&reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("regex changed during normalization")
	}
	reloaded.Steps[1].ContentRules[0].Value = "["
	if _, result := v.ValidateCompiled(&reloaded); result.IsValid {
		t.Fatal("invalid compiled regex accepted")
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
