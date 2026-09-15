package dto

// ContractGraphDTO represents the DAG structure of a contract for JSON serialization
type ContractGraphDTO struct {
	SemanticsVersion   int                 `json:"semantics_version,omitempty"`
	Graph              GraphDTO            `json:"graph"`
	Transitions        []Transition        `json:"transitions,omitempty"`
	Validation         *Validation         `json:"validation,omitempty"`
	Parties            []string            `json:"parties,omitempty"`
	NotificationConfig *NotificationConfig `json:"notification_config,omitempty"`
}

// GraphDTO contains the nodes, entry points, and terminal steps of the workflow
type GraphDTO struct {
	Nodes         map[string]GraphNode `json:"nodes"`
	EntryPoints   []string             `json:"entry_points,omitempty"`
	TerminalSteps []string             `json:"terminal_steps,omitempty"`
	FinalStep     string               `json:"final_step,omitempty"`
}

// GraphNode represents a step or parallel group in the workflow
type GraphNode struct {
	ID              string           `json:"id"`
	Owner           string           `json:"owner,omitempty"`
	Role            string           `json:"role,omitempty"`
	Type            string           `json:"type"`
	Mode            string           `json:"mode,omitempty"`
	Required        bool             `json:"required"`
	DependsOn       []string         `json:"depends_on,omitempty"`
	Next            []string         `json:"next,omitempty"`
	Steps           []string         `json:"steps,omitempty"`
	Timeout         string           `json:"timeout,omitempty"`
	MaxDuration     string           `json:"max_duration,omitempty"`
	BusinessContext *BusinessContext `json:"business_context,omitempty"`
	ParentGroup     string           `json:"parent_group,omitempty"`
}

// BusinessContext represents the required and optional fields for a step
type BusinessContext struct {
	Required []string `json:"required,omitempty"`
	Optional []string `json:"optional,omitempty"`
}

// Validation defines contract-level validation rules
type Validation struct {
	MaxDuration               string `json:"MaxDuration,omitempty"`
	AllowMultipleTerminals    bool   `json:"AllowMultipleTerminals,omitempty"`
	MultipleTerminalsSeverity string `json:"MultipleTerminalsSeverity,omitempty"`
}

// Transition represents a valid step-to-step flow
type Transition struct {
	From       string   `json:"From"`
	To         []string `json:"To"`
	Timeout    string   `json:"Timeout,omitempty"`
	MaxRetries int      `json:"MaxRetries,omitempty"`
}

// NotificationConfig defines notification scope configuration for a contract
type NotificationConfig struct {
	DefaultScope string            `json:"DefaultScope,omitempty"`
	RoleDefaults map[string]string `json:"RoleDefaults,omitempty"`
}
