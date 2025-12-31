# Contract V3 Implementation Guide

This guide covers the implementation approach for Contract V3 validation, from contract upload validation to runtime thread validation.

---

## Part 1: Contract Validation (Upload Time)

Contract validation occurs when a contract is uploaded via `POST /contracts` or `PUT /contracts/{name}`. This is a **synchronous, blocking operation** that must pass before the contract is stored.

### 1.1 Contract Validation Pipeline

```
POST /contracts
    ↓
Parse YAML → Validate Schema → Validate Business Rules → Store Contract
    ↓              ↓                    ↓                      ↓
  Fail          Fail                 Fail                  Success
  400           400                  400                   201
```

### 1.2 Validation Steps

#### Step 1: Schema Validation

Validate the basic structure and data types.

**Checks:**
- Required fields present: `contract_name`, `version`, `parties`, `steps`, `transitions`, `terminal_steps`
- Field types correct (arrays, objects, strings, etc.)
- Duration formats valid (e.g., `30s`, `5m`, `24h`)

**Implementation:**

```go
type Contract struct {
    ContractName    string              `yaml:"contract_name" validate:"required"`
    Version         int                 `yaml:"version" validate:"required,min=1"`
    Description     string              `yaml:"description"`
    EntryPoints     []string            `yaml:"entry_points" validate:"required,min=1"`
    Parties         []string            `yaml:"parties" validate:"required,min=1"`
    Steps           []Step              `yaml:"steps" validate:"required,min=1,dive"`
    Transitions     []Transition        `yaml:"transitions" validate:"required,min=1,dive"`
    TerminalSteps   []string            `yaml:"terminal_steps" validate:"required,min=1"`
    Validation      ValidationRules     `yaml:"validation"`
    Versioning      VersioningRules     `yaml:"versioning"`
}

type Step struct {
    ID              string              `yaml:"id" validate:"required"`
    Owner           string              `yaml:"owner" validate:"required"`
    Timeout         *Duration           `yaml:"timeout"`
    BusinessContext *BusinessContext    `yaml:"business_context"`
}

type BusinessContext struct {
    Required []string `yaml:"required"`
    Optional []string `yaml:"optional"`
}

type Transition struct {
    From          string   `yaml:"from" validate:"required"`
    To            []string `yaml:"to" validate:"required,min=1"`
    CanRetry      bool     `yaml:"can_retry"`
    MaxRetries    int      `yaml:"max_retries"`
    RetryLeadsTo  string   `yaml:"retry_leads_to"`
}
```

**Error Response:**

```json
{
  "valid": false,
  "errors": [
    {
      "type": "schema_validation_error",
      "field": "steps[0].owner",
      "message": "Field 'owner' is required"
    }
  ]
}
```

---

#### Step 2: Business Rule Validation

Validate the contract's internal consistency and business logic.

**Validation Rules:**

1. **Entry Points Must Exist in Steps**
2. **At Least One Entry Point Required**
3. **Terminal Steps Must Exist in Steps**
4. **Terminal Steps Must Be Reachable**
5. **All Transition Steps Must Exist**
6. **All Owners Must Be Defined Parties**
7. **Retry Leads To Must Reference Valid Steps**
8. **No Orphaned Steps**
9. **Business Context Structure Valid**

**Implementation Approach:**

```go
func ValidateContract(contract *Contract) []ValidationError {
    var errors []ValidationError
    
    // Build lookup maps for efficient validation
    stepMap := buildStepMap(contract.Steps)
    partyMap := buildPartyMap(contract.Parties)
    transitionGraph := buildTransitionGraph(contract.Transitions)
    
    // Rule 1 & 2: Entry Points
    errors = append(errors, validateEntryPoints(contract.EntryPoints, stepMap)...)
    
    // Rule 3 & 4: Terminal Steps
    errors = append(errors, validateTerminalSteps(contract.TerminalSteps, stepMap, transitionGraph)...)
    
    // Rule 5: Transition Steps
    errors = append(errors, validateTransitionSteps(contract.Transitions, stepMap)...)
    
    // Rule 6: Owners
    errors = append(errors, validateOwners(contract.Steps, partyMap)...)
    
    // Rule 7: Retry Leads To
    errors = append(errors, validateRetryLeadsTo(contract.Transitions, stepMap)...)
    
    // Rule 8: Orphaned Steps
    errors = append(errors, validateNoOrphanedSteps(contract, stepMap, transitionGraph)...)
    
    // Rule 9: Business Context
    errors = append(errors, validateBusinessContext(contract.Steps)...)
    
    return errors
}
```

