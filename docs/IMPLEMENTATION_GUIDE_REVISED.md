# Contract V3 Implementation Guide (Revised)

This guide reflects the **actual Threadify architecture** where steps are submitted via **WebSocket**, not REST API.

---

## Architecture Overview

```
SDK (JavaScript)                WebSocket Server (Go)              Valkey/Redis
     |                                  |                               |
     |-- WebSocket Connect ------------>|                               |
     |<-- Connected -------------------|                               |
     |                                  |                               |
     |-- startThread ------------------>|                               |
     |                                  |-- Create Thread ------------->|
     |<-- threadId ---------------------|                               |
     |                                  |                               |
     |-- recordThreadEvent ------------>|                               |
     |   (step data)                    |-- Validate (blocking) --------|
     |                                  |-- ProcessStepEvent() -------->|
     |<-- success --------------------- |   (async, non-blocking)       |
     |                                  |                               |
     |                                  |-- Hash & Store -------------->|
     |                                  |-- Emit WebSocket Events ----->|
```

### Key Differences from Proposed Strategy

1. **No REST API for steps** - All step submission happens via WebSocket `recordThreadEvent` action
2. **SDK-driven** - Steps are created using fluent API: `thread.step("name").addContext({...}).stop()`
3. **Cryptographic chain** - Each step is hashed with previous hash (blockchain-like)
4. **Batched writes** - Steps are batched and written to Valkey in bulk
5. **Idempotency via hash** - Steps use `stepName:idempotencyKey` for deduplication

---

## Part 1: Contract Validation (Upload Time)

**This part remains the same as proposed** - Contract validation happens when uploading via REST API.

### Contract Upload Flow

```
POST /contracts
    ↓
Parse YAML → Schema Validation → Business Rules → Store in DB
    ↓              ↓                    ↓              ↓
  Fail          Fail                 Fail          Success
  400           400                  400           201
```

**Validation Rules:**
1. Entry points must exist in steps
2. At least one entry point required
3. Terminal steps must exist and be reachable
4. All transition steps must exist
5. All owners must be defined parties
6. Retry leads to must reference valid steps
7. No orphaned steps
8. Business context structure valid

---

## Part 2: Thread Validation (Runtime via WebSocket)

### 2.1 WebSocket Connection Flow

```javascript
// SDK: Connect to server
const connection = await Threadify.connect(apiKey, serviceName);

// WebSocket message
{
  "action": "connect",
  "apiKey": "key_123",
  "serviceName": "order-service"
}

// Server response
{
  "action": "connect",
  "status": "success",
  "ownerId": "user_123",
  "companyId": "company_456"
}
```

---

### 2.2 Thread Creation with Entry Point Validation

```javascript
// SDK: Start thread with contract
const thread = await connection.start("product_delivery:3", "merchant-service");

// WebSocket message
{
  "action": "startThread",
  "contractName": "product_delivery:3",
  "role": "participant",
  "refs": {
    "serviceName": "merchant-service"
  }
}
```

**Server-side validation (in `HandleStartThread`):**

```go
func (s *ThreadService) HandleStartThread(req *StartThreadRequest, ownerID, companyID string) *StartThreadResponse {
    // 1. Load contract
    contractName, version := parseContractIdentifier(req.ContractName)
    contract, err := s.contractValidator.GetContractGraph(contractName, version)
    if err != nil {
        return &StartThreadResponse{
            Status:  "error",
            Message: fmt.Sprintf("Contract not found: %s", req.ContractName),
        }
    }
    
    // 2. Validate entry point (if initial step provided)
    if req.InitialStepName != "" {
        if !isValidEntryPoint(req.InitialStepName, contract.EntryPoints) {
            return &StartThreadResponse{
                Status:  "error",
                Message: fmt.Sprintf("Step '%s' is not a valid entry point", req.InitialStepName),
            }
        }
    }
    
    // 3. Create thread
    thread := models.NewThreadWithCompany(
        uuid.New().String(),
        contractName,
        version,
        ownerID,
        companyID,
    )
    thread.ContractName = contractName
    
    // 4. Store thread
    if err := s.repo.Save(ctx, thread); err != nil {
        return &StartThreadResponse{
            Status:  "error",
            Message: "Failed to create thread",
        }
    }
    
    return &StartThreadResponse{
        Status:   "success",
        ThreadID: thread.ID,
        Message:  "Thread started successfully",
    }
}
```

