# CASE 3: `recordThreadEvent` - Step Recording with Complete Validation Flow

**Handler Entry Point**: `/internal/handlers/thread.go:162-165`

**Purpose**: Record a step event within a thread, validate against contract rules, maintain cryptographic hash chain, perform async validations, and publish real-time notifications.

**Complexity**: This is the most complex case, involving 15+ validation checks, atomic operations, and multiple async processes.

---

## Complete Flow Diagram

```
Client Request (recordThreadEvent)
    ↓
WebSocket Handler
    ↓
ThreadService.HandleRecordEvent
    ↓
├─→ Initial Validations (auth, required fields)
├─→ Thread Retrieval (cache-aside)
├─→ Access Control Check
├─→ Thread Status Check
├─→ Idempotency Key Generation/Check
├─→ Contract Validations (if contract exists)
│   ├─→ Step exists in contract
│   ├─→ Entry point validation (if first step)
│   ├─→ Role/owner validation
│   └─→ Context field validation
├─→ Refs Storage (if provided)
├─→ StepEventService.RecordStepEventDirect
│   ├─→ Atomic Hash Chain Update (Lua)
│   └─→ NATS Publish: activity.log
└─→ NotificationService.PerformAsyncValidation (goroutine)
    ├─→ Step State Update (Lua)
    ├─→ Non-Blocking Validations (6 checks)
    ├─→ Terminal Step Detection
    ├─→ Archive Validation Results (NATS)
    └─→ Publish Notifications (NATS → WebSocket clients)
```

---

## Request Structure

```json
{
  "action": "recordThreadEvent",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "stepName": "order_placed",
  "status": "success",
  "startedAt": "2026-01-12T12:00:00Z",
  "finishedAt": "2026-01-12T12:00:05Z",
  "context": {
    "orderId": "12345",
    "amount": "100.00",
    "currency": "USD"
  },
  "refs": {
    "orderId": "12345"
  },
  "idempotencyKey": "abc123",
  "serviceName": "merchant-service"
}
```

---

## Detailed Flow

### **Phase 1: Initial Validations**

**Location**: `/internal/service/thread.go:407-450`

```
ThreadService.HandleRecordEvent(req, ownerID, companyID):

VALIDATION 1: Authentication
├─ Call ConnectionManager.IsConnected(ownerID)
└─ IF not connected → Return error "Not authenticated"

VALIDATION 2: Required Fields
├─ Check threadID != ""
├─ Check stepName != ""
├─ Check status != ""
├─ Check startedAt != ""
├─ Check finishedAt != ""
├─ Check context != nil
└─ IF any missing → Return error "Missing required field: {field}"

VALIDATION 3: Thread Retrieval (Cache-Aside Pattern)
├─ Call ThreadService.GetThread(threadID)
│
├─ Check in-memory cache:
│  └─ CacheManager.GetThread(threadID)
│     └─ IF found → Return cached thread (fastest)
│
├─ Check Valkey:
│  ├─ Command: GET thread:{threadID}
│  ├─ IF found:
│  │  ├─ Deserialize JSON to Thread object
│  │  ├─ Store in memory cache
│  │  └─ Return thread
│  └─ IF not found → Return error "Thread not found"
│
└─ Thread retrieved successfully

VALIDATION 4: Access Control
├─ Call AccessService.CheckThreadAccess(threadID, ownerID, "write", thread)
├─ Command: HGET thread:{threadID}:access:{ownerID} permissions
├─ Parse permissions: "read,write,invite,manage"
├─ Check if "write" is in permissions list
└─ IF not found OR "write" not in list → Return error "Access denied"

VALIDATION 5: Thread Status
├─ Check thread.Status
└─ IF thread.Status == "completed" → Return error "Cannot add steps to completed thread"
```

---

### **Phase 2: Idempotency Handling**

**Location**: `/internal/service/thread.go:452-490`

