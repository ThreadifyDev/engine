package models

// ContractGraph represents the DAG structure of a contract
// Metadata (contract_id, version) is stored in contract_versions table
type ContractGraph struct {
	Graph              Graph               `json:"graph"`
	Transitions        []Transition        `json:"transitions,omitempty"`         // Valid step-to-step transitions
	Validation         *Validation         `json:"validation,omitempty"`          // Contract-level validation rules
	Parties            []string            `json:"parties,omitempty"`             // Contract parties
	NotificationConfig *NotificationConfig `json:"notification_config,omitempty"` // Notification scope configuration
}

// Graph contains the nodes, entry points, and terminal steps of the workflow
type Graph struct {
	Nodes         map[string]GraphNode `json:"nodes"`          // Key: step_id
	EntryPoints   []string             `json:"entry_points"`   // Steps where threads can start
	TerminalSteps []string             `json:"terminal_steps"` // Steps where threads end
	FinalStep     string               `json:"final_step,omitempty"`
}

// GraphNode represents a step or parallel group in the workflow
type GraphNode struct {
	ID              string      `json:"id"`
	Owner           string      `json:"owner,omitempty"` // Party that owns/executes this step (e.g., "merchant", "payment_processor")
	Role            string      `json:"role,omitempty"`  // Deprecated alias for Owner (kept for backward compatibility)
	Type            string      `json:"type"`            // "step" or "parallel_group"
	Mode            string      `json:"mode,omitempty"`  // "all_of" or "any_of" for groups
	Required        bool        `json:"required"`
	DependsOn       []string    `json:"depends_on,omitempty"` // Steps that must complete before this step
	Next            []string    `json:"next"`            // Steps that can be transitioned to from this step
	Steps           []string    `json:"steps,omitempty"` // For parallel_group type
	Timeout         string      `json:"timeout,omitempty"`
	MaxDuration     string      `json:"max_duration,omitempty"`     // For groups
	BusinessContext interface{} `json:"business_context,omitempty"` // Can be BusinessContextV3 struct
	ParentGroup     string      `json:"parent_group,omitempty"`
}

// ContractYAML represents the parsed YAML/JSON contract
type ContractYAML struct {
	ContractName       string              `yaml:"contract_name" json:"ContractName"`
	Version            int                 `yaml:"version" json:"Version"`
	Description        string              `yaml:"description" json:"Description"`
	EntryPoints        []string            `yaml:"entry_points,omitempty" json:"EntryPoints,omitempty"`
	Parties            []string            `yaml:"parties" json:"Parties"`
	Steps              []Step              `yaml:"steps" json:"Steps"`
	Transitions        []Transition        `yaml:"transitions,omitempty" json:"Transitions,omitempty"`
	TerminalSteps      []string            `yaml:"terminal_steps,omitempty" json:"TerminalSteps,omitempty"`
	Groups             []Group             `yaml:"groups,omitempty" json:"Groups,omitempty"`
	Validation         *Validation         `yaml:"validation,omitempty" json:"Validation,omitempty"`
	Versioning         *Versioning         `yaml:"versioning,omitempty" json:"Versioning,omitempty"`
	NotificationConfig *NotificationConfig `yaml:"notification_config,omitempty" json:"NotificationConfig,omitempty"`
}

// Step represents a workflow step
type Step struct {
	ID              string           `yaml:"id" json:"ID"`
	Owner           string           `yaml:"owner,omitempty" json:"Owner,omitempty"` // Party that owns/executes this step
	Role            string           `yaml:"role,omitempty" json:"Role,omitempty"`   // Deprecated alias for Owner (kept for backward compatibility)
	DependsOn       []string         `yaml:"depends_on,omitempty" json:"DependsOn,omitempty"`
	Timeout         string           `yaml:"timeout,omitempty" json:"Timeout,omitempty"`
	BusinessContext *BusinessContext `yaml:"business_context,omitempty" json:"BusinessContext,omitempty"`
}

// Group represents a parallel group of steps
type Group struct {
	ID    string      `yaml:"id" json:"ID"`
	Steps []string    `yaml:"steps" json:"Steps"`
	Rules *GroupRules `yaml:"rules,omitempty" json:"Rules,omitempty"`
}

// GroupRules defines execution rules for a group
type GroupRules struct {
	AllMustSucceed      bool   `yaml:"all_must_succeed,omitempty" json:"AllMustSucceed,omitempty"`
	MaxCombinedDuration string `yaml:"max_combined_duration,omitempty" json:"MaxCombinedDuration,omitempty"`
}

// Validation defines contract-level validation rules
type Validation struct {
	MaxDuration               string `yaml:"max_duration,omitempty" json:"MaxDuration,omitempty"`
	AllowMultipleTerminals    bool   `yaml:"allow_multiple_terminals,omitempty" json:"AllowMultipleTerminals,omitempty"`
	MultipleTerminalsSeverity string `yaml:"multiple_terminals_severity,omitempty" json:"MultipleTerminalsSeverity,omitempty"`
}

// Transition represents a valid step-to-step flow
type Transition struct {
	From       string   `yaml:"from" json:"From"`
	To         []string `yaml:"to" json:"To"`
	CanRetry   bool     `yaml:"can_retry,omitempty" json:"CanRetry,omitempty"`
	MaxRetries int      `yaml:"max_retries,omitempty" json:"MaxRetries,omitempty"`
}

// Versioning defines version locking rules
type Versioning struct {
	ThreadsLockToVersion bool `yaml:"threads_lock_to_version,omitempty" json:"ThreadsLockToVersion,omitempty"`
}

// BusinessContext represents the business context structure
type BusinessContext struct {
	Required []string `yaml:"required,omitempty" json:"required,omitempty"`
	Optional []string `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// NotificationConfig defines notification scope configuration for a contract
type NotificationConfig struct {
	DefaultScope string            `yaml:"default_scope,omitempty" json:"DefaultScope,omitempty"` // Default scope for parties (owner, participant, observer)
	RoleDefaults map[string]string `yaml:"role_defaults,omitempty" json:"RoleDefaults,omitempty"` // Role-specific scope overrides
}
