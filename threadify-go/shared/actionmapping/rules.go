// Package actionmapping defines Engine-managed captured input to contract step rules.
package actionmapping

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrConflict = errors.New("action mappings changed; reload before saving")

const MaxRules = 100

type Rule struct {
	Action   string `json:"action"`
	Contract string `json:"contract"`
	Version  int    `json:"version"`
	Step     string `json:"step"`
}

type Settings struct {
	Rules     []Rule     `json:"rules"`
	Revision  string     `json:"revision"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type Store interface {
	Load(context.Context, string) (Settings, error)
	Save(context.Context, string, string, []Rule) (Settings, error)
}

func Normalize(rules []Rule) ([]Rule, error) {
	if len(rules) > MaxRules {
		return nil, fmt.Errorf("at most %d action mappings are allowed", MaxRules)
	}
	out := make([]Rule, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.Version < 1 {
			return nil, errors.New("contract version must be a positive integer")
		}
		for _, value := range []string{rule.Action, rule.Contract, rule.Step} {
			if value == "" || value != strings.TrimSpace(value) || len(value) > 128 || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return nil, errors.New("action, contract, and step must be nonblank UTF-8 names of at most 128 bytes without control characters")
			}
		}
		if rule.Action == "*" || strings.Contains(strings.TrimSuffix(rule.Action, "*"), "*") {
			return nil, errors.New("action patterns support only one trailing * wildcard")
		}
		if strings.Contains(rule.Action, ",") {
			return nil, errors.New("action names in mappings cannot contain commas")
		}
		key := fmt.Sprintf("%s\x00%d\x00%s", rule.Contract, rule.Version, rule.Action)
		if seen[key] {
			return nil, fmt.Errorf("duplicate action mapping for %s in %s version %d", rule.Action, rule.Contract, rule.Version)
		}
		seen[key] = true
		out = append(out, rule)
	}
	return out, nil
}

// Match prefers an exact action, then the longest matching prefix.
func Match(rules []Rule, contract string, version int, action string) (Rule, bool) {
	best, length := Rule{}, -1
	for _, rule := range rules {
		if rule.Contract != contract || rule.Version != version {
			continue
		}
		if rule.Action == action {
			return rule, true
		}
		if strings.HasSuffix(rule.Action, "*") {
			prefix := strings.TrimSuffix(rule.Action, "*")
			if strings.HasPrefix(action, prefix) && len(prefix) > length {
				best, length = rule, len(prefix)
			}
		}
	}
	return best, length >= 0
}
