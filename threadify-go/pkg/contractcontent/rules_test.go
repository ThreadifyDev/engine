package contractcontent

import "testing"

func TestContentRules(t *testing.T) {
	tests := []struct {
		name             string
		rule             Rule
		value            string
		missing, allowed bool
	}{
		{"positive", Rule{Field: "v", Operator: "gt", Value: "0"}, "12.50", false, true},
		{"zero", Rule{Field: "v", Operator: "gt", Value: "0"}, "0", false, false},
		{"negative", Rule{Field: "v", Operator: "gt", Value: "0"}, "-1", false, false},
		{"exact large decimal", Rule{Field: "v", Operator: "gt", Value: "9007199254740992"}, "9007199254740993", false, true},
		{"scientific", Rule{Field: "v", Operator: "gte", Value: "100"}, "1e2", false, true},
		{"upper bound", Rule{Field: "v", Operator: "lt", Value: "100"}, "100", false, false},
		{"inclusive upper bound", Rule{Field: "v", Operator: "lte", Value: "100"}, "100", false, true},
		{"NaN", Rule{Field: "v", Operator: "number"}, "NaN", false, false},
		{"infinity", Rule{Field: "v", Operator: "number"}, "Infinity", false, false},
		{"overflow", Rule{Field: "v", Operator: "number"}, "1e999999999", false, false},
		{"huge negative exponent", Rule{Field: "v", Operator: "number"}, "1e-999999999", false, false},
		{"no coercion", Rule{Field: "v", Operator: "number"}, " 5 ", false, false},
		{"missing", Rule{Field: "v", Operator: "number"}, "", true, false},
		{"enum", Rule{Field: "v", Operator: "one_of", Values: []string{"GBP", "USD"}}, "GBP", false, true},
		{"enum case", Rule{Field: "v", Operator: "one_of", Values: []string{"GBP"}}, "gbp", false, false},
		{"blank", Rule{Field: "v", Operator: "nonempty"}, " \t", false, false},
		{"text", Rule{Field: "v", Operator: "nonempty"}, "ok", false, true},
		{"false boolean", Rule{Field: "v", Operator: "boolean"}, "false", false, true},
		{"invalid boolean", Rule{Field: "v", Operator: "boolean"}, "yes", false, false},
		{"equals", Rule{Field: "v", Operator: "equals", Value: "approved"}, "approved", false, true},
		{"unequal", Rule{Field: "v", Operator: "equals", Value: "approved"}, "declined", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := map[string]string{}
			if !tt.missing {
				content["v"] = tt.value
			}
			err := Check([]Rule{tt.rule}, content)
			if (err == nil) != tt.allowed {
				t.Fatalf("allowed=%v err=%v", tt.allowed, err)
			}
		})
	}
}

func TestRejectMalformedRules(t *testing.T) {
	for _, r := range []Rule{{Field: "v", Operator: "eval", Value: "true"}, {Field: "v", Operator: "gt", Value: "NaN"}, {Field: "v", Operator: "one_of"}, {Operator: "number"}, {Field: "v", Operator: "number", Value: "1"}} {
		if err := Validate([]Rule{r}); err == nil {
			t.Fatalf("accepted malformed rule %+v", r)
		}
	}
}
