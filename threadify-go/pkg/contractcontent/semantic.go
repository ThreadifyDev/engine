package contractcontent

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// SemanticRule asks one bounded yes/no question about a candidate content
// field, optionally grounded in successful earlier step content.
type SemanticRule struct {
	Field          string   `json:"field"`
	Question       string   `json:"question"`
	ContextSteps   []string `json:"context_steps,omitempty"`
	MinProbability float64  `json:"min_probability,omitempty"`
}

func ValidateSemantic(rules []SemanticRule) error {
	for _, rule := range rules {
		if !referenceName.MatchString(rule.Field) {
			return fmt.Errorf("semantic rule field must be a simple identifier")
		}
		if len(strings.TrimSpace(rule.Question)) < 5 || len(rule.Question) > 1000 {
			return fmt.Errorf("semantic rule question must be 5–1000 characters")
		}
		if math.IsNaN(rule.MinProbability) || math.IsInf(rule.MinProbability, 0) || rule.MinProbability != 0 && (rule.MinProbability < 0.5 || rule.MinProbability > 1) {
			return fmt.Errorf("semantic rule min_probability must be between 0.5 and 1")
		}
		if len(rule.ContextSteps) > 12 {
			return fmt.Errorf("semantic rule has too many context steps")
		}
		for _, step := range rule.ContextSteps {
			if !referenceName.MatchString(step) {
				return fmt.Errorf("semantic context step must be a simple identifier")
			}
		}
	}
	return nil
}

func SemanticDependencies(existing []string, rules []SemanticRule) []string {
	result := append([]string(nil), existing...)
	for _, rule := range rules {
		for _, step := range rule.ContextSteps {
			if !slices.Contains(result, step) {
				result = append(result, step)
			}
		}
	}
	return result
}