**Example Validation Functions:**

```go
// Rule 1 & 2: Entry Points Must Exist and At Least One Required
func validateEntryPoints(entryPoints []string, stepMap map[string]*Step) []ValidationError {
    var errors []ValidationError
    
    if len(entryPoints) == 0 {
        errors = append(errors, ValidationError{
            Type:    "missing_entry_points",
            Message: "At least one entry point is required",
        })
        return errors
    }
    
    for _, entryPoint := range entryPoints {
        if _, exists := stepMap[entryPoint]; !exists {
            errors = append(errors, ValidationError{
                Type:    "entry_point_not_defined",
                Message: fmt.Sprintf("Entry point '%s' is not defined in steps", entryPoint),
                Field:   "entry_points",
                Value:   entryPoint,
            })
        }
    }
    
    return errors
}

// Rule 4: Terminal Steps Must Be Reachable
func validateTerminalSteps(terminalSteps []string, stepMap map[string]*Step, graph TransitionGraph) []ValidationError {
    var errors []ValidationError
    
    for _, terminal := range terminalSteps {
        // Check if step exists
        if _, exists := stepMap[terminal]; !exists {
            errors = append(errors, ValidationError{
                Type:    "terminal_step_not_defined",
                Message: fmt.Sprintf("Terminal step '%s' is not defined in steps", terminal),
                Field:   "terminal_steps",
                Value:   terminal,
            })
            continue
        }
        
        // Check if step is reachable (appears in at least one transition's 'to')
        if !graph.IsReachable(terminal) {
            errors = append(errors, ValidationError{
                Type:    "terminal_step_not_reachable",
                Message: fmt.Sprintf("Terminal step '%s' is not reachable from any transition", terminal),
                Field:   "terminal_steps",
                Value:   terminal,
            })
        }
    }
    
    return errors
}

// Rule 8: No Orphaned Steps
func validateNoOrphanedSteps(contract *Contract, stepMap map[string]*Step, graph TransitionGraph) []ValidationError {
    var errors []ValidationError
    
    entryPointSet := toSet(contract.EntryPoints)
    terminalStepSet := toSet(contract.TerminalSteps)
    
    for stepID := range stepMap {
        isEntryPoint := entryPointSet[stepID]
        isTerminal := terminalStepSet[stepID]
        hasIncoming := graph.HasIncomingTransitions(stepID)
        hasOutgoing := graph.HasOutgoingTransitions(stepID)
        
        // Entry points don't need incoming, terminals don't need outgoing
        // But all other steps need at least one of each
        if !isEntryPoint && !hasIncoming && !isTerminal {
            errors = append(errors, ValidationError{
                Type:    "orphaned_step",
                Message: fmt.Sprintf("Step '%s' has no incoming transitions and is not an entry point", stepID),
                Field:   "steps",
                Value:   stepID,
            })
        }
        
        if !isTerminal && !hasOutgoing && !isEntryPoint {
            errors = append(errors, ValidationError{
                Type:    "orphaned_step",
                Message: fmt.Sprintf("Step '%s' has no outgoing transitions and is not a terminal step", stepID),
                Field:   "steps",
                Value:   stepID,
            })
        }
    }
    
    return errors
}

// Rule 9: Business Context Structure Valid
func validateBusinessContext(steps []Step) []ValidationError {
    var errors []ValidationError
    
    for _, step := range steps {
        if step.BusinessContext == nil {
            continue
        }
        
        bc := step.BusinessContext
        
        // Must have at least required or optional
        if len(bc.Required) == 0 && len(bc.Optional) == 0 {
            errors = append(errors, ValidationError{
                Type:    "invalid_business_context",
                Message: fmt.Sprintf("Step '%s' has empty business_context (must have required or optional fields)", step.ID),
                Field:   fmt.Sprintf("steps[%s].business_context", step.ID),
            })
        }
        
        // Check for duplicates between required and optional
        requiredSet := toSet(bc.Required)
        for _, optField := range bc.Optional {
            if requiredSet[optField] {
                errors = append(errors, ValidationError{
                    Type:    "duplicate_business_context_field",
                    Message: fmt.Sprintf("Step '%s' has field '%s' in both required and optional", step.ID, optField),
                    Field:   fmt.Sprintf("steps[%s].business_context", step.ID),
                    Value:   optField,
                })
            }
        }
    }
    
    return errors
}
```