---

### 2.3 Step Submission via WebSocket

**SDK Usage:**

```javascript
// Create and submit step
await thread
  .step("order_placed")
  .addContext({
    order_id: "ORD-123",
    customer_id: "CUST-456",
    total_amount: "99.99"
  })
  .stop("success");

// WebSocket message sent
{
  "action": "recordThreadEvent",
  "threadId": "thread_abc123",
  "stepName": "order_placed",
  "startedAt": "2025-12-27T17:00:00Z",
  "finishedAt": "2025-12-27T17:00:01Z",
  "status": "success",
  "context": {
    "order_id": "ORD-123",
    "customer_id": "CUST-456",
    "total_amount": "99.99"
  },
  "idempotencyKey": "a3f2b1c4",
  "serviceName": "merchant-service"
}
```

---

### 2.4 Two-Phase Validation in `HandleRecordEvent`

**Current Implementation:**

```go
func (s *ThreadService) HandleRecordEvent(req *RecordEventRequest, ownerID, companyID string) *RecordEventResponse {
    // ========================================
    // PHASE 1: BLOCKING VALIDATION
    // ========================================
    
    // 1. Validate required fields
    if req.ThreadID == "" || req.StepName == "" || req.Status == "" {
        return &RecordEventResponse{
            Status:  "error",
            Message: "Missing required fields",
        }
    }
    
    // 2. Load thread
    thread, err := s.getThread(req.ThreadID)
    if err != nil {
        return &RecordEventResponse{
            Status:  "error",
            Message: "Thread not found",
        }
    }
    
    // 3. Check access permission
    hasAccess, err := s.accessService.CheckThreadAccess(req.ThreadID, ownerID, "write", thread)
    if err != nil || !hasAccess {
        return &RecordEventResponse{
            Status:  "error",
            Message: "Access denied",
        }
    }
    
    // 4. Check for duplicate (idempotency)
    if req.IdempotencyKey != "" {
        storageKey := req.StepName + ":" + req.IdempotencyKey
        if existingStep, exists := thread.Steps[storageKey]; exists {
            if existingStep.Status == "success" {
                return &RecordEventResponse{
                    Status:      "error",
                    Message:     "Step with this signature already successful",
                    IsDuplicate: true,
                }
            }
        }
    }
    
    // 5. Validate against contract (if thread has one)
    if thread.ContractName != "" {
        graph, err := s.contractValidator.GetContractGraph(thread.ContractName, 1)
        if err != nil {
            return &RecordEventResponse{
                Status:  "error",
                Message: "Failed to load contract",
            }
        }
        
        // Check if step exists in contract
        stepNode, exists := graph.Graph.Nodes[req.StepName]
        if !exists {
            return &RecordEventResponse{
                Status:  "error",
                Message: fmt.Sprintf("Step '%s' not found in contract", req.StepName),
            }
        }
        
        // Validate role
        if stepNode.Role != "" {
            hasRole, err := s.accessService.ValidateUserRoleForStep(req.ThreadID, ownerID, stepNode.Role)
            if err != nil || !hasRole {
                return &RecordEventResponse{
                    Status:  "error",
                    Message: "Access denied: Invalid role for step",
                }
            }
        }
        
        // Validate step context against contract
        if err := s.contractValidator.ValidateStepInContract(thread.ContractName, 1, req.StepName, req.Context); err != nil {
            return &RecordEventResponse{
                Status:  "error",
                Message: fmt.Sprintf("Step validation failed: %v", err),
            }
        }
    }
    
    // ========================================
    // WRITE STEP (Non-blocking from here)
    // ========================================
    
    stepID := uuid.New().String()
    
    // Create step event
    stepEvent := &models.StepEvent{
        StepID:      stepID,
        ThreadID:    req.ThreadID,
        StepName:    req.StepName,
        ServiceName: req.ServiceName,
        Type:        req.Type,
        Status:      req.Status,
        Context:     req.Context,
        StartedAt:   req.StartedAt,
        FinishedAt:  req.FinishedAt,
        Timestamp:   time.Now(),
    }
    
    // Process step event (async hashing and storage)
    if err := s.stepEventService.ProcessStepEvent(*stepEvent); err != nil {
        return &RecordEventResponse{
            Status:  "error",
            Message: "Failed to process step event",
        }
    }
    
    // Update thread state for idempotency tracking
    if req.IdempotencyKey != "" {
        storageKey := req.StepName + ":" + req.IdempotencyKey
        thread.Steps[storageKey] = &models.StepState{
            StepID:         stepID,
            StepName:       req.StepName,
            Status:         req.Status,
            IdempotencyKey: req.IdempotencyKey,
            Context:        req.Context,
            CreatedAt:      time.Now(),
            UpdatedAt:      time.Now(),
            RetryCount:     0,
        }
        s.repo.Save(context.Background(), thread)
    }
    
    return &RecordEventResponse{
        Status:   "success",
        Message:  "Step Event recorded successfully",
        ThreadID: req.ThreadID,
        StepID:   stepID,
    }
}
```

