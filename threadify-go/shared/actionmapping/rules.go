// Package actionmapping defines Engine-managed captured input to contract step rules.
package actionmapping

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var ErrConflict = errors.New("action mappings changed; reload before saving")

const MaxRules = 100

type Rule struct {
	expression *regexp.Regexp
	Action     string `json:"action"`
	Contract   string `json:"contract"`
	Version    int    `json:"version"`
	Step       string `json:"step"`
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
		rule.expression = nil
		if expression, ok := strings.CutPrefix(rule.Action, "regex:"); ok {
			if expression == "" {
				return nil, errors.New("input mapping regex expression is empty")
			}
			var err error
			rule.expression, err = regexp.Compile(expression)
			if err != nil {
				return nil, fmt.Errorf("invalid input mapping regex %q: %w", rule.Action, err)
			}
		} else {
			if rule.Action == "*" || strings.Contains(strings.TrimSuffix(rule.Action, "*"), "*") {
				return nil, errors.New("action patterns support only one trailing * wildcard; use regex: for regular expressions")
			}
			if strings.Contains(rule.Action, ",") {
				return nil, errors.New("action names in mappings cannot contain commas")
			}
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

// Match prefers an exact action, then the longest prefix, then the first matching regex.
// Regex order is authored order, scoped to the thread's contract and version.
func Match(rules []Rule, contract string, version int, action string) (Rule, bool) {
	best, length := Rule{}, -1
	regexMatch, regexFound := Rule{}, false
	for _, rule := range rules {
		if rule.Contract != contract || rule.Version != version {
			continue
		}
		if expression, ok := strings.CutPrefix(rule.Action, "regex:"); ok {
			if !regexFound {
				compiled := rule.expression
				// Loaded settings are normalized; allow direct callers to supply raw rules too.
				if compiled == nil && expression != "" {
					compiled, _ = regexp.Compile(expression)
				}
				if compiled != nil && compiled.MatchString(action) {
					regexMatch, regexFound = rule, true
				}
			}
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
	if length >= 0 {
		return best, true
	}
	return regexMatch, regexFound
}
