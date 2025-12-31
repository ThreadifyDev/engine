# Contract V3 Validation Implementation Plan

This document outlines the implementation plan for adding Contract V3 validation rules to the existing contract upload system.

---

## Current State

### Existing Contract Structure

**File:** `pkg/validator/models.go`

```go
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
    BusinessContext map[string]string `yaml:"business_context,omitempty"` // Currently string->string
}

type ValidationRules struct {
    MaxDuration string `yaml:"max_duration"`
}
```

### Existing Validation (in `pkg/validator/contract.go`)

✅ **Already validates:**
- Contract name format (alphanumeric + underscores)
- Version is positive integer
- Description not empty
- No duplicate step IDs
- Step owners are defined parties
- Step type is valid (managed, human_in_loop, external)
- DependsOn references exist
- Timeout format is valid
- BusinessContext field types are valid
- All parties assigned to at least one step
- Group steps exist
- Max duration format

---

## Required Changes for Contract V3

### 1. Update Data Models

#### A. Add New Fields to Contract Structure

**File:** `pkg/validator/models.go`

```go
type Contract struct {
    ContractName string           `yaml:"contract_name"`
    Version      int              `yaml:"version"`
    Description  string           `yaml:"description"`
    EntryPoints  []string         `yaml:"entry_points"`           // NEW
    Parties      []string         `yaml:"parties"`
    Steps        []Step           `yaml:"steps"`
    Transitions  []Transition     `yaml:"transitions"`            // NEW
    TerminalSteps []string        `yaml:"terminal_steps"`         // NEW
    Groups       []Group          `yaml:"groups,omitempty"`
    Validation   ValidationRules  `yaml:"validation"`
    Versioning   VersioningRules  `yaml:"versioning,omitempty"`   // NEW
}

type Step struct {
    ID              string            `yaml:"id"`
    Owner           string            `yaml:"owner"`
    Type            string            `yaml:"type,omitempty"`
    DependsOn       []string          `yaml:"depends_on,omitempty"`
    Timeout         string            `yaml:"timeout,omitempty"`
    BusinessContext *BusinessContext  `yaml:"business_context,omitempty"` // CHANGED
}

// NEW: BusinessContext with required/optional fields
type BusinessContext struct {
    Required []string `yaml:"required,omitempty"`
    Optional []string `yaml:"optional,omitempty"`
}

// NEW: Transition structure
type Transition struct {
    From     string   `yaml:"from"`
    To       []string `yaml:"to"`
    CanRetry bool     `yaml:"can_retry,omitempty"`
}

// UPDATED: ValidationRules with new fields
type ValidationRules struct {
    MaxDuration              string `yaml:"max_duration"`
    AllowMultipleTerminals   bool   `yaml:"allow_multiple_terminals,omitempty"`   // NEW
    MultipleTerminalsSeverity string `yaml:"multiple_terminals_severity,omitempty"` // NEW
}

// NEW: Versioning rules
type VersioningRules struct {
    ThreadsLockToVersion bool `yaml:"threads_lock_to_version,omitempty"`
}
```

---

### 2. Add New Validation Rules

**File:** `pkg/validator/contract.go`

Add these validation functions to the `Validate()` method:

#### Rule 1: Entry Points Must Exist in Steps

```go
func (v *ContractValidator) validateEntryPoints(contract *Contract, stepIds map[string]bool) []ValidationError {
    errors := []ValidationError{}
    
    // At least one entry point required
    if len(contract.EntryPoints) == 0 {
        errors = append(errors, ValidationError{
            Field:   "entry_points",
            Message: "At least one entry point is required",
        })
        return errors
    }
    
    // All entry points must reference valid steps
    for _, entryPoint := range contract.EntryPoints {
        if !stepIds[entryPoint] {
            errors = append(errors, ValidationError{
                Field:   "entry_points",
                Message: fmt.Sprintf("Entry point '%s' is not defined in steps", entryPoint),
            })
        }
    }
    
    return errors
}
```

#### Rule 2: Terminal Steps Must Exist in Steps