```
IDEMPOTENCY KEY GENERATION:

IF req.IdempotencyKey == "":
  ├─ Generate from context hash:
  │  ├─ Sort context keys alphabetically
  │  ├─ Build string: "amount:100.00|currency:USD|orderId:12345"
  │  ├─ Hash: sha256(concatenated)
  │  └─ idempotencyKey = first 8 chars of hash
  │
  └─ Log: "Auto-generated idempotency key: {key}"
ELSE:
  └─ Use provided idempotencyKey

DUPLICATE CHECK:
├─ stepKey = stepName + ":" + idempotencyKey
│  Example: "order_placed:abc123"
│
├─ Command: HGET thread:{threadID}:steps:{stepKey} status
│
├─ IF status == "completed":
│  └─ Return RecordEventResponse{
│       status: "success",
│       message: "Step already completed (duplicate)",
│       isDuplicate: true
│     }
│
├─ IF status in ["pending", "violated", "failed"]:
│  └─ Allow retry (continue processing)
│
└─ IF not found:
   └─ First time seeing this step (continue)
```

---

### **Phase 3: Contract Validation**

**Location**: `/internal/service/thread.go:492-570`

**Only executed if `thread.ContractName != ""`**

```
CONTRACT VALIDATION:

Get Contract Graph:
├─ Call ContractValidator.GetContractGraph(contractName, version, companyID)
└─> Returns cached ContractGraph (from 3-tier cache)

VALIDATION 1: Step Exists in Contract
├─ Check: graph.Nodes[stepName]
└─ IF not found → Return error "Step '{stepName}' not found in contract '{contractName}'"

VALIDATION 2: Entry Point (if first step)
├─ Check if thread has successful steps:
│  └─ Command: ZCARD thread:{threadID}:current_steps
│     └─> Returns count of completed steps
│
├─ IF count == 0 (first step):
│  ├─ Check if stepName in graph.EntryPoints
│  └─ IF not → Return error "Thread must start with entry point. Valid entry points: {list}"
│
└─ ELSE: Skip entry point validation

VALIDATION 3: Role/Owner Requirement
├─ Get stepNode from graph.Nodes[stepName]
│
├─ IF stepNode.Owner != "":
│  │
│  ├─ Validate role is in contract parties:
│  │  └─ IF stepNode.Owner NOT IN graph.Parties 
│  │     → Return error "Invalid owner '{owner}' in contract"
│  │
│  └─ Validate user has required role:
│     ├─ Call AccessService.ValidateUserRoleForStep(threadID, ownerID, stepNode.Owner)
│     ├─ Command: HGET thread:{threadID}:access:{ownerID} roles
│     ├─ Parse roles JSON: ["merchant"]
│     ├─ Check if stepNode.Owner in roles
│     └─ IF not → Return error "Access denied: Step '{stepName}' requires role '{owner}'"
│
└─ Continue

VALIDATION 4: Context Fields
├─ Call ContractValidator.ValidateStepContext(stepNode, req.Context)
│
├─ FOR EACH required field in stepNode.RequiredFields:
│  └─ Check if field exists in req.Context
│     └─ IF missing → Return error "Missing required field: {field}"
│
└─ Continue (optional fields checked in async validation)
```

---

### **Phase 4: Refs Storage**

**Location**: `/internal/service/thread.go:572-590`

```
IF req.Refs != nil && len(req.Refs) > 0:

  Valkey Write:
  ├─ Call ThreadRepository.AddRefs(threadID, refs)
  ├─ FOR EACH key, value in refs:
  │  └─ Command: HSET thread:{threadID}:refs {key} {value}
  └─ Return

  Async NATS Publish (goroutine):
  └─ FOR EACH key, value in refs:
     ├─ Build ref event: {threadId, refKey, refValue, action: "ref_added"}
     ├─ Call: natsArchivalPublisher.PublishThreadMetadata(ctx, refEvent)
     └─> NATS JetStream → Archiver → PostgreSQL thread_refs table
```

---