---

### 2.5 Async Step Processing (Cryptographic Chain)

**After blocking validation passes, step is processed asynchronously:**

```go
func (ses *StepEventService) ProcessStepEvent(event models.StepEvent) error {
    // 1. Queue to thread-specific channel (ensures sequential processing per thread)
    threadQueue := ses.getOrCreateThreadQueue(event.ThreadID)
    threadQueue <- event
    
    return nil
}

func (ses *StepEventService) processStepEvent(event models.StepEvent) error {
    // 1. Get last hash for this thread
    previousHash, err := ses.getLastStepHash(event.ThreadID)
    if err != nil {
        return err
    }
    
    // 2. Calculate new hash (SHA256 of: prevHash + stepID + stepName + timestamps + context)
    newHash := ses.calculateStepHash(previousHash, event)
    
    // 3. Create hashed event
    hashedEvent := event.ToHashedStepEvent(newHash, previousHash)
    
    // 4. Add to batch for bulk write
    ses.addToBatch(*hashedEvent)
    
    // 5. Update memory cache immediately
    ses.lastHashes[event.ThreadID] = newHash
    
    return nil
}

func (ses *StepEventService) calculateStepHash(previousHash string, event models.StepEvent) string {
    hasher := sha256.New()
    hasher.Write([]byte(previousHash))
    hasher.Write([]byte(event.StepID))
    hasher.Write([]byte(event.StepName))
    hasher.Write([]byte(event.StartedAt))
    hasher.Write([]byte(event.FinishedAt))
    hasher.Write([]byte(event.Status))
    hasher.Write([]byte(event.ContextJSON()))
    return hex.EncodeToString(hasher.Sum(nil))
}
```

---

### 2.6 Batched Write to Valkey

```go
func (ses *StepEventService) writeBatch() {
    ses.batchMu.Lock()
    eventsToWrite := make([]models.HashedStepEvent, len(ses.batch))
    copy(eventsToWrite, ses.batch)
    ses.batch = ses.batch[:0] // Clear batch
    ses.batchMu.Unlock()
    
    // Bulk write to Valkey
    for _, event := range eventsToWrite {
        // 1. Add to activity list (for quick access)
        activityKey := fmt.Sprintf("thread:%s:activity", event.ThreadID)
        ses.valkeyRepo.LPush(ctx, activityKey, eventJSON)
        ses.valkeyRepo.Expire(ctx, activityKey, 7*24*time.Hour)
        
        // 2. Add to stream (for archival)
        ses.valkeyRepo.XAdd(ctx, "streams:step_events", streamValues)
        
        // 3. Update thread last hash
        ses.updateThreadLastHash(event.ThreadID, event.Hash)
    }
}
```