```go
func (v *ContractValidator) validateTerminalSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
    errors := []ValidationError{}
    
    // At least one terminal step required
    if len(contract.TerminalSteps) == 0 {
        errors = append(errors, ValidationError{
            Field:   "terminal_steps",
            Message: "At least one terminal step is required",
        })
        return errors
    }
    
    // All terminal steps must reference valid steps
    for _, terminalStep := range contract.TerminalSteps {
        if !stepIds[terminalStep] {
            errors = append(errors, ValidationError{
                Field:   "terminal_steps",
                Message: fmt.Sprintf("Terminal step '%s' is not defined in steps", terminalStep),
            })
        }
    }
    
    return errors
}
```

#### Rule 3: Terminal Steps Must Be Reachable

```go
func (v *ContractValidator) validateTerminalStepsReachable(contract *Contract) []ValidationError {
    errors := []ValidationError{}
    
    // Build set of steps that appear in transitions' "to" field
    reachableSteps := make(map[string]bool)
    for _, transition := range contract.Transitions {
        for _, toStep := range transition.To {
            reachableSteps[toStep] = true
        }
    }
    
    // Check each terminal step is reachable
    for _, terminalStep := range contract.TerminalSteps {
        if !reachableSteps[terminalStep] {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("terminal_steps.%s", terminalStep),
                Message: fmt.Sprintf("Terminal step '%s' is not reachable from any transition", terminalStep),
            })
        }
    }
    
    return errors
}
```

#### Rule 4: All Transition Steps Must Exist

```go
func (v *ContractValidator) validateTransitionSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
    errors := []ValidationError{}
    
    for i, transition := range contract.Transitions {
        // Validate "from" step exists
        if !stepIds[transition.From] {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("transitions[%d].from", i),
                Message: fmt.Sprintf("Step '%s' is not defined in steps", transition.From),
            })
        }
        
        // Validate all "to" steps exist
        for _, toStep := range transition.To {
            if !stepIds[toStep] {
                errors = append(errors, ValidationError{
                    Field:   fmt.Sprintf("transitions[%d].to", i),
                    Message: fmt.Sprintf("Step '%s' is not defined in steps", toStep),
                })
            }
        }
    }
    
    return errors
}
```

#### Rule 5: No Orphaned Steps

```go
func (v *ContractValidator) validateNoOrphanedSteps(contract *Contract, stepIds map[string]bool) []ValidationError {
    errors := []ValidationError{}
    
    // Build sets
    entryPointSet := make(map[string]bool)
    for _, ep := range contract.EntryPoints {
        entryPointSet[ep] = true
    }
    
    terminalStepSet := make(map[string]bool)
    for _, ts := range contract.TerminalSteps {
        terminalStepSet[ts] = true
    }
    
    // Build incoming and outgoing transition maps
    hasIncoming := make(map[string]bool)
    hasOutgoing := make(map[string]bool)
    
    for _, transition := range contract.Transitions {
        hasOutgoing[transition.From] = true
        for _, toStep := range transition.To {
            hasIncoming[toStep] = true
        }
    }
    
    // Check each step
    for stepId := range stepIds {
        isEntryPoint := entryPointSet[stepId]
        isTerminal := terminalStepSet[stepId]
        incoming := hasIncoming[stepId]
        outgoing := hasOutgoing[stepId]
        
        // Entry points don't need incoming, terminals don't need outgoing
        // But all other steps need at least one of each
        if !isEntryPoint && !incoming && !isTerminal {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("steps.%s", stepId),
                Message: fmt.Sprintf("Step '%s' has no incoming transitions and is not an entry point", stepId),
            })
        }
        
        if !isTerminal && !outgoing && !isEntryPoint {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("steps.%s", stepId),
                Message: fmt.Sprintf("Step '%s' has no outgoing transitions and is not a terminal step", stepId),
            })
        }
    }
    
    return errors
}
```

#### Rule 6: Business Context Structure Valid

