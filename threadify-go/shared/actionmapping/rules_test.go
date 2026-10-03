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