---

## What Needs to Be Added for Contract V3

### 1. Contract Validation on Step Submission

**Current:** Basic validation (step exists, role check, context validation)

**Needed for V3:**

```go
// In HandleRecordEvent, after loading contract:

// A. Validate step owner matches party
stepDef := contract.GetStep(req.StepName)
if stepDef == nil {
    return &RecordEventResponse{
        Status:  "error",
        Message: fmt.Sprintf("Step '%s' not defined in contract", req.StepName),
    }
}

// B. Validate owner/party authorization
expectedOwner := stepDef.Owner
userRole, _ := s.accessService.GetUserRole(req.ThreadID, ownerID)
if userRole != expectedOwner {
    return &RecordEventResponse{
        Status:  "error",
        Message: fmt.Sprintf("Step '%s' must be owned by '%s', you are '%s'", req.StepName, expectedOwner, userRole),
    }
}

// C. Validate required fields in business_context
if stepDef.BusinessContext != nil {
    missingRequired := []string{}
    for _, requiredField := range stepDef.BusinessContext.Required {
        if _, exists := req.Context[requiredField]; !exists {
            missingRequired = append(missingRequired, requiredField)
        }
    }
    
    if len(missingRequired) > 0 {
        return &RecordEventResponse{
            Status:  "error",
            Message: "Missing required fields",
            BlockingErrors: []BlockingError{
                {
                    Type:          "missing_required_fields",
                    Message:       "Required business_context fields missing",
                    MissingFields: missingRequired,
                },
            },
        }
    }
}

// D. Check if thread is already completed
if thread.Status == "completed" || thread.Status == "failed" {
    return &RecordEventResponse{
        Status:  "error",
        Message: fmt.Sprintf("Thread is already %s", thread.Status),
    }
}
```

---

### 2. Non-Blocking Validation (Post-Write)

**Add after successful step write:**

```go
// After ProcessStepEvent succeeds, trigger async validation
go func() {
    violations := s.validateStepTransition(thread, contract, req.StepName)
    
    for _, violation := range violations {
        // Emit WebSocket event to all thread subscribers
        s.websocketHub.EmitToThread(req.ThreadID, violation)
        
        // Store violation in audit trail
        s.storeViolation(req.ThreadID, violation)
    }
}()
```

**Validation checks:**