```go
func (v *ContractValidator) validateBusinessContext(contract *Contract) []ValidationError {
    errors := []ValidationError{}
    
    for _, step := range contract.Steps {
        if step.BusinessContext == nil {
            continue
        }
        
        bc := step.BusinessContext
        
        // Must have at least required or optional
        if len(bc.Required) == 0 && len(bc.Optional) == 0 {
            errors = append(errors, ValidationError{
                Field:   fmt.Sprintf("steps.%s.business_context", step.ID),
                Message: "business_context must have at least 'required' or 'optional' fields",
            })
        }
        
        // Check for duplicates between required and optional
        requiredSet := make(map[string]bool)
        for _, field := range bc.Required {
            requiredSet[field] = true
        }
        
        for _, field := range bc.Optional {
            if requiredSet[field] {
                errors = append(errors, ValidationError{
                    Field:   fmt.Sprintf("steps.%s.business_context", step.ID),
                    Message: fmt.Sprintf("Field '%s' appears in both required and optional", field),
                })
            }
        }
    }
    
    return errors
}
```

#### Rule 7: Validation Rules Valid

```go
func (v *ContractValidator) validateValidationRules(contract *Contract) []ValidationError {
    errors := []ValidationError{}
    
    // Validate multiple_terminals_severity if present
    if contract.Validation.MultipleTerminalsSeverity != "" {
        validSeverities := map[string]bool{
            "minor":   true,
            "warning": true,
            "error":   true,
        }
        
        if !validSeverities[contract.Validation.MultipleTerminalsSeverity] {
            errors = append(errors, ValidationError{
                Field:   "validation.multiple_terminals_severity",
                Message: "Must be one of: minor, warning, error",
            })
        }
    }
    
    return errors
}
```

---

### 3. Update Validate() Method

**File:** `pkg/validator/contract.go`

Update the main `Validate()` method to call all new validation functions:

```go
func (v *ContractValidator) Validate(yamlString string) (*Contract, *ValidationResult) {
    errors := []ValidationError{}
    
    // Parse YAML
    contract, err := v.parseContract(yamlString)
    if err != nil {
        return nil, &ValidationResult{
            IsValid: false,
            Errors: []ValidationError{
                {Field: "yaml", Message: fmt.Sprintf("Failed to parse YAML: %v", err)},
            },
        }
    }
    
    // ... existing validations ...
    
    // Get all step IDs
    stepIds := make(map[string]bool)
    for _, step := range contract.Steps {
        stepIds[step.ID] = true
    }
    
    // NEW VALIDATIONS FOR V3
    errors = append(errors, v.validateEntryPoints(contract, stepIds)...)
    errors = append(errors, v.validateTerminalSteps(contract, stepIds)...)
    errors = append(errors, v.validateTerminalStepsReachable(contract)...)
    errors = append(errors, v.validateTransitionSteps(contract, stepIds)...)
    errors = append(errors, v.validateNoOrphanedSteps(contract, stepIds)...)
    errors = append(errors, v.validateBusinessContext(contract)...)
    errors = append(errors, v.validateValidationRules(contract)...)
    
    return contract, &ValidationResult{
        IsValid: len(errors) == 0,
        Errors:  errors,
    }
}
```

---

### 4. Update Graph Builder

**File:** `internal/service/contract_graph.go`

Update the graph builder to include new fields:

```go
func (b *GraphBuilder) BuildGraph(content []byte) (*models.ContractGraph, error) {
    // Parse contract
    var contract models.ContractYAML
    // ... parsing logic ...
    
    // Build nodes with new fields
    for _, step := range contract.Steps {
        nodes[step.ID] = models.GraphNode{
            ID:              step.ID,
            Owner:           step.Owner,                    // NEW
            Role:            step.Role,
            Type:            "step",
            Required:        true,
            DependsOn:       dependsOn,
            Next:            next,
            Timeout:         step.Timeout,
            BusinessContext: step.BusinessContext,          // Already exists
            ParentGroup:     parentGroup,
        }
    }
    
    return &models.ContractGraph{
        Name:          contract.ContractName,              // NEW
        Version:       contract.Version,                   // NEW
        EntryPoints:   contract.EntryPoints,               // NEW
        Parties:       contract.Parties,                   // NEW
        TerminalSteps: contract.TerminalSteps,             // NEW
        Validation:    contract.Validation,                // NEW
        Versioning:    contract.Versioning,                // NEW
        Graph: models.Graph{
            Nodes:     nodes,
            FinalStep: finalStep,
        },
    }, nil
}
```

---

### 5. Update ContractGraph Model

