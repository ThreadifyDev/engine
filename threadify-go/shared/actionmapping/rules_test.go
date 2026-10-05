package actionmapping

import "testing"

func TestNormalizeAndMatch(t *testing.T) {
	rules, err := Normalize([]Rule{
		{Action: "checkout_*", Contract: "order", Version: 1, Step: "checkout"},
		{Action: "checkout_submit", Contract: "order", Version: 1, Step: "submit"},
		{Action: "checkout_confirm", Contract: "order", Version: 1, Step: "submit"},
		{Action: "checkout_*", Contract: "order", Version: 2, Step: "revised_checkout"},
		{Action: "checkout_*", Contract: "other", Version: 1, Step: "other_step"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		contract     string
		version      int
		action, step string
	}{
		{"order", 1, "checkout_submit", "submit"},
		{"order", 1, "checkout_confirm", "submit"},
		{"order", 1, "checkout_review", "checkout"},
		{"order", 2, "checkout_review", "revised_checkout"},
		{"other", 1, "checkout_review", "other_step"},
		{"order", 3, "checkout_review", ""},
		{"order", 1, "ignored", ""},
	} {
		matched, ok := Match(rules, test.contract, test.version, test.action)
		if ok != (test.step != "") || matched.Step != test.step {
			t.Errorf("Match(%q,%d,%q) = %q,%v; want %q", test.contract, test.version, test.action, matched.Step, ok, test.step)
		}
	}
	for _, invalid := range [][]Rule{
		{{Action: "", Contract: "order", Version: 1, Step: "pay"}},
		{{Action: "*", Contract: "order", Version: 1, Step: "pay"}},
		{{Action: "a*b", Contract: "order", Version: 1, Step: "pay"}},
		{{Action: "a,b", Contract: "order", Version: 1, Step: "pay"}},
		{{Action: "a", Contract: "order", Version: 0, Step: "pay"}},
		{{Action: "a", Contract: "order", Version: 1, Step: "pay"}, {Action: "a", Contract: "order", Version: 1, Step: "other"}},
	} {
		if _, err := Normalize(invalid); err == nil {
			t.Errorf("accepted invalid rules: %#v", invalid)
		}
	}
}

func TestRegexMappingPrecedenceAndScope(t *testing.T) {
	raw := []Rule{
		{Action: `regex:(?i)^checkout`, Contract: "other", Version: 1, Step: "wrong_contract"},
		{Action: `regex:(?i)^checkout`, Contract: "order", Version: 2, Step: "wrong_version"},
		{Action: `regex:(?i)^checkout`, Contract: "order", Version: 1, Step: "regex_first"},
		{Action: `regex:(?i)checkout.*`, Contract: "order", Version: 1, Step: "regex_second"},
		{Action: "checkout*", Contract: "order", Version: 1, Step: "prefix"},
		{Action: "checkout.review*", Contract: "order", Version: 1, Step: "long_prefix"},
		{Action: "checkout.review", Contract: "order", Version: 1, Step: "exact"},
		{Action: `regex:^request=[a-z]{1,3}$`, Contract: "order", Version: 1, Step: "query"},
	}
	normalized, err := Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, rules := range [][]Rule{raw, normalized} {
		for _, tc := range []struct{ action, step string }{
			{"CHECKOUT.review", "regex_first"}, {"checkout.review", "exact"},
			{"checkout.review.open", "long_prefix"}, {"checkout.open", "prefix"},
			{"request=abc", "query"}, {"request=abcd", ""}, {"unrelated", ""},
		} {
			rule, ok := Match(rules, "order", 1, tc.action)
			if ok != (tc.step != "") || rule.Step != tc.step {
				t.Errorf("Match(%q)=%q,%v; want %q", tc.action, rule.Step, ok, tc.step)
			}
		}
	}
	// Changing a normalized rule and normalizing again must not retain a stale regex.
	normalized[2].Action = `regex:^changed$`
	changed, err := Normalize(normalized)
	if err != nil {
		t.Fatal(err)
	}
	rule, ok := Match(changed, "order", 1, "CHECKOUT.review")
	if !ok || rule.Step != "regex_second" {
		t.Fatalf("stale compiled regex retained: %#v", rule)
	}
}

func TestInvalidMappingRegex(t *testing.T) {
	for _, expression := range []string{"regex:", "regex:[", "regex:(?=checkout)", `regex:(a)\1`} {
		_, err := Normalize([]Rule{{Action: expression, Contract: "order", Version: 1, Step: "checkout"}})
		if err == nil {
			t.Errorf("accepted %q", expression)
		}
	}
}
