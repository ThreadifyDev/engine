package validator

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/threadify/engine/pkg/contractcontent"
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

const quoted = `("(?:[^"\\]|\\.)*")`

var (
	whenStep              = regexp.MustCompile(`^When step ` + quoted + ` is submitted$`)
	ownerRule             = regexp.MustCompile(`^owner must be ` + quoted + `$`)
	stepDescriptionRule   = regexp.MustCompile(`^step description is ` + quoted + `$`)
	stepTypeRule          = regexp.MustCompile(`^step type is ` + quoted + `$`)
	freshDependencyRule   = regexp.MustCompile(`^step ` + quoted + ` must succeed before each invocation$`)
	dependencyRule        = regexp.MustCompile(`^step ` + quoted + ` must have succeeded$`)
	contextRule           = regexp.MustCompile(`^content ` + quoted + ` (must be present|is optional)$`)
	timeoutRule           = regexp.MustCompile(`^this step must finish within ` + quoted + `$`)
	nextRule              = regexp.MustCompile(`^next step must be one of (.+)$`)
	transitionTimeoutRule = regexp.MustCompile(`^the next step must start within ` + quoted + `$`)
	retryRule             = regexp.MustCompile(`^this step may be retried at most ([0-9]+) times$`)
	durationRule          = regexp.MustCompile(`^the thread must finish within ` + quoted + `$`)
	terminalSeverityRule  = regexp.MustCompile(`^multiple terminal severity is ` + quoted + `$`)
	contentTypeRule       = regexp.MustCompile(`^content ` + quoted + ` must (not be empty|be a number|be a boolean)$`)
	contentBoundRule      = regexp.MustCompile(`^content ` + quoted + ` must be a number (greater than or equal to|less than or equal to|greater than|less than) (\S+)$`)
	contentEnumRule       = regexp.MustCompile(`^content ` + quoted + ` must be one of (.+)$`)
	contentEqualRule      = regexp.MustCompile(`^content ` + quoted + ` must equal ` + quoted + `$`)
	contentRegexRule      = regexp.MustCompile(`^content ` + quoted + ` must match regex ` + quoted + `$`)

	contentReferenceRule   = regexp.MustCompile(`^content ` + quoted + ` must equal ([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)$`)
	semanticQuestionRule   = regexp.MustCompile(`^content ` + quoted + ` must satisfy question ` + quoted + `$`)
	semanticContextRule    = regexp.MustCompile(`^semantic context for content ` + quoted + ` is (.+)$`)
	semanticConfidenceRule = regexp.MustCompile(`^semantic confidence for content ` + quoted + ` is (\S+)$`)
	parallelStepsRule      = regexp.MustCompile(`^parallel steps are (.+)$`)
	groupDurationRule      = regexp.MustCompile(`^combined duration must be within ` + quoted + `$`)
	groupTitleRule         = regexp.MustCompile(`^Group: ` + quoted + `$`)
	stepTitleRule          = regexp.MustCompile(`^Step: ` + quoted + `$`)
	definitionDescription  = regexp.MustCompile(`^Description: ` + quoted + `$`)
	definitionContext      = regexp.MustCompile(`^(Required|Optional) context: ` + quoted + `(?: means ` + quoted + `)?$`)
	legacyStepDescription  = regexp.MustCompile(`^description is ` + quoted + `$`)
	legacyStepContext      = regexp.MustCompile(`^(required|optional) context ` + quoted + `(?: means ` + quoted + `)?$`)
	quotedItem             = regexp.MustCompile(`^` + quoted)
	includeRule            = regexp.MustCompile(`^Include: ([A-Za-z_][A-Za-z0-9_]*):([1-9][0-9]*)$`)
)

