// Package contractcontent defines portable, deterministic checks for the string
// context submitted with a Threadify step. It does not execute expressions.
package contractcontent

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Rule is persisted with the contract and graph so preview and runtime use the same checks.
type Reference struct {
	Step  string `json:"step"`
	Field string `json:"field"`
}

// Rule holds a literal operand or an explicit reference, never both.
type Rule struct {
	Reference *Reference `json:"reference,omitempty"`
	Field     string     `json:"field"`
	Operator  string     `json:"operator"`
	Value     string     `json:"value,omitempty"`
	Values    []string   `json:"values,omitempty"`
}

var referenceName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var numeric = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// number accepts finite JSON-style numbers, rejecting NaN, Infinity and loose coercions.
func number(value string) (*big.Rat, error) {
	if len(value) > 4096 || !numeric.MatchString(value) {
		return nil, fmt.Errorf("expected a JSON-style number")
	}
	// Bound exponent work before constructing an exact rational; decimal money
	// comparisons must not round distinct values to the same float64.
	if i := strings.IndexAny(value, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(value[i+1:])
		if err != nil || exponent < -308 || exponent > 308 {
			return nil, fmt.Errorf("numeric exponent must be between -308 and 308")
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, fmt.Errorf("expected a finite number")
	}
	exact, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, fmt.Errorf("expected a number")
	}
	return exact, nil
}

// Validate rejects unsupported or malformed rules before a contract can be published.
func Validate(rules []Rule) error {
	for _, r := range rules {
		if strings.TrimSpace(r.Field) == "" {
			return fmt.Errorf("content rule field cannot be empty")
		}
		if r.Reference != nil {
			if r.Operator != "equals" || r.Value != "" || len(r.Values) > 0 {
				return fmt.Errorf("references require equals without a literal operand")
			}
			if !referenceName.MatchString(r.Reference.Step) || !referenceName.MatchString(r.Reference.Field) {
				return fmt.Errorf("references require simple step and field identifiers")
			}
		}
		switch r.Operator {
		case "nonempty", "number", "boolean":
			if r.Value != "" || len(r.Values) > 0 {
				return fmt.Errorf("%s does not accept operands", r.Operator)
			}
		case "equals":
			if len(r.Values) > 0 {
				return fmt.Errorf("equals accepts one value")
			}
		case "matches":
			if len(r.Values) > 0 {
				return fmt.Errorf("matches accepts one regex pattern")
			}
			if _, err := compilePattern(r.Value); err != nil {
				return fmt.Errorf("%s regex: %w", r.Field, err)
			}
		case "one_of":
			if len(r.Values) == 0 || r.Value != "" {
				return fmt.Errorf("one_of requires a nonempty values list")
			}
		case "gt", "gte", "lt", "lte":
			if _, err := number(r.Value); err != nil {
				return fmt.Errorf("%s bound: %w", r.Field, err)
			}
			if len(r.Values) > 0 {
				return fmt.Errorf("numeric bounds accept one value")
			}
		default:
			return fmt.Errorf("unsupported content operator %q", r.Operator)
		}
	}
	return nil
}

// Check requires each referenced field and evaluates its literal value. All
// rules are conjunctive; missing or malformed values cannot satisfy a comparison.
func Check(rules []Rule, content map[string]string) error {
	return CheckWithReferences(rules, content, nil)
}

// CheckWithReferences resolves explicit references from the caller's bound thread.
// A missing reader fails closed; quoted literal operands never invoke it.
func CheckWithReferences(rules []Rule, content map[string]string, resolve func(Reference) (string, error)) error {
	if err := Validate(rules); err != nil {
		return err
	}
	for _, r := range rules {
		value, ok := content[r.Field]
		if !ok {
			return fmt.Errorf("required content field %q is missing", r.Field)
		}
		valid := false
		switch r.Operator {
		case "nonempty":
			valid = strings.TrimSpace(value) != ""
		case "boolean":
			valid = value == "true" || value == "false"
		case "equals":
			expected := r.Value
			if r.Reference != nil {
				if resolve == nil {
					return fmt.Errorf("step reference %s.%s requires thread context", r.Reference.Step, r.Reference.Field)
				}
				resolved, err := resolve(*r.Reference)
				if err != nil {
					return err
				}
				expected = resolved
			}
			valid = value == expected
		case "one_of":
			valid = slices.Contains(r.Values, value)
		case "matches":
			pattern, err := compilePattern(r.Value)
			if err != nil {
				return fmt.Errorf("%s regex: %w", r.Field, err)
			}
			valid = pattern.MatchString(value)
		case "number":
			_, err := number(value)
			valid = err == nil
		case "gt", "gte", "lt", "lte":
			n, err := number(value)
			bound, _ := number(r.Value)
			if err == nil {
				switch r.Operator {
				case "gt":
					valid = n.Cmp(bound) > 0
				case "gte":
					valid = n.Cmp(bound) >= 0
				case "lt":
					valid = n.Cmp(bound) < 0
				case "lte":
					valid = n.Cmp(bound) <= 0
				}
			}
		}
		if !valid {
			return fmt.Errorf("content field %q violates %s rule", r.Field, r.Operator)
		}
	}
	return nil
}

// Dependencies includes referenced steps because a comparison needs an earlier
// success, even when the author omits a separate prerequisite sentence.
func Dependencies(existing []string, rules []Rule) []string {
	result := append([]string(nil), existing...)
	for _, rule := range rules {
		if rule.Reference != nil && !slices.Contains(result, rule.Reference.Step) {
			result = append(result, rule.Reference.Step)
		}
	}
	return result
}