---

#### Step 3: Store Contract

If validation passes, store the contract in the database.

**Storage Schema:**

```sql
CREATE TABLE contracts (
    id UUID PRIMARY KEY,
    contract_name VARCHAR(255) NOT NULL,
    version INT NOT NULL,
    description TEXT,
    contract_data JSONB NOT NULL,  -- Full contract as JSON
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_by VARCHAR(255),
    is_active BOOLEAN DEFAULT TRUE,
    UNIQUE(contract_name, version)
);

CREATE INDEX idx_contracts_name_version ON contracts(contract_name, version);
CREATE INDEX idx_contracts_active ON contracts(contract_name, is_active) WHERE is_active = TRUE;
```

**Implementation:**

```go
func StoreContract(ctx context.Context, contract *Contract, createdBy string) error {
    contractJSON, err := json.Marshal(contract)
    if err != nil {
        return fmt.Errorf("failed to marshal contract: %w", err)
    }
    
    query := `
        INSERT INTO contracts (id, contract_name, version, description, contract_data, created_by)
        VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (contract_name, version) 
        DO UPDATE SET 
            description = EXCLUDED.description,
            contract_data = EXCLUDED.contract_data,
            created_by = EXCLUDED.created_by,
            created_at = NOW()
    `
    
    _, err = db.ExecContext(ctx, query,
        uuid.New(),
        contract.ContractName,
        contract.Version,
        contract.Description,
        contractJSON,
        createdBy,
    )
    
    return err
}
```

---

### 1.3 Contract Validation API Flow

```
Client                    API Server                  Validator                Database
  |                           |                           |                        |
  |-- POST /contracts ------->|                           |                        |
  |                           |-- Parse YAML ------------>|                        |
  |                           |                           |                        |
  |                           |<-- Parsed Contract -------|                        |
  |                           |                           |                        |
  |                           |-- Validate() ------------>|                        |
  |                           |                           |-- Check Rules          |
  |                           |                           |-- Build Graphs         |
  |                           |                           |-- Validate Logic       |
  |                           |<-- Validation Result -----|                        |
  |                           |                           |                        |
  |                           |-- Store Contract ---------------------------------->|
  |                           |                           |                        |
  |                           |<-- Stored ------------------------------------------|
  |<-- 201 Created ----------|                           |                        |
  |    {contract_id, version} |                           |                        |
```

**Success Response:**

```json
{
  "contract_id": "550e8400-e29b-41d4-a716-446655440000",
  "contract_name": "product_delivery",
  "version": 3,
  "status": "active",
  "created_at": "2025-12-27T17:00:00Z"
}
```

**Failure Response:**

```json
{
  "valid": false,
  "errors": [
    {
      "type": "entry_point_not_defined",
      "message": "Entry point 'initial_order' is not defined in steps",
      "field": "entry_points",
      "value": "initial_order"
    },
    {
      "type": "orphaned_step",
      "message": "Step 'abandoned_step' has no incoming transitions and is not an entry point",
      "field": "steps",
      "value": "abandoned_step"
    }
  ]
}
```

---

## Part 2: Thread Validation (Runtime)

Thread validation occurs in two phases:
1. **Blocking Validation** (HTTP 400) - Prevents step from being written
2. **Non-Blocking Validation** (HTTP 201 + WebSocket) - Step written, violations emitted

### 2.1 Thread Creation Validation

When creating a thread, validate the initial step.

**API Endpoint:** `POST /threads/new`

**Validation Flow:**

