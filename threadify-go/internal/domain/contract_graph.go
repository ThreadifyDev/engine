package domain

import "github.com/threadify/engine/pkg/contractcontent"

// ContractGraph represents the DAG structure of a contract
// Metadata (contract_id, version) is stored in contract_versions table
type ContractGraph struct {
	SemanticsVersion   int
	Graph              Graph
	Transitions        []Transition
	Validation         *Validation
	Parties            []string
	NotificationConfig *NotificationConfig
}

// CurrentContractGraphSemantics identifies graphs where depends_on is a
// partial-order constraint rather than a synthesized immediate transition.
const CurrentContractGraphSemantics = 2

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
	Description     string
	Owner           string
	Role            string // Deprecated alias for Owner (kept for backward compatibility)
	Type            string
	Mode            string
	Required        bool
	FreshDependsOn  []string `json:"fresh_depends_on,omitempty"`
	DependsOn       []string
	Next            []string
	Steps           []string
	Timeout         string
	MaxDuration     string
	BusinessContext *BusinessContext
	ContentRules    []contractcontent.Rule         `json:"content_rules,omitempty"`
	SemanticRules   []contractcontent.SemanticRule `json:"semantic_rules,omitempty"`
	ParentGroup     string
}

// ContractDefinition represents a compiled contract
type ContractDefinition struct {
	ContractName       string
	Version            int
	Description        string
	EntryPoints        []string
	Parties            []string
	Steps              []Step
	Transitions        []Transition
	TerminalSteps      []string
	Groups             []Group
	Validation         *Validation
	Versioning         *Versioning
	NotificationConfig *NotificationConfig
}

// Step represents a workflow step
type Step struct {
	ID              string
	Description     string `json:"description,omitempty"`
	Owner           string
	Role            string
	FreshDependsOn  []string `json:"fresh_depends_on,omitempty"`
	DependsOn       []string
	Timeout         string
	BusinessContext *BusinessContext
	ContentRules    []contractcontent.Rule         `json:"content_rules,omitempty"`
	SemanticRules   []contractcontent.SemanticRule `json:"semantic_rules,omitempty"`
}

// Group represents a parallel group of steps
type Group struct {
	ID    string
	Steps []string
	Rules *GroupRules
}

// GroupRules defines execution rules for a group
type GroupRules struct {
	AllMustSucceed      bool
	MaxCombinedDuration string
}

// Validation defines contract-level validation rules
type Validation struct {
	MaxDuration               string
	AllowMultipleTerminals    bool
	MultipleTerminalsSeverity string
}

// Transition represents a valid step-to-step flow
type Transition struct {
	From       string
	To         []string
	Timeout    string
	MaxRetries int
}

// Versioning defines version locking rules
type Versioning struct {
	ThreadsLockToVersion bool
}

// BusinessContext represents the business context structure
type BusinessContext struct {
	Required     []string
	Optional     []string
	Descriptions map[string]string `json:"descriptions,omitempty"`
}

// NotificationConfig defines notification scope configuration for a contract
type NotificationConfig struct {
	DefaultScope string
	RoleDefaults map[string]string
}
