package validator

import (
	"gopkg.in/yaml.v3"
)

type Contract struct {
	ContractName string           `yaml:"contract_name"`
	Version      int              `yaml:"version"`
	Description  string           `yaml:"description"`
	Parties      []string         `yaml:"parties"`
	Steps        []Step           `yaml:"steps"`
	Groups       []Group          `yaml:"groups,omitempty"`
	Validation   ValidationRules  `yaml:"validation"`
}

type Step struct {
	ID              string            `yaml:"id"`
	Owner           string            `yaml:"owner"`
	Type            string            `yaml:"type,omitempty"`
	DependsOn       []string          `yaml:"depends_on,omitempty"`
	Timeout         string            `yaml:"timeout,omitempty"`
	BusinessContext map[string]string `yaml:"business_context,omitempty"`
}

// UnmarshalYAML implements custom unmarshaling for Step to handle depends_on as string or array
func (s *Step) UnmarshalYAML(value *yaml.Node) error {
	// Create a temporary struct with all fields except DependsOn
	type stepAlias struct {
		ID              string            `yaml:"id"`
		Owner           string            `yaml:"owner"`
		Type            string            `yaml:"type,omitempty"`
		Timeout         string            `yaml:"timeout,omitempty"`
		BusinessContext map[string]string `yaml:"business_context,omitempty"`
	}
	
	var alias stepAlias
	if err := value.Decode(&alias); err != nil {
		return err
	}
	
	// Copy basic fields
	s.ID = alias.ID
	s.Owner = alias.Owner
	s.Type = alias.Type
	s.Timeout = alias.Timeout
	s.BusinessContext = alias.BusinessContext
	
	// Handle depends_on specially
	for i := 0; i < len(value.Content); i += 2 {
		if value.Content[i].Value == "depends_on" {
			dependsOnNode := value.Content[i+1]
			
			// Check if it's a sequence (array)
			if dependsOnNode.Kind == yaml.SequenceNode {
				var deps []string
				if err := dependsOnNode.Decode(&deps); err != nil {
					return err
				}
				s.DependsOn = deps
			} else {
				// It's a scalar (single string)
				var dep string
				if err := dependsOnNode.Decode(&dep); err != nil {
					return err
				}
				s.DependsOn = []string{dep}
			}
			break
		}
	}
	
	return nil
}

type Group struct {
	ID    string     `yaml:"id"`
	Steps []string   `yaml:"steps"`
	Rules GroupRules `yaml:"rules"`
}

type GroupRules struct {
	AllMustSucceed      *bool  `yaml:"all_must_succeed,omitempty"`
	MaxCombinedDuration string `yaml:"max_combined_duration,omitempty"`
}

type ValidationRules struct {
	MaxDuration string `yaml:"max_duration"`
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
	Parties    []string        `json:"parties"`
	Steps      []Step          `json:"steps"`
	Groups     []Group         `json:"groups,omitempty"`
	Validation ValidationRules `json:"validation"`
}