```
POST /threads/new
    ↓
Load Contract → Validate Entry Point → Validate Owner → Validate Required Fields → Create Thread
    ↓                  ↓                     ↓                    ↓                      ↓
  404              400 (blocking)        400 (blocking)      400 (blocking)         201 Created
```

**Implementation:**

```go
func CreateThread(ctx context.Context, req CreateThreadRequest) (*Thread, error) {
    // Load contract
    contract, err := LoadContract(ctx, req.ContractID, req.Version)
    if err != nil {
        return nil, &BlockingError{
            Type:    "contract_not_found",
            Message: fmt.Sprintf("Contract '%s' version %d not found", req.ContractID, req.Version),
        }
    }
    
    // Blocking Validation 1: Entry Point
    if !isValidEntryPoint(req.InitialStep.StepID, contract.EntryPoints) {
        return nil, &BlockingError{
            Type:    "invalid_entry_point",
            Message: fmt.Sprintf("Step '%s' is not a valid entry point", req.InitialStep.StepID),
            ValidEntryPoints: contract.EntryPoints,
        }
    }
    
    // Blocking Validation 2: Owner
    step := contract.GetStep(req.InitialStep.StepID)
    if step.Owner != req.InitialStep.Owner {
        return nil, &BlockingError{
            Type:     "unauthorized_owner",
            Message:  fmt.Sprintf("Step '%s' must be owned by '%s'", step.ID, step.Owner),
            Expected: step.Owner,
            Provided: req.InitialStep.Owner,
        }
    }
    
    // Blocking Validation 3: Required Fields
    if err := validateRequiredFields(req.InitialStep.Data, step.BusinessContext); err != nil {
        return nil, err // Returns BlockingError
    }
    
    // Create thread
    thread := &Thread{
        ID:              uuid.New().String(),
        ContractName:    contract.ContractName,
        ContractVersion: contract.Version,
        CurrentStep:     req.InitialStep.StepID,
        Status:          "active",
        CreatedAt:       time.Now(),
    }
    
    // Store thread
    if err := storeThread(ctx, thread); err != nil {
        return nil, err
    }
    
    // Store initial step
    if err := storeStep(ctx, thread.ID, req.InitialStep); err != nil {
        return nil, err
    }
    
    return thread, nil
}
```

---

### 2.2 Step Submission Validation

When submitting a step to an existing thread.

**API Endpoint:** `POST /threads/{thread_id}/steps`

**Two-Phase Validation:**

```
POST /threads/{thread_id}/steps
    ↓
Load Thread & Contract
    ↓
┌─────────────────────────────────────┐
│  PHASE 1: BLOCKING VALIDATION       │
│  (HTTP 400 - Step NOT Written)      │
├─────────────────────────────────────┤
│  1. Step defined in contract?       │
│  2. Owner authorized?                │
│  3. Required fields present?         │
│  4. Thread already completed?        │
└─────────────────────────────────────┘
    ↓ PASS
Write Step to Database
    ↓
┌─────────────────────────────────────┐
│  PHASE 2: NON-BLOCKING VALIDATION   │
│  (HTTP 201 + WebSocket Events)      │
├─────────────────────────────────────┤
│  1. Valid transition?                │
│  2. Step timeout exceeded?           │
│  3. Max duration exceeded?           │
│  4. Multiple terminals?              │
│  5. Retry limit exceeded?            │
└─────────────────────────────────────┘
    ↓
Emit Violations via WebSocket
    ↓
Return 201 Created
```

**Implementation:**

