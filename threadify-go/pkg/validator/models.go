package validator

import "github.com/threadify/engine/pkg/contractcontent"

type Contract struct {
	ContractName  string
	Version       int
	Description   string
	Includes      []ContractInclude `json:"includes,omitempty"`
	EntryPoints   []string
	Parties       []string
	Steps         []Step
	Transitions   []Transition
	TerminalSteps []string
	Groups        []Group
	Validation    ValidationRules
	Versioning    VersioningRules
}

// ContractInclude pins a version of another contract in the same company.
type ContractInclude struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type Step struct {
	ID              string
	Description     string `json:"description,omitempty"`
	Owner           string
	Type            string
	FreshDependsOn  []string `json:"fresh_depends_on,omitempty"`
	DependsOn       []string
	Timeout         string
	BusinessContext *BusinessContext
	ContentRules    []contractcontent.Rule         `json:"content_rules,omitempty"`
	SemanticRules   []contractcontent.SemanticRule `json:"semantic_rules,omitempty"`
}

type Group struct {
	ID      string
	Steps   []string
	Rules   GroupRules
	Timeout string
}

type GroupRules struct {
	AllMustSucceed      *bool
	MaxCombinedDuration string
}

type ValidationRules struct {
	MaxDuration               string
	AllowMultipleTerminals    bool
	MultipleTerminalsSeverity string
}

type BusinessContext struct {
	Required     []string
	Optional     []string
	Descriptions map[string]string `json:"descriptions,omitempty"`
}

type Transition struct {
	From       string
	To         []string
	Timeout    string
	MaxRetries int
}

type VersioningRules struct {
	ThreadsLockToVersion bool
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationResult struct {
	IsValid  bool              `json:"isValid"`
	Errors   []ValidationError `json:"errors"`
	Warnings []string          `json:"warnings,omitempty"`
}

type ContractContent struct {
	EntryPoints   []string        `json:"entryPoints,omitempty"`
	Parties       []string        `json:"parties"`
	Steps         []Step          `json:"steps"`
	Transitions   []Transition    `json:"transitions,omitempty"`
	TerminalSteps []string        `json:"terminalSteps,omitempty"`
	Groups        []Group         `json:"groups,omitempty"`
	Validation    ValidationRules `json:"validation"`
	Versioning    VersioningRules `json:"versioning,omitempty"`
}