// ParseGherkin parses a deliberately bounded rule language, not executable
// Cucumber scenarios. Feature metadata precedes Step definitions and one Rule
// block per executable step.
func ParseGherkin(source string) (*Contract, error) {
	c := &Contract{Version: 1}
	type stepDefinition struct {
		Description string
		Context     BusinessContext
	}
	definitions := map[string]*stepDefinition{}
	var definitionID string
	explicitRuleDescriptions := map[string]bool{}
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
	var ruleTitle string
	finishBlock := func() error {
		if block == "group" && len(c.Groups[len(c.Groups)-1].Steps) < 2 {
			return fmt.Errorf("Group requires at least two parallel steps")
		}
		if block == "step" && definitions[definitionID].Description == "" {
			return fmt.Errorf("Step requires: Description: \"business meaning\"")
		}
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
		if strings.HasPrefix(line, "Version:") || (strings.HasPrefix(line, "Description:") && block != "step") {
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
		if strings.HasPrefix(line, "Include:") {
			if block != "" {
				return fail("Include must appear before Background or Rule")
			}
			m := includeRule.FindStringSubmatch(line)
			if m == nil {
				return fail("Include must use contract_name:positive_version")
			}
			version, err := strconv.Atoi(m[2])
			if err != nil || version < 1 {
				return fail("Include version must be a positive integer")
			}
			for _, included := range c.Includes {
				if included.Name == m[1] {
					return fail("contract may be included only once: " + m[1])
				}
			}
			c.Includes = append(c.Includes, ContractInclude{Name: m[1], Version: version})
			continue
		}
		if strings.HasPrefix(line, "Rule:") || line == "Background:" || strings.HasPrefix(line, "Group:") || strings.HasPrefix(line, "Step:") {
			if err := finishBlock(); err != nil {
				return fail(err.Error())
			}
			seen = map[string]bool{}
			seenThen = false
			if line == "Background:" {
				if seenBackground || block != "" || len(c.Steps) > 0 {
					return fail("Background must appear once, before Step, Group, or Rule blocks")
				}
				seenBackground = true
				block = "background"
			} else if strings.HasPrefix(line, "Step:") {
				m := stepTitleRule.FindStringSubmatch(line)
				if m == nil {
					return fail("Step requires a quoted name")
				}
				id, err := unquoteContract(m[1])
				if err != nil {
					return fail(err.Error())
				}
				if definitions[id] != nil {
					return fail("duplicate Step definition " + strconv.Quote(id))
				}
				definitions[id] = &stepDefinition{}
				definitionID = id
				block = "step"
			} else if strings.HasPrefix(line, "Group:") {
				m := groupTitleRule.FindStringSubmatch(line)
				if m == nil {
					return fail("Group requires a quoted name")
				}
				name, err := unquoteContract(m[1])
				if err != nil {
					return fail(err.Error())
				}
				for _, group := range c.Groups {
					if group.ID == name {
						return fail("duplicate group " + strconv.Quote(name))
					}
				}
				c.Groups = append(c.Groups, Group{ID: name})
				block = "group"
			} else {
				ruleTitle = strings.TrimSpace(strings.TrimPrefix(line, "Rule:"))
				if ruleTitle == "" {
					return fail("Rule title cannot be empty")
				}
				block = "rule"
				waitingWhen = true
			}
			continue
		}
		if block == "step" {
			clause := line
			legacy := false
			if strings.HasPrefix(line, "Given ") || strings.HasPrefix(line, "And ") {
				clause = strings.TrimPrefix(strings.TrimPrefix(line, "Given "), "And ")
				legacy = true
			}
			definition := definitions[definitionID]
			switch {
			case (!legacy && definitionDescription.MatchString(clause)) || (legacy && legacyStepDescription.MatchString(clause)):
				if !unique("description") {
					return fail("duplicate Step description")
				}
				m := definitionDescription.FindStringSubmatch(clause)
				if legacy {
					m = legacyStepDescription.FindStringSubmatch(clause)
				}
				value, err := unquoteContract(m[1])
				if err != nil {
					return fail(err.Error())
				}
				definition.Description = value
			case (!legacy && definitionContext.MatchString(clause)) || (legacy && legacyStepContext.MatchString(clause)):
				m := definitionContext.FindStringSubmatch(clause)
				if legacy {
					m = legacyStepContext.FindStringSubmatch(clause)
				}
				field, err := unquoteContract(m[2])
				if err != nil {
					return fail(err.Error())
				}
				if !unique("context:" + field) {
					return fail("duplicate or conflicting Step context field " + strconv.Quote(field))
				}
				if strings.EqualFold(m[1], "required") {
					definition.Context.Required = append(definition.Context.Required, field)
				} else {
					definition.Context.Optional = append(definition.Context.Optional, field)
				}
				if m[3] != "" {
					meaning, err := unquoteContract(m[3])
					if err != nil {
						return fail(err.Error())
					}
					if definition.Context.Descriptions == nil {
						definition.Context.Descriptions = map[string]string{}
					}
					definition.Context.Descriptions[field] = meaning
				}
			default:
				return fail("unsupported Step clause: " + clause)
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
			} else if m := terminalSeverityRule.FindStringSubmatch(clause); m != nil {
				if !unique("terminal-severity") {
					return fail("duplicate terminal severity rule")
				}
				value, err := unquoteContract(m[1])
				if err != nil {
					return fail(err.Error())
				}
				c.Validation.MultipleTerminalsSeverity = value
			} else if clause == "threads lock to this version" {
				if !unique("version-lock") {
					return fail("duplicate version lock rule")
				}
				c.Versioning.ThreadsLockToVersion = true
			} else {
				return fail("unsupported Background clause: " + clause)
			}
			continue
		}
		if block == "group" {
			prefix := "And "
			if !seenThen {
				prefix = "Given "
			}
			if !strings.HasPrefix(line, prefix) {
				return fail("Group requires Given followed by And clauses")
			}
			clause := strings.TrimPrefix(line, prefix)
			seenThen = true
			group := &c.Groups[len(c.Groups)-1]
			switch {
			case parallelStepsRule.MatchString(clause):
				if !unique("steps") {
					return fail("duplicate parallel steps clause")
				}
				steps, err := quotedContractList(parallelStepsRule.FindStringSubmatch(clause)[1])
				if err != nil {
					return fail(err.Error())
				}
				group.Steps = steps
			case clause == "all parallel steps must succeed":
				if !unique("all") {
					return fail("duplicate group success rule")
				}
				value := true
				group.Rules.AllMustSucceed = &value
			case groupDurationRule.MatchString(clause):
				if !unique("duration") {
					return fail("duplicate group duration rule")
				}
				value, err := unquoteContract(groupDurationRule.FindStringSubmatch(clause)[1])
				if err != nil {
					return fail(err.Error())
				}
				group.Rules.MaxCombinedDuration = value
			default:
				return fail("unsupported Group clause: " + clause)
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
			c.Steps = append(c.Steps, Step{ID: id, Description: ruleTitle})
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
		case stepDescriptionRule.MatchString(clause):
			if !unique("description") {
				return fail("duplicate step description")
			}
			value, err := unquoteContract(stepDescriptionRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			step.Description = value
			explicitRuleDescriptions[step.ID] = true
		case stepTypeRule.MatchString(clause):
			if !unique("type") {
				return fail("duplicate step type")
			}
			value, err := unquoteContract(stepTypeRule.FindStringSubmatch(clause)[1])
			if err != nil {
				return fail(err.Error())
			}
			step.Type = value
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
		case contentTypeRule.MatchString(clause), contentBoundRule.MatchString(clause), contentEnumRule.MatchString(clause), contentEqualRule.MatchString(clause), contentReferenceRule.MatchString(clause), contentRegexRule.MatchString(clause):
			rule, err := parseContentRule(clause)
			if err != nil {
				return fail(err.Error())
			}
			ruleKey := "content-rule:" + rule.Field + ":" + rule.Operator
			if rule.Operator == "equals" {
				ruleKey += ":" + rule.Value
			}
			if !unique(ruleKey) {
				return fail("duplicate content constraint")
			}
			step.ContentRules = append(step.ContentRules, rule)
		case semanticQuestionRule.MatchString(clause):
			m := semanticQuestionRule.FindStringSubmatch(clause)
			field, err := unquoteContract(m[1])
			if err != nil {
				return fail(err.Error())
			}
			question, err := unquoteContract(m[2])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("semantic:" + field) {
				return fail("duplicate semantic question for " + field)
			}
			step.SemanticRules = append(step.SemanticRules, contractcontent.SemanticRule{Field: field, Question: question})
		case semanticContextRule.MatchString(clause):
			m := semanticContextRule.FindStringSubmatch(clause)
			field, err := unquoteContract(m[1])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("semantic-context:" + field) {
				return fail("duplicate semantic context for " + field)
			}
			values, err := quotedContractList(m[2])
			if err != nil {
				return fail(err.Error())
			}
			rule := semanticRuleForField(step, field)
			if rule == nil {
				return fail("semantic context requires a question for " + field)
			}
			rule.ContextSteps = values
		case semanticConfidenceRule.MatchString(clause):
			m := semanticConfidenceRule.FindStringSubmatch(clause)
			field, err := unquoteContract(m[1])
			if err != nil {
				return fail(err.Error())
			}
			if !unique("semantic-confidence:" + field) {
				return fail("duplicate semantic confidence for " + field)
			}
			rule := semanticRuleForField(step, field)
			if rule == nil {
				return fail("semantic confidence requires a question for " + field)
			}
			value, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				return fail("semantic confidence must be a number")
			}
			rule.MinProbability = value

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
	if err := finishBlock(); err != nil {
		return fail(err.Error())
	}
	if len(c.Steps) == 0 && len(c.Includes) == 0 {
		return fail("at least one Rule or Include is required")
	}
	for _, t := range c.Transitions {
		if len(t.To) == 0 {
			return fail("transition timeout/retry rules require a next-step rule for " + strconv.Quote(t.From))
		}
	}
	for id, definition := range definitions {
		index := -1
		for i := range c.Steps {
			if c.Steps[i].ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			return fail("Step definition " + strconv.Quote(id) + " has no matching Rule")
		}
		step := &c.Steps[index]
		if explicitRuleDescriptions[id] {
			return fail("Step description is declared in both Step and Rule for " + strconv.Quote(id))
		}
		step.Description = definition.Description
		if len(definition.Context.Required) == 0 && len(definition.Context.Optional) == 0 {
			continue
		}
		if step.BusinessContext == nil {
			step.BusinessContext = &BusinessContext{}
		}
		for _, field := range definition.Context.Required {
			if slices.Contains(step.BusinessContext.Optional, field) {
				return fail("context field " + strconv.Quote(field) + " is both required and optional")
			}
			if !slices.Contains(step.BusinessContext.Required, field) {
				step.BusinessContext.Required = append(step.BusinessContext.Required, field)
			}
		}
		for _, field := range definition.Context.Optional {
			if slices.Contains(step.BusinessContext.Required, field) {
				return fail("context field " + strconv.Quote(field) + " is both required and optional")
			}
			if !slices.Contains(step.BusinessContext.Optional, field) {
				step.BusinessContext.Optional = append(step.BusinessContext.Optional, field)
			}
		}
		if len(definition.Context.Descriptions) > 0 {
			step.BusinessContext.Descriptions = definition.Context.Descriptions
		}
	}
	// A content rule always requires its field. Register it in the compiled
	// context so runtime projection retains it even without a presence clause.
	for i := range c.Steps {
		step := &c.Steps[i]
		for _, rule := range step.ContentRules {
			if step.BusinessContext == nil {
				step.BusinessContext = &BusinessContext{}
			}
			if !slices.Contains(step.BusinessContext.Required, rule.Field) && !slices.Contains(step.BusinessContext.Optional, rule.Field) {
				step.BusinessContext.Required = append(step.BusinessContext.Required, rule.Field)
			}
		}
		for _, rule := range step.SemanticRules {
			if step.BusinessContext == nil {
				step.BusinessContext = &BusinessContext{}
			}
			if !slices.Contains(step.BusinessContext.Required, rule.Field) && !slices.Contains(step.BusinessContext.Optional, rule.Field) {
				step.BusinessContext.Required = append(step.BusinessContext.Required, rule.Field)
			}
		}
	}
	return c, nil
}

func semanticRuleForField(step *Step, field string) *contractcontent.SemanticRule {
	for i := range step.SemanticRules {
		if step.SemanticRules[i].Field == field {
			return &step.SemanticRules[i]
		}
	}
	return nil
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
	case contentRegexRule.MatchString(clause):
		m := contentRegexRule.FindStringSubmatch(clause)
		field = m[1]
		r.Operator = "matches"
		value, err := unquoteContract(m[2])
		if err != nil {
			return r, err
		}
		r.Value = value
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