### **Phase 5: Step Event Processing - Hash Chain**

**Location**: `/internal/service/step_event.go:44-120`

```
Generate Step ID:
stepID = uuid.New().String()
└─> Example: "step-uuid-123"

Get Service Name:
├─ IF req.ServiceName != "": Use req.ServiceName
├─ ELSE: Get from ConnectionManager.GetClient(ownerID).ServiceName
└─ IF still empty: serviceName = "unknown"

Create StepEvent:
stepEvent = {
  StepID: stepID,
  ThreadID: threadID,
  StepName: "order_placed",
  ServiceName: "merchant-service",
  Type: req.Type,
  Status: "success",
  Context: {orderId: "12345", amount: "100.00", currency: "USD"},
  StartedAt: "2026-01-12T12:00:00Z",
  FinishedAt: "2026-01-12T12:00:05Z",
  Timestamp: time.Now(),
  IdempotencyKey: "abc123"
}

Call StepEventService.RecordStepEventDirect(stepEvent, ownerID, serviceName):

┌─────────────────────────────────────────────────────────────────┐
│ ATOMIC HASH CHAIN UPDATE (Two-Step Lua Script)                  │
└─────────────────────────────────────────────────────────────────┘

STEP 1: Get Current Thread Hash
├─ Lua Script:
│  ```lua
│  local threadKey = KEYS[1]  -- thread:{threadID}
│  local threadData = redis.call('GET', threadKey)
│  if not threadData then
│    return {err = "Thread not found"}
│  end
│  local threadObj = cjson.decode(threadData)
│  local oldHash = threadObj.lastHash or ""
│  return {oldHash}
│  ```
│
├─ Command: EVAL {script} 1 thread:{threadID}
└─ Returns: oldHash (e.g., "sha256:xyz..." or "")

STEP 2: Calculate New Hash (in Go)
├─ Build hash input:
│  hashData = oldHash + ":" + threadID + ":" + stepID + ":" + idempotencyKey + ":" + timestamp
│  Example: "sha256:xyz...:550e8400:step-uuid-123:abc123:2026-01-12T12:00:05Z"
│
├─ Calculate: sha256(hashData)
└─ newHash = "sha256:" + hex(hash)

STEP 3: Update Thread with New Hash
├─ Lua Script:
│  ```lua
│  local threadKey = KEYS[1]
│  local threadData = redis.call('GET', threadKey)
│  if not threadData then
│    return {err = "Thread not found"}
│  end
│  local threadObj = cjson.decode(threadData)
│  threadObj.lastHash = ARGV[1]  -- newHash
│  local updatedThread = cjson.encode(threadObj)
│  redis.call('SET', threadKey, updatedThread, 'EX', 86400)
│  return {ARGV[1]}
│  ```
│
├─ Command: EVAL {script} 1 thread:{threadID} {newHash}
└─ Thread now has updated lastHash field

┌─────────────────────────────────────────────────────────────────┐
│ ASYNC ACTIVITY LOG (Goroutine)                                  │
└─────────────────────────────────────────────────────────────────┘

Build Activity Event:
activityEvent = {
  type: "step_recorded",
  thread_id: threadID,
  step_id: "order_placed:abc123",
  step_name: "order_placed",
  step_uuid: stepID,
  idempotency_key: "abc123",
  timestamp: "2026-01-12T12:00:05Z",
  context: {orderId: "12345", amount: "100.00", currency: "USD"},
  actor: ownerID,
  actor_service: "merchant-service",
  status: "success",
  hash: newHash,
  prev_hash: oldHash,
  started_at: "2026-01-12T12:00:00Z",
  finished_at: "2026-01-12T12:00:05Z"
}

NATS Publish:
├─ Call: natsArchivalPublisher.PublishActivityLog(ctx, activityEvent)
├─ Subject: "activity.log"
└─> NATS JetStream → Archiver → PostgreSQL thread_activities table
```

---

### **Phase 6: Async Validation & Notifications**