**File:** `internal/models/contract.go` (or wherever ContractGraph is defined)

```go
type ContractGraph struct {
    Name          string                    // NEW
    Version       int                       // NEW
    EntryPoints   []string                  // NEW
    Parties       []string                  // NEW
    TerminalSteps []string                  // NEW
    Validation    ValidationRules           // NEW
    Versioning    VersioningRules           // NEW
    Graph         Graph
}

type GraphNode struct {
    ID              string
    Owner           string                   // NEW
    Role            string
    Type            string
    Mode            string
    Required        bool
    Steps           []string
    DependsOn       []string
    Next            []string
    Timeout         string
    MaxDuration     string
    BusinessContext *BusinessContext         // UPDATED
    ParentGroup     string
}

type BusinessContext struct {
    Required []string `json:"required,omitempty"`
    Optional []string `json:"optional,omitempty"`
}

type ValidationRules struct {
    MaxDuration              string
    AllowMultipleTerminals   bool
    MultipleTerminalsSeverity string
}

type VersioningRules struct {
    ThreadsLockToVersion bool
}
```

---

### 6. Update ContractYAML Model

**File:** `internal/models/contract.go`

```go
type ContractYAML struct {
    ContractName  string           `yaml:"contract_name" json:"contractName"`
    Version       int              `yaml:"version" json:"version"`
    Description   string           `yaml:"description" json:"description"`
    EntryPoints   []string         `yaml:"entry_points" json:"entryPoints"`           // NEW
    Parties       []string         `yaml:"parties" json:"parties"`
    Steps         []Step           `yaml:"steps" json:"steps"`
    Transitions   []Transition     `yaml:"transitions" json:"transitions"`            // NEW
    TerminalSteps []string         `yaml:"terminal_steps" json:"terminalSteps"`       // NEW
    Groups        []Group          `yaml:"groups,omitempty" json:"groups,omitempty"`
    Validation    ValidationRules  `yaml:"validation" json:"validation"`
    Versioning    VersioningRules  `yaml:"versioning,omitempty" json:"versioning,omitempty"` // NEW
}

type Step struct {
    ID              string            `yaml:"id" json:"id"`
    Owner           string            `yaml:"owner" json:"owner"`
    Role            string            `yaml:"role,omitempty" json:"role,omitempty"`
    Type            string            `yaml:"type,omitempty" json:"type,omitempty"`
    DependsOn       []string          `yaml:"depends_on,omitempty" json:"dependsOn,omitempty"`
    Timeout         string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
    BusinessContext *BusinessContext  `yaml:"business_context,omitempty" json:"businessContext,omitempty"` // UPDATED
}

type Transition struct {
    From     string   `yaml:"from" json:"from"`
    To       []string `yaml:"to" json:"to"`
    CanRetry bool     `yaml:"can_retry,omitempty" json:"canRetry,omitempty"`
}
```

---

## Implementation Checklist

### Phase 1: Data Models (2-3 days)

- [ ] Update `pkg/validator/models.go`
  - [ ] Add `EntryPoints`, `Transitions`, `TerminalSteps`, `Versioning` to `Contract`
  - [ ] Change `BusinessContext` from `map[string]string` to struct with `Required`/`Optional`
  - [ ] Add `Transition` struct
  - [ ] Add `VersioningRules` struct
  - [ ] Update `ValidationRules` with new fields
  
- [ ] Update `internal/models/contract.go`
  - [ ] Add new fields to `ContractYAML`
  - [ ] Add new fields to `ContractGraph`
  - [ ] Update `GraphNode` with `Owner` and updated `BusinessContext`
  - [ ] Add `BusinessContext`, `ValidationRules`, `VersioningRules` structs

### Phase 2: Validation Logic (3-4 days)

- [ ] Add validation functions to `pkg/validator/contract.go`
  - [ ] `validateEntryPoints()`
  - [ ] `validateTerminalSteps()`
  - [ ] `validateTerminalStepsReachable()`
  - [ ] `validateTransitionSteps()`
  - [ ] `validateNoOrphanedSteps()`
  - [ ] `validateBusinessContext()`
  - [ ] `validateValidationRules()`
  