```go
func SubmitStep(ctx context.Context, threadID string, req SubmitStepRequest) (*StepResponse, error) {
    // Load thread and contract
    thread, err := LoadThread(ctx, threadID)
    if err != nil {
        return nil, err
    }
    
    contract, err := LoadContract(ctx, thread.ContractName, thread.ContractVersion)
    if err != nil {
        return nil, err
    }
    
    // ========================================
    // PHASE 1: BLOCKING VALIDATION (HTTP 400)
    // ========================================
    
    blockingErrors := []BlockingError{}
    
    // 1. Step defined in contract?
    step := contract.GetStep(req.StepID)
    if step == nil {
        blockingErrors = append(blockingErrors, BlockingError{
            Type:    "step_not_defined",
            Message: fmt.Sprintf("Step '%s' is not defined in contract", req.StepID),
        })
    }
    
    // 2. Owner authorized?
    if step != nil && step.Owner != req.Owner {
        blockingErrors = append(blockingErrors, BlockingError{
            Type:     "unauthorized_owner",
            Message:  fmt.Sprintf("Step '%s' must be owned by '%s'", step.ID, step.Owner),
            Expected: step.Owner,
            Provided: req.Owner,
        })
    }
    
    // 3. Required fields present?
    if step != nil && step.BusinessContext != nil {
        missingFields := findMissingRequiredFields(req.Data, step.BusinessContext.Required)
        if len(missingFields) > 0 {
            blockingErrors = append(blockingErrors, BlockingError{
                Type:          "missing_required_fields",
                Message:       "Required fields missing",
                MissingFields: missingFields,
            })
        }
    }
    
    // 4. Thread already completed?
    if thread.Status == "completed" || thread.Status == "failed" {
        blockingErrors = append(blockingErrors, BlockingError{
            Type:    "thread_already_completed",
            Message: fmt.Sprintf("Thread is already %s", thread.Status),
        })
    }
    
    // Return blocking errors if any
    if len(blockingErrors) > 0 {
        return nil, &ValidationFailedError{
            BlockingErrors: blockingErrors,
        }
    }
    
    // ========================================
    // WRITE STEP TO DATABASE
    // ========================================
    
    stepRecord := &StepRecord{
        ThreadID:  threadID,
        StepID:    req.StepID,
        Owner:     req.Owner,
        Data:      req.Data,
        Sequence:  getNextSequence(ctx, threadID),
        Timestamp: time.Now(),
    }
    
    if err := storeStep(ctx, stepRecord); err != nil {
        return nil, err
    }
    
    // Update thread current step
    if err := updateThreadCurrentStep(ctx, threadID, req.StepID); err != nil {
        return nil, err
    }
    
    // ========================================
    // PHASE 2: NON-BLOCKING VALIDATION
    // ========================================
    
    go func() {
        violations := performNonBlockingValidation(ctx, thread, contract, stepRecord)
        
        for _, violation := range violations {
            emitViolationEvent(ctx, threadID, violation)
            storeViolation(ctx, threadID, violation)
        }
    }()
    
    // Return success immediately
    return &StepResponse{
        ThreadID:  threadID,
        StepID:    req.StepID,
        Sequence:  stepRecord.Sequence,
        Timestamp: stepRecord.Timestamp,
    }, nil
}
```

---

### 2.3 Non-Blocking Validation Implementation