```go
func (s *ThreadService) validateStepTransition(thread *Thread, contract *Contract, stepName string) []Violation {
    violations := []Violation{}
    
    // 1. Load thread history
    history, _ := s.loadThreadHistory(thread.ID)
    
    // 2. Check valid transition
    if len(history.Steps) > 0 {
        previousStep := history.GetPreviousStepName()
        allowedTransitions := contract.GetAllowedTransitions(previousStep)
        
        isValid := false
        for _, allowed := range allowedTransitions {
            if allowed == stepName {
                isValid = true
                break
            }
        }
        
        if !isValid {
            violations = append(violations, Violation{
                Type:     "invalid_transition",
                Severity: "major",
                Message:  fmt.Sprintf("Invalid transition from '%s' to '%s'", previousStep, stepName),
                Details: map[string]interface{}{
                    "from_step":      previousStep,
                    "to_step":        stepName,
                    "expected_steps": allowedTransitions,
                },
            })
        }
    }
    
    // 3. Check step timeout
    if len(history.Steps) > 0 {
        previousStepRecord := history.GetPreviousStepRecord()
        previousStepDef := contract.GetStep(previousStepRecord.StepName)
        
        if previousStepDef.Timeout != nil {
            elapsed := time.Since(previousStepRecord.Timestamp)
            if elapsed > previousStepDef.Timeout.Duration {
                violations = append(violations, Violation{
                    Type:     "step_timeout_exceeded",
                    Severity: "major",
                    Message:  fmt.Sprintf("Step '%s' exceeded timeout", previousStepRecord.StepName),
                })
            }
        }
    }
    
    // 4. Check max duration
    if contract.Validation.MaxDuration != nil {
        elapsed := time.Since(thread.StartedAt)
        if elapsed > contract.Validation.MaxDuration.Duration {
            violations = append(violations, Violation{
                Type:     "max_duration_exceeded",
                Severity: "major",
                Message:  "Thread exceeded maximum duration",
            })
        }
    }
    
    // 5. Check multiple terminals
    if contract.Validation.AllowMultipleTerminals {
        terminalCount := history.CountTerminalSteps(contract.TerminalSteps)
        if terminalCount > 1 {
            violations = append(violations, Violation{
                Type:     "multiple_terminal_states",
                Severity: contract.Validation.MultipleTerminalsSeverity,
                Message:  "Thread reached multiple terminal states",
            })
        }
    }
    
    // 6. Check missing optional fields (info only)
    stepDef := contract.GetStep(stepName)
    if stepDef.BusinessContext != nil {
        missingOptional := []string{}
        for _, optField := range stepDef.BusinessContext.Optional {
            if _, exists := req.Context[optField]; !exists {
                missingOptional = append(missingOptional, optField)
            }
        }
        
        if len(missingOptional) > 0 {
            violations = append(violations, Violation{
                Type:     "missing_optional_field",
                Severity: "info",
                Message:  "Optional fields missing",
                Details: map[string]interface{}{
                    "missing_optional_fields": missingOptional,
                },
            })
        }
    }
    
    return violations
}
```

---

## Summary: Actual vs Proposed Architecture

| Aspect | Proposed | Actual |
|--------|----------|--------|
| **Step Submission** | REST API `POST /threads/{id}/steps` | WebSocket `recordThreadEvent` |
| **Communication** | HTTP | WebSocket (persistent connection) |
| **SDK Pattern** | Direct API calls | Fluent API: `thread.step().addContext().stop()` |
| **Validation** | Two-phase (blocking + async) | ✅ Same (already implemented) |
| **Storage** | Direct DB write | Cryptographic chain + batched Valkey writes |
| **Idempotency** | Request-based | Hash-based (`stepName:idempotencyKey`) |
| **Audit Trail** | Database | Valkey List + Stream (7-day TTL) |

---

## Implementation Checklist for Contract V3

### ✅ Already Implemented
- [x] WebSocket-based step submission
- [x] Blocking validation (access, role, basic context)
- [x] Cryptographic step chaining
- [x] Batched writes to Valkey
- [x] Idempotency via hash
- [x] Thread-specific sequential processing

### ❌ Needs Implementation

**Blocking Validations:**
- [ ] Entry point validation on thread creation
- [ ] Owner/party validation against contract
- [ ] Required fields validation (business_context.required)
- [ ] Thread completion status check

**Non-Blocking Validations:**
- [ ] Invalid transition detection
- [ ] Step timeout exceeded
- [ ] Max duration exceeded
- [ ] Multiple terminal states
- [ ] Retry limit exceeded
- [ ] Missing optional fields (info)
- [ ] Extra undocumented fields (info)

**WebSocket Events:**
- [ ] Emit `thread_violation` events to subscribers
- [ ] Store violations in audit trail
- [ ] Real-time violation monitoring

**Contract Storage:**
- [ ] Store contracts with entry_points and versioning fields
- [ ] Implement version locking (threads_lock_to_version)
- [ ] Contract validation rules (7 new rules)

This reflects the **real architecture** where everything flows through WebSocket, not REST APIs.