- [ ] Update `Validate()` method to call new validation functions

### Phase 3: Graph Builder (2 days)

- [ ] Update `internal/service/contract_graph.go`
  - [ ] Parse new fields from contract YAML
  - [ ] Add new fields to `ContractGraph` return value
  - [ ] Update node building with `Owner` field

### Phase 4: Testing (2-3 days)

- [ ] Unit tests for each validation rule
- [ ] Integration tests for contract upload
- [ ] Test with valid V3 contracts
- [ ] Test with invalid contracts (each validation rule)
- [ ] Test backward compatibility with existing contracts

### Phase 5: Documentation (1 day)

- [ ] Update API documentation
- [ ] Add migration guide for V2 -> V3
- [ ] Document validation error messages

---

## Testing Strategy

### Unit Tests

```go
func TestValidateEntryPoints_Valid(t *testing.T) {
    contract := &Contract{
        EntryPoints: []string{"order_placed"},
        Steps: []Step{
            {ID: "order_placed", Owner: "merchant"},
        },
    }
    
    stepIds := map[string]bool{"order_placed": true}
    errors := validator.validateEntryPoints(contract, stepIds)
    
    assert.Empty(t, errors)
}

func TestValidateEntryPoints_NotDefined(t *testing.T) {
    contract := &Contract{
        EntryPoints: []string{"nonexistent_step"},
        Steps: []Step{
            {ID: "order_placed", Owner: "merchant"},
        },
    }
    
    stepIds := map[string]bool{"order_placed": true}
    errors := validator.validateEntryPoints(contract, stepIds)
    
    assert.Len(t, errors, 1)
    assert.Contains(t, errors[0].Message, "not defined in steps")
}

func TestValidateNoOrphanedSteps(t *testing.T) {
    contract := &Contract{
        EntryPoints: []string{"order_placed"},
        TerminalSteps: []string{"delivered"},
        Steps: []Step{
            {ID: "order_placed", Owner: "merchant"},
            {ID: "orphaned", Owner: "merchant"},
            {ID: "delivered", Owner: "logistics"},
        },
        Transitions: []Transition{
            {From: "order_placed", To: []string{"delivered"}},
        },
    }
    
    stepIds := map[string]bool{
        "order_placed": true,
        "orphaned": true,
        "delivered": true,
    }
    
    errors := validator.validateNoOrphanedSteps(contract, stepIds)
    
    assert.Len(t, errors, 2) // orphaned has no incoming and no outgoing
}
```

### Integration Test

```go
func TestCreateContract_V3_Valid(t *testing.T) {
    yamlContent := `
contract_name: product_delivery
version: 3
description: Product delivery workflow
entry_points:
  - order_placed
parties:
  - merchant
  - logistics_carrier
steps:
  - id: order_placed
    owner: merchant
    business_context:
      required:
        - order_id
        - customer_id
      optional:
        - notes
  - id: delivered
    owner: logistics_carrier
transitions:
  - from: order_placed
    to:
      - delivered
terminal_steps:
  - delivered
validation:
  max_duration: 72h
  allow_multiple_terminals: false
versioning:
  threads_lock_to_version: true
`
    
    statusCode, response := contractService.CreateContract(ctx, ownerID, userID, yamlContent)
    
    assert.Equal(t, 200, statusCode)
    assert.NotNil(t, response)
}
```

---

## Estimated Timeline

| Phase | Duration | Dependencies |
|-------|----------|--------------|
| Phase 1: Data Models | 2-3 days | None |
| Phase 2: Validation Logic | 3-4 days | Phase 1 |
| Phase 3: Graph Builder | 2 days | Phase 1 |
| Phase 4: Testing | 2-3 days | Phases 1-3 |
| Phase 5: Documentation | 1 day | All phases |
| **Total** | **10-13 days** | |

---

## Backward Compatibility

To maintain backward compatibility with existing contracts:

1. Make new fields optional in YAML parsing
2. Set sensible defaults for missing fields
3. Existing contracts without `entry_points` can default to first step
4. Existing contracts without `terminal_steps` can infer from graph
5. Existing `business_context` (map[string]string) can be migrated to new structure

This ensures existing contracts continue to work while new contracts can use V3 features.