```go
func performNonBlockingValidation(ctx context.Context, thread *Thread, contract *Contract, step *StepRecord) []Violation {
    violations := []Violation{}
    
    // Load thread history
    history, err := loadThreadHistory(ctx, thread.ID)
    if err != nil {
        log.Error("Failed to load thread history", "error", err)
        return violations
    }
    
    // 1. Valid Transition?
    if !isValidTransition(history, step.StepID, contract) {
        violations = append(violations, Violation{
            Type:     "invalid_transition",
            Severity: "major",
            Message:  fmt.Sprintf("Invalid transition from '%s' to '%s'", thread.CurrentStep, step.StepID),
            Details: map[string]interface{}{
                "from_step":      history.GetPreviousStep(),
                "to_step":        step.StepID,
                "expected_steps": contract.GetAllowedTransitions(history.GetPreviousStep()),
            },
        })
    }
    
    // 2. Step Timeout Exceeded?
    if timeout := checkStepTimeout(history, step, contract); timeout != nil {
        violations = append(violations, *timeout)
    }
    
    // 3. Max Duration Exceeded?
    if maxDuration := checkMaxDuration(thread, contract); maxDuration != nil {
        violations = append(violations, *maxDuration)
    }
    
    // 4. Multiple Terminals?
    if multipleTerminals := checkMultipleTerminals(history, step, contract); multipleTerminals != nil {
        violations = append(violations, *multipleTerminals)
    }
    
    // 5. Retry Limit Exceeded?
    if retryLimit := checkRetryLimit(history, step, contract); retryLimit != nil {
        violations = append(violations, *retryLimit)
    }
    
    // 6. Missing Optional Fields? (Info only)
    if missingOptional := checkMissingOptionalFields(step, contract); missingOptional != nil {
        violations = append(violations, *missingOptional)
    }
    
    // 7. Extra Undocumented Fields? (Info only)
    if extraFields := checkExtraFields(step, contract); extraFields != nil {
        violations = append(violations, *extraFields)
    }
    
    return violations
}

func isValidTransition(history *ThreadHistory, toStep string, contract *Contract) bool {
    if len(history.Steps) == 0 {
        // First step - must be entry point (already validated in blocking)
        return true
    }
    
    previousStep := history.GetPreviousStep()
    allowedTransitions := contract.GetAllowedTransitions(previousStep)
    
    for _, allowed := range allowedTransitions {
        if allowed == toStep {
            return true
        }
    }
    
    return false
}

func checkStepTimeout(history *ThreadHistory, currentStep *StepRecord, contract *Contract) *Violation {
    if len(history.Steps) == 0 {
        return nil
    }
    
    previousStep := history.GetPreviousStep()
    stepDef := contract.GetStep(previousStep)
    
    if stepDef.Timeout == nil {
        return nil
    }
    
    previousStepRecord := history.GetStepRecord(previousStep)
    elapsed := currentStep.Timestamp.Sub(previousStepRecord.Timestamp)
    
    if elapsed > stepDef.Timeout.Duration {
        return &Violation{
            Type:     "step_timeout_exceeded",
            Severity: "major",
            Message:  fmt.Sprintf("Step '%s' exceeded timeout of %s", previousStep, stepDef.Timeout),
            Details: map[string]interface{}{
                "step_id": previousStep,
                "timeout": stepDef.Timeout.String(),
                "elapsed": elapsed.String(),
            },
        }
    }
    
    return nil
}

func checkMaxDuration(thread *Thread, contract *Contract) *Violation {
    if contract.Validation.MaxDuration == nil {
        return nil
    }
    
    elapsed := time.Since(thread.CreatedAt)
    
    if elapsed > contract.Validation.MaxDuration.Duration {
        return &Violation{
            Type:     "max_duration_exceeded",
            Severity: "major",
            Message:  fmt.Sprintf("Thread execution exceeded maximum duration of %s", contract.Validation.MaxDuration),
            Details: map[string]interface{}{
                "max_duration": contract.Validation.MaxDuration.String(),
                "elapsed":      elapsed.String(),
                "started_at":   thread.CreatedAt,
            },
        }
    }
    
    return nil
}
```

---

### 2.4 WebSocket Event Emission

```go
func emitViolationEvent(ctx context.Context, threadID string, violation Violation) {
    event := ViolationEvent{
        Event:         "thread_violation",
        ThreadID:      threadID,
        ViolationType: violation.Type,
        Severity:      violation.Severity,
        Message:       violation.Message,
        Details:       violation.Details,
        Timestamp:     time.Now(),
    }
    
    // Publish to WebSocket channel
    websocketHub.PublishToThread(threadID, event)
    
    // Also publish to monitoring systems
    metricsCollector.RecordViolation(violation.Type, violation.Severity)
}
```

---

## Summary

### Contract Validation (Upload)
- **When:** Contract upload via API
- **Type:** Synchronous, blocking
- **Validates:** Schema + Business Rules
- **Response:** HTTP 400 (fail) or HTTP 201 (success)
- **Storage:** Contract stored in database if valid

### Thread Validation (Runtime)
- **When:** Thread creation and step submission
- **Type:** Two-phase (blocking + non-blocking)

**Phase 1 - Blocking (HTTP 400):**
- Step defined?
- Owner authorized?
- Required fields present?
- Thread completed?

**Phase 2 - Non-Blocking (HTTP 201 + WebSocket):**
- Valid transition?
- Timeout exceeded?
- Max duration exceeded?
- Multiple terminals?
- Retry limit exceeded?

This approach ensures data integrity at write time while preserving complete audit trails for workflow analysis.
