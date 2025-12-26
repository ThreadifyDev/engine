package models

// ContractGraph represents the DAG structure of a contract
// Metadata (contract_id, version) is stored in contract_versions table
type ContractGraph struct {
	Graph Graph `json:"graph"`
}

// Graph contains the nodes and final step of the workflow
type Graph struct {
	Nodes     map[string]GraphNode `json:"nodes"`      // Key: step_id
	FinalStep string               `json:"final_step"` // Last step in workflow
}

// GraphNode represents a step or parallel group in the workflow
type GraphNode struct {
	ID              string            `json:"id"`
	Role            string            `json:"role,omitempty"` // Required role to execute this step (e.g., "buyer", "seller")
	Type            string            `json:"type"`           // "step" or "parallel_group"
	Mode            string            `json:"mode,omitempty"` // "all_of" or "any_of" for groups
	Required        bool              `json:"required"`
	DependsOn       []string          `json:"depends_on"`
	Next            []string          `json:"next"`            // Steps that depend on this
	Steps           []string          `json:"steps,omitempty"` // For parallel_group type
	Timeout         string            `json:"timeout,omitempty"`
	MaxDuration     string            `json:"max_duration,omitempty"` // For groups
	BusinessContext map[string]string `json:"business_context,omitempty"`
	ParentGroup     string            `json:"parent_group,omitempty"`
}

// ContractYAML represents the parsed YAML/JSON contract
type ContractYAML struct {
	ContractName string      `yaml:"contract_name" json:"ContractName"`
	Version      int         `yaml:"version" json:"Version"`
	Description  string      `yaml:"description" json:"Description"`
	Parties      []string    `yaml:"parties" json:"Parties"`
	Steps        []Step      `yaml:"steps" json:"Steps"`
	Groups       []Group     `yaml:"groups,omitempty" json:"Groups,omitempty"`
	Validation   *Validation `yaml:"validation,omitempty" json:"Validation,omitempty"`
}

// Step represents a workflow step
type Step struct {
	ID              string            `yaml:"id" json:"ID"`
	Role            string            `yaml:"role,omitempty" json:"Role,omitempty"`
	DependsOn       []string          `yaml:"depends_on,omitempty" json:"DependsOn,omitempty"`
	Timeout         string            `yaml:"timeout,omitempty" json:"Timeout,omitempty"`
	BusinessContext map[string]string `yaml:"business_context,omitempty" json:"BusinessContext,omitempty"`
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
	MaxDuration string `yaml:"max_duration,omitempty" json:"MaxDuration,omitempty"`
}
