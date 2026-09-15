package validator

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/threadify/engine/pkg/contractcontent"
	"gopkg.in/yaml.v3"
)

// IsGherkin detects the first meaningful line; comments and a UTF-8 BOM are allowed.
func IsGherkin(source string) bool {
	for _, line := range strings.Split(strings.TrimPrefix(source, "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return strings.HasPrefix(line, "Feature:")
	}
	return false
}

// NormalizeSource compiles the supported Gherkin-style vocabulary to the same
// YAML model as legacy contracts. Unsupported sentences never become comments.
func NormalizeSource(source string) (string, error) {
	if !IsGherkin(source) {
		return source, nil
	}
	contract, err := ParseGherkin(source)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(contract)
	return string(data), err
}

const quoted = `("(?:[^"\\]|\\.)*")`

var (
	whenStep              = regexp.MustCompile(`^When step ` + quoted + ` is submitted$`)
	ownerRule             = regexp.MustCompile(`^owner must be ` + quoted + `$`)
	freshDependencyRule   = regexp.MustCompile(`^step ` + quoted + ` must succeed before each invocation$`)
	dependencyRule        = regexp.MustCompile(`^step ` + quoted + ` must have succeeded$`)
	contextRule           = regexp.MustCompile(`^content ` + quoted + ` (must be present|is optional)$`)
	timeoutRule           = regexp.MustCompile(`^this step must finish within ` + quoted + `$`)
	nextRule              = regexp.MustCompile(`^next step must be one of (.+)$`)
	transitionTimeoutRule = regexp.MustCompile(`^the next step must start within ` + quoted + `$`)
	retryRule             = regexp.MustCompile(`^this step may be retried at most ([0-9]+) times$`)
	durationRule          = regexp.MustCompile(`^the thread must finish within ` + quoted + `$`)
	contentTypeRule       = regexp.MustCompile(`^content ` + quoted + ` must (not be empty|be a number|be a boolean)$`)
	contentBoundRule      = regexp.MustCompile(`^content ` + quoted + ` must be a number (greater than or equal to|less than or equal to|greater than|less than) (\S+)$`)
	contentEnumRule       = regexp.MustCompile(`^content ` + quoted + ` must be one of (.+)$`)
	contentEqualRule      = regexp.MustCompile(`^content ` + quoted + ` must equal ` + quoted + `$`)

	contentReferenceRule = regexp.MustCompile(`^content ` + quoted + ` must equal ([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)
	quotedItem           = regexp.MustCompile(`^` + quoted)
)

// ParseGherkin parses a deliberately bounded rule language, not executable
// Cucumber scenarios. Feature metadata precedes one Rule block per reported step.
func ParseGherkin(source string) (*Contract, error) {
	c := &Contract{Version: 1}
	stepIndex := -1
	block := ""
	waitingWhen, seenThen, seenBackground := false, false, false
	metadata := map[string]bool{}
	seen := map[string]bool{}
	lineNumber := 0
	fail := func(message string) (*Contract, error) { return nil, fmt.Errorf("line %d: %s", lineNumber, message) }
	unique := func(key string) bool {
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}
	finishRule := func() error {
		if block != "rule" {
			return nil
		}
		if waitingWhen {
			return fmt.Errorf("Rule requires: When step \"name\" is submitted")
		}
		if !seenThen || c.Steps[stepIndex].Owner == "" {
			return fmt.Errorf("each step requires: Then owner must be \"party\"")
		}
		return nil
	}
	transition := func() *Transition {
		id := c.Steps[stepIndex].ID
		for i := range c.Transitions {
			if c.Transitions[i].From == id {
				return &c.Transitions[i]
			}
		}
		c.Transitions = append(c.Transitions, Transition{From: id})
		return &c.Transitions[len(c.Transitions)-1]
	}
	for i, raw := range strings.Split(strings.TrimPrefix(source, "\ufeff"), "\n") {
		lineNumber = i + 1
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if c.ContractName == "" {
			if !strings.HasPrefix(line, "Feature:") {
				return fail("expected Feature: contract_name")
			}
			c.ContractName = strings.TrimSpace(strings.TrimPrefix(line, "Feature:"))
			if c.ContractName == "" {
				return fail("Feature name cannot be empty")
			}
			c.Description = c.ContractName
			continue
		}
		if strings.HasPrefix(line, "Version:") || strings.HasPrefix(line, "Description:") {
			key, value, _ := strings.Cut(line, ":")
			if block != "" || metadata[key] {
				return fail("metadata must appear once, before Background or Rule")
			}
			metadata[key] = true
			value = strings.TrimSpace(value)
			if key == "Version" {
				version, err := strconv.Atoi(value)
				if err != nil || version < 1 {
					return fail("Version must be a positive integer")
				}
				c.Version = version
			} else {
				if value == "" {
					return fail("Description cannot be empty")
				}
				c.Description = value
			}
			continue
		}
		if strings.HasPrefix(line, "Rule:") || line == "Background:" {
			if err := finishRule(); err != nil {
				return fail(err.Error())
			}
			seen = map[string]bool{}
			seenThen = false
			if line == "Background:" {
				if seenBackground || len(c.Steps) > 0 || block == "rule" {
					return fail("Background must appear once, before all Rules")
				}
				seenBackground = true
				block = "background"
			} else {
				if strings.TrimSpace(strings.TrimPrefix(line, "Rule:")) == "" {
					return fail("Rule title cannot be empty")
				}
				block = "rule"
				waitingWhen = true
			}
			continue
		}
		if block == "background" {
			prefix := "And "
			if !seenThen {
				prefix = "Given "
			}
			if !strings.HasPrefix(line, prefix) {
				return fail("Background requires Given followed by And clauses")
			}
			clause := strings.TrimPrefix(line, prefix)
			seenThen = true
			if m := durationRule.FindStringSubmatch(clause); m != nil {
				if !unique("duration") {
					return fail("duplicate thread duration rule")
				}
				value, err := unquoteContract(m[1])
				if err != nil {
					return fail(err.Error())
				}
				c.Validation.MaxDuration = value
			} else if clause == "multiple terminal steps are allowed" {
				if !unique("terminals") {
					return fail("duplicate multiple-terminal rule")
				}
				c.Validation.AllowMultipleTerminals = true
			} else {
				return fail("unsupported Background clause: " + clause)
			}
			continue
		}
		if block != "rule" {
			return fail("expected Background: or Rule:")
		}
		if waitingWhen {
			m := whenStep.FindStringSubmatch(line)
			if m == nil {
				return fail("Rule requires: When step \"name\" is submitted")
			}
			id, err := unquoteContract(m[1])
			if err != nil {
				return fail(err.Error())
			}
			for _, s := range c.Steps {
				if s.ID == id {
					return fail("duplicate step " + strconv.Quote(id) + "; put its constraints in one Rule")
				}
			}
			c.Steps = append(c.Steps, Step{ID: id})
			stepIndex = len(c.Steps) - 1
			waitingWhen = false
			continue
		}
		prefix := "And "
		if !seenThen {
			prefix = "Then "
		}
		if !strings.HasPrefix(line, prefix) {
			return fail("step constraints require Then followed by And clauses")
		}
		clause := strings.TrimPrefix(line, prefix)
		seenThen = true
		step := &c.Steps[stepIndex]
		switch {
		case ownerRule.MatchString(clause):
			if !unique("owner") {
				return fail("duplicate owner rule")
			}
			value, err := unquoteContract(ownerRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			step.Owner = value
			if !slices.Contains(c.Parties, value) {
				c.Parties = append(c.Parties, value)
			}
		case freshDependencyRule.MatchString(clause):
			value, err := unquoteContract(freshDependencyRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("dependency:" + value) {
				return fail("duplicate prerequisite")
			}
			step.FreshDependsOn = append(step.FreshDependsOn, value)
		case dependencyRule.MatchString(clause):
			value, err := unquoteContract(dependencyRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("dependency:" + value) {
				return fail("duplicate prerequisite")
			}
			step.DependsOn = append(step.DependsOn, value)
		case contextRule.MatchString(clause):
			m := contextRule.FindStringSubmatch(clause)
			value, err := unquoteContract(m[1])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("context:" + value) {
				return fail("duplicate or conflicting content rule")
			}
			if step.BusinessContext == nil {
				step.BusinessContext = &BusinessContext{}
			}
			if m[2] == "must be present" {
				step.BusinessContext.Required = append(step.BusinessContext.Required, value)
			} else {
				step.BusinessContext.Optional = append(step.BusinessContext.Optional, value)
			}
		case contentTypeRule.MatchString(clause), contentBoundRule.MatchString(clause), contentEnumRule.MatchString(clause), contentEqualRule.MatchString(clause), contentReferenceRule.MatchString(clause):
			rule, err := parseContentRule(clause)
			if err != nil {
				return fail(err.Error())
			}
			if !unique("content-rule:" + rule.Field + ":" + rule.Operator) {
				return fail("duplicate content constraint")
			}
			step.ContentRules = append(step.ContentRules, rule)

		case clause == "this step is an entry point":
			if !unique("entry") {
				return fail("duplicate entry point rule")
			}
			c.EntryPoints = append(c.EntryPoints, step.ID)
		case clause == "this step is terminal":
			if !unique("terminal") {
				return fail("duplicate terminal rule")
			}
			c.TerminalSteps = append(c.TerminalSteps, step.ID)
		case timeoutRule.MatchString(clause):
			if !unique("timeout") {
				return fail("duplicate step timeout rule")
			}
			value, err := unquoteContract(timeoutRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			step.Timeout = value
		case nextRule.MatchString(clause):
			if !unique("next") {
				return fail("duplicate next-step rule")
			}
			values, err := quotedContractList(nextRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			transition().To = values
		case transitionTimeoutRule.MatchString(clause):
			if !unique("transition_timeout") {
				return fail("duplicate transition timeout rule")
			}
			value, err := unquoteContract(transitionTimeoutRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			transition().Timeout = value
		case retryRule.MatchString(clause):
			if !unique("retries") {
				return fail("duplicate retry rule")
			}
			value, err := strconv.Atoi(retryRule.FindStringSubmatch(clause)[1])
			if err != nil || value < 1 {
				return fail("retry limit must be a positive integer; zero means unlimited in existing contracts")
			}
			transition().MaxRetries = value
		default:
			return fail("unsupported step constraint: " + clause)
		}
	}
	if c.ContractName == "" {
		return fail("expected Feature: contract_name")
	}
	if err := finishRule(); err != nil {
		return fail(err.Error())
	}
	if len(c.Steps) == 0 {
		return fail("at least one Rule with a step is required")
	}
	for _, t := range c.Transitions {
		if len(t.To) == 0 {
			return fail("transition timeout/retry rules require a next-step rule for " + strconv.Quote(t.From))
		}
	}
	return c, nil
}

// unquoteContract accepts escaped quoted strings without allowing empty identifiers.
func unquoteContract(value string) (string, error) {
	result, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("invalid quoted value: %w", err)
	}
	if strings.TrimSpace(result) == "" || strings.ContainsAny(result, "\r\n\x00") {
		return "", fmt.Errorf("quoted values must be nonempty single-line strings")
	}
	return result, nil
}

// quotedContractList consumes the entire list, rejecting trailing text and duplicates.
func quotedContractList(value string) ([]string, error) {
	var result []string
	for {
		value = strings.TrimSpace(value)
		m := quotedItem.FindStringSubmatch(value)
		if m == nil {
			return nil, fmt.Errorf("expected a comma-separated list of quoted step names")
		}
		item, err := unquoteContract(m[1])
		if err != nil {
			return nil, err
		}
		if slices.Contains(result, item) {
			return nil, fmt.Errorf("duplicate name in list: %s", item)
		}
		result = append(result, item)
		value = strings.TrimSpace(value[len(m[0]):])
		if value == "" {
			return result, nil
		}
		if !strings.HasPrefix(value, ",") {
			return nil, fmt.Errorf("expected comma between quoted names")
		}
		value = value[1:]
	}
}

// parseContentRule maps supported sentences to literal operators with no executable code.
func parseContentRule(clause string) (contractcontent.Rule, error) {
	var r contractcontent.Rule
	var field string
	switch {
	case contentTypeRule.MatchString(clause):
		m := contentTypeRule.FindStringSubmatch(clause)
		field = m[1]
		r.Operator = map[string]string{"not be empty": "nonempty", "be a number": "number", "be a boolean": "boolean"}[m[2]]
	case contentBoundRule.MatchString(clause):
		m := contentBoundRule.FindStringSubmatch(clause)
		field = m[1]
		r.Value = m[3]
		r.Operator = map[string]string{"greater than": "gt", "greater than or equal to": "gte", "less than": "lt", "less than or equal to": "lte"}[m[2]]
	case contentEnumRule.MatchString(clause):
		m := contentEnumRule.FindStringSubmatch(clause)
		field = m[1]
		r.Operator = "one_of"
		values, err := quotedContractList(m[2])
		if err != nil {
			return r, err
		}
		r.Values = values
	case contentReferenceRule.MatchString(clause):
		m := contentReferenceRule.FindStringSubmatch(clause)
		field = m[1]
		r.Operator = "equals"
		r.Reference = &contractcontent.Reference{Step: m[2], Field: m[3]}
	case contentEqualRule.MatchString(clause):
		m := contentEqualRule.FindStringSubmatch(clause)
		field = m[1]
		r.Operator = "equals"
		value, err := unquoteContract(m[2])
		if err != nil {
			return r, err
		}
		r.Value = value
	}
	value, err := unquoteContract(field)
	if err != nil {
		return r, err
	}
	r.Field = value
	return r, contractcontent.Validate([]contractcontent.Rule{r})
}
