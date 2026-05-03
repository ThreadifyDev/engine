package domain

// ContractGraph represents the DAG structure of a contract
// Metadata (contract_id, version) is stored in contract_versions table
type ContractGraph struct {
	Graph              Graph
	Transitions        []Transition
	Validation         *Validation
	Parties            []string
	NotificationConfig *NotificationConfig
}

// Graph contains the nodes, entry points, and terminal steps of the workflow
type Graph struct {
	Nodes         map[string]GraphNode
	EntryPoints   []string
	TerminalSteps []string
	FinalStep     string
}

// GraphNode represents a step or parallel group in the workflow
type GraphNode struct {
	ID              string
	Owner           string
	Role            string // Deprecated alias for Owner (kept for backward compatibility)
	Type            string
	Mode            string
	Required        bool
	DependsOn       []string
	Next            []string
	Steps           []string
	Timeout         string
	MaxDuration     string
	BusinessContext *BusinessContext
	ParentGroup     string
}

// ContractYAML represents the parsed YAML/JSON contract
type ContractYAML struct {
	ContractName       string              `yaml:"contract_name"`
	Version            int                 `yaml:"version"`
	Description        string              `yaml:"description"`
	EntryPoints        []string            `yaml:"entry_points,omitempty"`
	Parties            []string            `yaml:"parties"`
	Steps              []Step              `yaml:"steps"`
	Transitions        []Transition        `yaml:"transitions,omitempty"`
	TerminalSteps      []string            `yaml:"terminal_steps,omitempty"`
	Groups             []Group             `yaml:"groups,omitempty"`
	Validation         *Validation         `yaml:"validation,omitempty"`
	Versioning         *Versioning         `yaml:"versioning,omitempty"`
	NotificationConfig *NotificationConfig `yaml:"notification_config,omitempty"`
}

// Step represents a workflow step
type Step struct {
	ID              string           `yaml:"id"`
	Owner           string           `yaml:"owner,omitempty"`
	Role            string           `yaml:"role,omitempty"`
	DependsOn       []string         `yaml:"depends_on,omitempty"`
	Timeout         string           `yaml:"timeout,omitempty"`
	BusinessContext *BusinessContext `yaml:"business_context,omitempty"`
}

// Group represents a parallel group of steps
type Group struct {
	ID    string      `yaml:"id"`
	Steps []string    `yaml:"steps"`
	Rules *GroupRules `yaml:"rules,omitempty"`
}

// GroupRules defines execution rules for a group
type GroupRules struct {
	AllMustSucceed      bool   `yaml:"all_must_succeed,omitempty"`
	MaxCombinedDuration string `yaml:"max_combined_duration,omitempty"`
}

// Validation defines contract-level validation rules
type Validation struct {
	MaxDuration               string `yaml:"max_duration,omitempty"`
	AllowMultipleTerminals    bool   `yaml:"allow_multiple_terminals,omitempty"`
	MultipleTerminalsSeverity string `yaml:"multiple_terminals_severity,omitempty"`
}

// Transition represents a valid step-to-step flow
type Transition struct {
	From       string   `yaml:"from"`
	To         []string `yaml:"to"`
	Timeout    string   `yaml:"timeout,omitempty"`
	MaxRetries int      `yaml:"max_retries,omitempty"`
}

// Versioning defines version locking rules
type Versioning struct {
	ThreadsLockToVersion bool `yaml:"threads_lock_to_version,omitempty"`
}

// BusinessContext represents the business context structure
type BusinessContext struct {
	Required []string `yaml:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty"`
}

// NotificationConfig defines notification scope configuration for a contract
type NotificationConfig struct {
	DefaultScope string            `yaml:"default_scope,omitempty"`
	RoleDefaults map[string]string `yaml:"role_defaults,omitempty"`
}
