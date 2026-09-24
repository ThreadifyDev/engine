package contractcontent

import (
	"fmt"
	"math/big"
	"slices"
	"sort"
)

// Contradictions finds content constraints that cannot all be satisfied. It is
// deliberately conservative: unconstrained references and intersections of
// two arbitrary regexes are left for runtime validation.
func Contradictions(rules []Rule) []error {
	byField := map[string][]Rule{}
	for _, rule := range rules {
		if rule.Reference == nil {
			byField[rule.Field] = append(byField[rule.Field], rule)
		}
	}
	fields := make([]string, 0, len(byField))
	for field := range byField {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	var errors []error
	for _, field := range fields {
		if err := fieldContradiction(field, byField[field]); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

func fieldContradiction(field string, rules []Rule) error {
	var candidates []string
	finite := false
	var lower, upper *big.Rat
	lowerStrict, upperStrict := false, false
	hasNumber, hasBoolean := false, false
	for _, rule := range rules {
		switch rule.Operator {
		case "equals":
			if !finite {
				candidates = []string{rule.Value}
				finite = true
			} else if !slices.Contains(candidates, rule.Value) {
				candidates = nil
			} else {
				candidates = []string{rule.Value}
			}
		case "one_of":
			if !finite {
				candidates = append([]string(nil), rule.Values...)
				finite = true
			} else {
				candidates = slices.DeleteFunc(candidates, func(candidate string) bool { return !slices.Contains(rule.Values, candidate) })
			}
		case "number":
			hasNumber = true
		case "boolean":
			hasBoolean = true
		case "gt", "gte":
			hasNumber = true
			bound, err := number(rule.Value)
			if err != nil {
				continue // Validate reports malformed bounds separately.
			}
			strict := rule.Operator == "gt"
			if lower == nil || bound.Cmp(lower) > 0 || (bound.Cmp(lower) == 0 && strict) {
				lower, lowerStrict = bound, strict
			}
		case "lt", "lte":
			hasNumber = true
			bound, err := number(rule.Value)
			if err != nil {
				continue
			}
			strict := rule.Operator == "lt"
			if upper == nil || bound.Cmp(upper) < 0 || (bound.Cmp(upper) == 0 && strict) {
				upper, upperStrict = bound, strict
			}
		}
	}
	if finite {
		for _, candidate := range candidates {
			if Check(rules, map[string]string{field: candidate}) == nil {
				return nil
			}
		}
		return fmt.Errorf("content field %q has no value satisfying all literal rules", field)
	}
	if hasNumber && hasBoolean {
		return fmt.Errorf("content field %q cannot be both a number and a boolean", field)
	}
	if hasBoolean {
		if Check(rules, map[string]string{field: "true"}) != nil && Check(rules, map[string]string{field: "false"}) != nil {
			return fmt.Errorf("content field %q has no boolean value satisfying all rules", field)
		}
		return nil
	}
	if lower != nil && upper != nil {
		comparison := lower.Cmp(upper)
		if comparison > 0 || (comparison == 0 && (lowerStrict || upperStrict)) {
			return fmt.Errorf("content field %q has incompatible numeric bounds", field)
		}
	}
	return nil
}
