package validator

type Contract struct {
	ContractName  string          `yaml:"contract_name"`
	Version       int             `yaml:"version"`
	Description   string          `yaml:"description"`
	EntryPoints   []string        `yaml:"entry_points,omitempty"`
	Parties       []string        `yaml:"parties"`
	Steps         []Step          `yaml:"steps"`
	Transitions   []Transition    `yaml:"transitions,omitempty"`
	TerminalSteps []string        `yaml:"terminal_steps,omitempty"`
	Groups        []Group         `yaml:"groups,omitempty"`
	Validation    ValidationRules `yaml:"validation"`
	Versioning    VersioningRules `yaml:"versioning,omitempty"`
}

type Step struct {
	ID              string           `yaml:"id"`
	Owner           string           `yaml:"owner"`
	Type            string           `yaml:"type,omitempty"`
	DependsOn       []string         `yaml:"depends_on,omitempty"`
	Timeout         string           `yaml:"timeout,omitempty"`
	BusinessContext *BusinessContext `yaml:"business_context,omitempty"`
}

type Group struct {
	ID      string     `yaml:"id"`
	Steps   []string   `yaml:"steps"`
	Rules   GroupRules `yaml:"rules"`
	Timeout string     `yaml:"timeout,omitempty"`
}

type GroupRules struct {
	AllMustSucceed      *bool  `yaml:"all_must_succeed,omitempty"`
	MaxCombinedDuration string `yaml:"max_combined_duration,omitempty"`
}

type ValidationRules struct {
	MaxDuration               string `yaml:"max_duration"`
	AllowMultipleTerminals    bool   `yaml:"allow_multiple_terminals,omitempty"`
	MultipleTerminalsSeverity string `yaml:"multiple_terminals_severity,omitempty"`
}

type BusinessContext struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

type Transition struct {
	From       string   `yaml:"from"`
	To         []string `yaml:"to"`
	Timeout    string   `yaml:"timeout,omitempty"`
	MaxRetries int      `yaml:"max_retries,omitempty"`
}

type VersioningRules struct {
	ThreadsLockToVersion bool `yaml:"threads_lock_to_version,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationResult struct {
	IsValid bool              `json:"isValid"`
	Errors  []ValidationError `json:"errors"`
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