**Location**: `/internal/service/notification_service.go:60-220`

**Triggered for statuses: success, failed, error**

```
Call NotificationService.PerformAsyncValidation(
  threadID, stepID, stepName, ownerID, req, thread, graph, stepNode
):

SPAWN GOROUTINE:
go func() {
  defer func() {
    if r := recover(); r != nil {
      log.Printf("❌ PANIC in async validation: %v", r)
    }
  }()

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.1: Handle No-Contract Case                                  │
  └───────────────────────────────────────────────────────────────┘

  IF graph == nil (no contract):
    ├─ Create immediate notification based on status:
    │  notification = {
    │    notificationID: uuid,
    │    threadID: threadID,
    │    stepID: stepID,
    │    stepName: stepName,
    │    ownerID: ownerID,
    │    contractName: thread.ContractName,
    │    stepStatus: req.Status,
    │    status: "passed" | "failed" | "error",
    │    violationType: "",
    │    severity: "info",
    │    message: "Step '{stepName}' {status} (no contract)",
    │    details: {},
    │    timestamp: time.Now()
    │  }
    │
    └─ Publish to NATS:
       ├─ Subject: "notifications.{threadID}.{stepName}"
       └─> NotificationRouter → WebSocket clients
    
    RETURN (skip contract validations)

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.2: Step State Update (Atomic Lua Script)                    │
  └───────────────────────────────────────────────────────────────┘

  Call StepStateRepository.UpdateStepState(threadID, stepName, idempotencyKey, status):

  Lua Script Logic:
  ```lua
  local stepKey = KEYS[1]  -- thread:{threadID}:steps:{stepName}:{idempotencyKey}
  local currentStepsKey = KEYS[2]  -- thread:{threadID}:current_steps
  local status = ARGV[1]  -- "success" | "failed" | "error"
  local stepID = ARGV[2]
  local timestamp = ARGV[3]
  local retryLimit = tonumber(ARGV[4])  -- From contract
  
  -- Get current state
  local currentState = redis.call('HGETALL', stepKey)
  local retryCount = tonumber(currentState.retryCount or 0)
  local exceeded = false
  
  if status == "success" then
    -- Mark as completed
    redis.call('HSET', stepKey,
      'status', 'completed',
      'retryCount', 0,
      'id', stepID,
      'lastUpdatedAt', timestamp
    )
    -- Add to current_steps sorted set
    redis.call('ZADD', currentStepsKey, timestamp, stepName)
    
  elseif status == "failed" or status == "error" then
    -- Increment retry count
    retryCount = retryCount + 1
    redis.call('HINCRBY', stepKey, 'retryCount', 1)
    redis.call('HSET', stepKey, 'lastUpdatedAt', timestamp)
    
    -- Check retry limit
    if retryCount > retryLimit then
      redis.call('HSET', stepKey, 'status', 'violated')
      exceeded = true
    end
  end
  
  -- Set firstSeenAt if not exists
  if not currentState.firstSeenAt then
    redis.call('HSET', stepKey, 'firstSeenAt', timestamp)
  end
  
  return {status, retryCount, exceeded}
  ```

  Result in Valkey:
  Key: thread:{threadID}:steps:order_placed:abc123
  Hash:
  {
    status: "completed",
    retryCount: "0",
    id: stepID,
    firstSeenAt: "2026-01-12T12:00:00Z",
    lastUpdatedAt: "2026-01-12T12:00:05Z",
    previousStep: ""
  }

  Key: thread:{threadID}:current_steps
  Sorted Set: {("order_placed", score=timestamp)}

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.3: Non-Blocking Validations                                 │
  └───────────────────────────────────────────────────────────────┘

  Initialize notifications array: []

  STRUCTURAL VALIDATIONS (Run for ALL statuses):

  1. CheckStepTimeout():
     ├─ Get step duration: finishedAt - startedAt = 5 seconds
     ├─ Get timeout from contract: stepNode.Timeout = 300 seconds
     ├─ IF duration > timeout:
     │  └─ Add notification: {
     │       type: "step_timeout",
     │       severity: "critical",
     │       passed: false,
     │       message: "Step exceeded timeout",
     │       details: {duration: 5, timeout: 300}
     │     }
     └─ ELSE:
        └─ Add notification: {type: "step_timeout", severity: "info", passed: true}

  2. CheckMaxDuration():
     ├─ Get thread duration: time.Now() - thread.StartedAt
     ├─ Get max duration from contract: graph.MaxDuration = 86400
     ├─ IF duration > maxDuration:
     │  └─ Add notification: {
     │       type: "max_duration",
     │       severity: "critical",
     │       passed: false,
     │       message: "Thread exceeded max duration"
     │     }
     └─ ELSE:
        └─ Add notification: {type: "max_duration", severity: "info", passed: true}

  3. CheckMultipleTerminalStates():
     ├─ Get all completed steps: ZRANGE thread:{threadID}:current_steps 0 -1
     ├─ Count terminal steps in completed list
     ├─ IF count > 1:
     │  └─ Add notification: {
     │       type: "multiple_terminal_states",
     │       severity: "critical",
     │       passed: false,
     │       message: "Multiple terminal steps detected"
     │     }
     └─ ELSE:
        └─ Add notification: {type: "multiple_terminal_states", severity: "info", passed: true}

  4. Retry Limit (already checked in Lua script):
     └─ IF exceeded == true:
        └─ Add notification: {
             type: "retry_limit",
             severity: "critical",
             passed: false,
             message: "Step exceeded retry limit"
           }

  BUSINESS VALIDATIONS (Only if status == "success"):

  5. CheckInvalidTransition():
     ├─ Get previous steps: ZRANGE thread:{threadID}:current_steps 0 -1
     ├─ Check if current step is reachable from previous steps:
     │  └─ FOR EACH previousStep:
     │     └─ Check if stepName in graph.Nodes[previousStep].NextSteps
     ├─ IF not reachable AND not entry point:
     │  └─ Add notification: {
     │       type: "invalid_transition",
     │       severity: "critical",
     │       passed: false,
     │       message: "Invalid step transition",
     │       details: {from: previousSteps, to: stepName}
     │     }
     └─ ELSE:
        └─ Add notification: {type: "invalid_transition", severity: "info", passed: true}

  6. CheckMissingOptionalFields():
     ├─ Get optional fields from contract: stepNode.OptionalFields
     ├─ Check which optional fields are missing in req.Context
     ├─ IF any missing:
     │  └─ Add notification: {
     │       type: "missing_optional_fields",
     │       severity: "info",
     │       passed: false,
     │       details: {missing: ["notes"]}
     │     }
     └─ ELSE:
        └─ Add notification: {type: "missing_optional_fields", severity: "info", passed: true}

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.4: Terminal Step Detection                                  │
  └───────────────────────────────────────────────────────────────┘

  IF stepNode.IsTerminal == true && req.Status == "success":

    Update Thread Status:
    ├─ Get thread from Valkey: GET thread:{threadID}
    ├─ Parse JSON, update: thread.Status = "completed", thread.CompletedAt = time.Now()
    ├─ Serialize and save: SET thread:{threadID} {json} EX 86400
    └─ Update memory cache

    Async Archive Thread Completion:
    ├─ Subject: "metadata.thread"
    ├─ Payload: {threadId, status: "completed", completedAt, lastHash}
    └─> Archiver → PostgreSQL threads table (UPDATE status="completed")

    Add Notification:
    └─ {type: "step_completed", severity: "info", message: "Thread completed", passed: true}

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.5: Archive Validation Results                               │
  └───────────────────────────────────────────────────────────────┘

  Build Validation Summary:
  ├─ Count by severity: criticalCount, warningCount, minorCount, infoCount
  ├─ Determine overall status:
  │  - IF any critical failed → "violated"
  │  - ELSE IF any warning failed → "warning"
  │  - ELSE → "passed"
  └─ hasCriticalViolation = (criticalCount > 0)

  Call ActivityRepository.ArchiveValidationResults(threadID, stepID, notifications):

  Build Payload:
  validationPayload = {
    validationID: uuid,
    threadID: threadID,
    stepID: stepID,
    stepName: stepName,
    idempotencyKey: idempotencyKey,
    timestamp: time.Now(),
    validations: JSON array of all notifications,
    overallStatus: "passed" | "violated" | "warning",
    hasCriticalViolation: true | false,
    criticalCount: 0,
    warningCount: 0,
    minorCount: 0,
    infoCount: 6,
    totalValidations: 6
  }

  NATS Publish:
  ├─ Call: natsArchivalPublisher.PublishThreadValidation(ctx, validationPayload)
  ├─ Subject: "validations.thread"
  └─> NATS JetStream → Archiver → PostgreSQL thread_validations table

  ┌───────────────────────────────────────────────────────────────┐
  │ 6.6: Publish Notifications to Clients                         │
  └───────────────────────────────────────────────────────────────┘

  FOR EACH notification in notifications:
    ├─ Call NATSPublisher.PublishNotification(notification)
    ├─ Subject: "notifications.{threadID}.{stepName}"
    ├─ Payload: {notification object}
    └─> NATS → NotificationRouter.HandleNotification()
       │
       └─> Filter and send to subscribed WebSocket clients:
           ├─ FOR EACH client in router.clients:
           │  ├─ Check if client subscribed to threadID
           │  ├─ Check if client subscribed to stepName
           │  ├─ Check if client subscribed to eventType
           │  └─ IF all match:
           │     ├─ Send notification via WebSocket
           │     └─ Store in client.pendingAcks (waiting for client ACK)
           │
           └─ Log: "Published {count} notifications"
}()
```

---

## Database Impact Summary

### **Valkey (Immediate Writes)**

1. **`thread:{threadID}`** → Updated with new lastHash
2. **`thread:{threadID}:refs`** → Hash of refs (if provided)
3. **`thread:{threadID}:steps:{stepName}:{idempotencyKey}`** → Step state hash
4. **`thread:{threadID}:current_steps`** → Sorted set of completed steps
5. **`thread:{threadID}`** → Updated status="completed" (if terminal step)

### **NATS JetStream (Async Publish)**

1. **Stream: `activity_log`** → Step recorded event with hash chain
2. **Stream: `thread_metadata`** → Refs (if provided)
3. **Stream: `validations.thread`** → Validation results summary
4. **Stream: `notifications.{threadID}.{stepName}`** → Real-time notifications (6 in example)
5. **Stream: `thread_metadata`** → Thread completion (if terminal step)

### **PostgreSQL (Eventually Consistent)**

1. **`thread_activities`** → INSERT step_recorded event
2. **`thread_refs`** → INSERT/UPDATE refs
3. **`thread_validations`** → INSERT validation summary
4. **`threads`** → UPDATE status="completed" (if terminal step)

---

## Performance Characteristics

**Total Response Time**: ~5-15ms (synchronous path only)

1. **Validations**: ~2-5ms
2. **Hash chain update**: ~2-3ms (Lua scripts)
3. **Async validations**: 10-50ms (non-blocking)

**Async Processing**:
- Step event archival: ~5-10ms
- Validation checks: ~10-30ms
- Notification publishing: ~5-15ms per notification

---

## Error Scenarios

1. **Duplicate step**: Return success with isDuplicate=true
2. **Invalid transition**: Notification with severity=critical
3. **Retry limit exceeded**: Step marked as "violated"
4. **Terminal step**: Thread marked as "completed"
5. **Missing required fields**: Blocked (synchronous validation)
6. **Missing optional fields**: Warning notification (async)

---

*This completes the detailed flow for CASE 3: recordThreadEvent*
