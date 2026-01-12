# ThreadifyEngine - Complete WebSocket Server Flow Analysis

**Detailed Project Assessment: WebSocket Handler to Database Registration**

This document provides a complete, detailed trace of each WebSocket action from the handler entry point through all service layers to final database persistence. Every step is documented with actual code locations, data structures, and database operations.

---

## Architecture Overview

### **Data Flow Layers**

1. **Hot Path (Valkey - Immediate)**: Sub-millisecond operational queries, synchronous writes
2. **Warm Path (NATS JetStream - Async)**: Reliable message delivery with at-least-once guarantees
3. **Cold Path (PostgreSQL - Eventually Consistent)**: Durable archival storage via Archiver service
4. **In-Memory State**: Session management, caching, and WebSocket connections

### **Key Architectural Patterns**

- **Cache-Aside**: Check cache → Miss → Load from source → Populate cache
- **Write-Through**: Write to primary store → Async write to archive
- **Event Sourcing**: All state changes published as events to NATS
- **CQRS**: Separate write path (Valkey) from read path (Valkey → PostgreSQL)
- **At-Least-Once Delivery**: NATS JetStream with consumer groups and explicit ACK

---

## WebSocket Handler Cases

### **CASE 1: `connect`**

**Handler Entry Point**: `/internal/handlers/thread.go:115-138`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:115-138)
   ├─ Unmarshal ConnectRequest from message
   ├─ Call ThreadService.HandleConnect(req)
   │
   └─> 2. THREAD SERVICE (thread.go:152-188)
       ├─ Validate: apiKey is not empty
       ├─ Call AuthService.ValidateApiKey(apiKey)
       │  └─> Decode JWT token → extract ownerID, companyID, serviceName
       │
       ├─ Call ConnectionManager.ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID)
       │  └─> Store in-memory connection state (map[ownerID] → Client)
       │
       └─ Return ConnectResponse{ownerID, companyID, status: "success"}

3. WEBSOCKET HANDLER (post-response)
   ├─ Store ownerID and companyID in session
   ├─ Store session in sessions map: sessions[ownerID] = session
   ├─ Register client with NotificationRouter
   │  └─> Create WebSocketClient{ID, Conn, OwnerID, ThreadIDs, pendingAcks}
   │  └─> Store in NotificationRouter.clients[clientID]
   │
   └─ Send ConnectResponse to client
```

**Database Impact**:
- **NONE** - Pure in-memory operation
- **In-Memory State:**
  - ConnectionManager: `map[ownerID] → Client{apiKey, serviceName, companyID}`
  - WebSocketHandler.sessions: `map[ownerID] → Session`
  - NotificationRouter.clients: `map[clientID] → WebSocketClient`

---

### **CASE 2: `startThread`**

**Handler Entry Point**: `/internal/handlers/thread.go:140-160`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:140-160)
   ├─ Unmarshal StartThreadRequest from message
   ├─ Call ThreadService.HandleStartThread(req, ownerID, companyID)
   │
   └─> 2. THREAD SERVICE (thread.go:190-394)
       ├─ Validate: ownerID is connected
       ├─ Validate: if contractName provided, role is required
       │
       ├─ IF contractName provided:
       │  ├─ Parse contract identifier (name:version or name)
       │  ├─ Call ContractValidator.LoadContractGraphIntoCache(name, version, companyID)
       │  │  └─> Three-tier cache check:
       │  │      1. In-memory cache (CacheService)
       │  │      2. Valkey cache (graph:{companyID}:{contractName}:{version})
       │  │      3. PostgreSQL (contracts table)
       │  │      └─> Cache result in Valkey + memory
       │  └─ Store actual version loaded
       │
       ├─ Generate threadID = uuid.New()
       ├─ Create Thread object:
       │  {
       │    ID: threadID,
       │    ContractID: contractName (nullable),
       │    ContractName: parsedContractName,
       │    OwnerID: ownerID,
       │    CompanyID: companyID,
       │    Status: "active",
       │    StartedAt: time.Now()
       │  }
       │
       ├─> 3. VALKEY WRITE - Thread Repository (valkey/thread.go)
       │   ├─ Key: thread:{threadID}
       │   ├─ Value: JSON serialized Thread object
       │   ├─ TTL: threadTTLSeconds (configurable, typically 86400 = 24h)
       │   └─ Command: SET thread:{threadID} {json} EX {ttl}
       │
       ├─ Cache thread in memory: CacheManager.SetThread(threadID, thread)
       │
       ├─> 4. ACCESS CONTROL - Grant Creator Access
       │   ├─ Call GrantOrUpdateThreadAccess(threadID, ownerID, role, permissions, "self", isCreator=true)
       │   │
       │   └─> 4a. SCOPE RESOLUTION
       │       ├─ Call ScopeResolver.ResolveScope(threadID, userID, role, isCreator=true)
       │       ├─ Logic: isCreator=true → scope = "owner"
       │       │
       │       └─> 4b. VALKEY WRITE - Access Repository (atomic Lua script)
       │           ├─ Key: thread:{threadID}:access:{ownerID}
       │           ├─ Hash fields:
       │           │  {
       │           │    roles: JSON array [role],
       │           │    permissions: comma-separated string,
       │           │    grantedBy: "self",
       │           │    grantedAt: timestamp,
       │           │    status: "active"
       │           │  }
       │           ├─ Command: HSET + SADD (via Lua script for atomicity)
       │           │  - HSET thread:{threadID}:access:{ownerID} {fields}
       │           │  - SADD thread:{threadID}:users {ownerID}
       │           │
       │           └─> 4c. ASYNC ACTIVITY LOG (goroutine)
       │               ├─ Write to Valkey LIST:
       │               │  ├─ Key: thread:{threadID}:activity
       │               │  ├─ Command: LPUSH thread:{threadID}:activity {json}
       │               │  ├─ TTL: EXPIRE thread:{threadID}:activity 604800 (7 days)
       │               │  └─ Event: {type: "access_granted", threadID, userID, role, permissions, scope}
       │               │
       │               └─ Publish to NATS JetStream (nats/archival_publisher.go)
       │                  ├─ Subject: "access.thread"
       │                  ├─ Payload: {threadID, userId, roles, permissions, grantedBy, scope}
       │                  └─> NATS → Archiver → PostgreSQL (thread_access table)
       │
       └─> 5. ASYNC THREAD METADATA ARCHIVAL (goroutine in thread.go:278-386)
           │
           ├─> 5a. NATS PUBLISH - Thread Metadata
           │   ├─ Subject: "metadata.thread"
           │   ├─ Payload: {threadId, ownerId, companyId, contractId, contractName, contractVersion, startedAt}
           │   └─> NATS → Archiver → PostgreSQL (threads table)
           │
           ├─> 5b. IF refs provided: NATS PUBLISH - Thread Refs (loop)
           │   ├─ Subject: "metadata.thread"
           │   ├─ Payload: {threadId, refKey, refValue, action: "ref_added"}
           │   └─> NATS → Archiver → PostgreSQL (thread_refs table)
           │
           ├─> 5c. VALKEY WRITE - Activity List
           │   ├─ Key: thread:{threadID}:activity
           │   ├─ Command: LPUSH thread:{threadID}:activity {json}
           │   ├─ TTL: EXPIRE 604800 (7 days)
           │   └─ Event: {type: "thread_created", threadID, ownerID, contractID, role, timestamp}
           │
           └─> 5d. NATS PUBLISH - Activity Log
               ├─ Subject: "activity.log"
               ├─ Payload: {type: "thread_created", thread_id, owner_id, actor, actor_service, contract_id, role}
               └─> NATS → Archiver → PostgreSQL (activity_log table)

6. WEBSOCKET HANDLER (post-response)
   ├─ Add threadID to session.threadIDs
   ├─ Subscribe to notifications (old consumer - NATS queue subscription)
   ├─ Subscribe via NotificationRouter (new router)
   │  └─> NotificationRouter.SubscribeToThread(clientID, threadID)
   │      └─> Store in client.ThreadIDs[threadID] = true
   │
   └─ Send StartThreadResponse{threadID, status: "success"}
```

**Database Impact**:

**Valkey (Immediate Writes)**:
1. `thread:{threadID}` → Thread JSON (TTL: 24h)
2. `thread:{threadID}:access:{ownerID}` → Access hash
3. `thread:{threadID}:users` → Set of user IDs
4. `thread:{threadID}:activity` → List of activity events (TTL: 7 days)

**NATS JetStream (Async Publish)**:
1. Subject: `metadata.thread` → Thread metadata
2. Subject: `metadata.thread` → Refs (if provided)
3. Subject: `access.thread` → Access granted event
4. Subject: `activity.log` → Thread created event

**PostgreSQL (Via Archiver - Eventually Consistent)**:
1. `threads` table → INSERT thread metadata
2. `thread_refs` table → INSERT refs (if provided)
3. `thread_access` table → INSERT access record
4. `activity_log` table → INSERT thread_created event

---

### **CASE 3: `recordThreadEvent` (Step Recording)**

**Handler Entry Point**: `/internal/handlers/thread.go:162-165`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:162-165)
   ├─ Unmarshal RecordEventRequest from message
   ├─ Call ThreadService.HandleRecordEvent(req, ownerID, companyID)
   │
   └─> 2. THREAD SERVICE (thread.go:407-700)
       ├─ Validate: ownerID is connected
       ├─ Validate: required fields (threadID, stepName, status, startedAt, finishedAt, context)
       │
       ├─> 3. THREAD RETRIEVAL (cache-aside pattern)
       │   ├─ Check CacheManager.GetThread(threadID)
       │   ├─ IF cache miss:
       │   │  ├─ Valkey GET thread:{threadID}
       │   │  └─ Cache result in memory
       │   └─ Return thread object
       │
       ├─> 4. ACCESS CONTROL CHECK
       │   ├─ Call AccessService.CheckThreadAccess(threadID, ownerID, "write", thread)
       │   ├─ Valkey HGET thread:{threadID}:access:{ownerID} permissions
       │   └─ Verify "write" permission exists
       │
       ├─ Validate: thread.Status != "completed" (cannot add steps to completed threads)
       │
       ├─> 5. IDEMPOTENCY KEY GENERATION
       │   ├─ IF idempotencyKey not provided:
       │   │  └─ Generate from context hash: sha256(sorted context fields)
       │   │
       │   └─ Check for duplicate:
       │      ├─ stepKey = stepName:idempotencyKey
       │      ├─ Valkey HGET thread:{threadID}:steps:{stepKey} status
       │      └─ IF status == "completed" → Reject as duplicate
       │
       ├─> 6. CONTRACT VALIDATION (if thread has contract)
       │   ├─ Get contract graph (three-tier cache)
       │   ├─ Validate: step exists in contract
       │   ├─ IF first step: Validate is entry point
       │   ├─ IF step has owner requirement:
       │   │  ├─ Validate role is in contract parties
       │   │  └─ Call AccessService.ValidateUserRoleForStep(threadID, ownerID, stepOwner)
       │   └─ Validate step context against contract schema
       │
       ├─ Generate stepID = uuid.New()
       ├─ Get serviceName from request or connection manager
       │
       ├─> 7. REFS STORAGE (if provided)
       │   ├─ Call ThreadRepository.AddRefs(threadID, refs)
       │   │  └─> Valkey HSET thread:{threadID}:refs {key: value}
       │   │
       │   └─ ASYNC: Publish refs to NATS
       │      ├─ Subject: "metadata.thread"
       │      └─> NATS → Archiver → PostgreSQL (thread_refs table)
       │
       ├─> 8. STEP EVENT PROCESSING (step_event.go:44-69)
       │   ├─ Call StepEventService.RecordStepEventDirect(stepEvent, ownerID, serviceName)
       │   │
       │   └─> 8a. ATOMIC HASH GENERATION (Lua script)
       │       ├─ Script 1: Get current thread hash
       │       │  ├─ Valkey GET thread:{threadID}
       │       │  ├─ Parse JSON → extract lastHash
       │       │  └─ Return oldHash
       │       │
       │       ├─ Calculate new hash in Go:
       │       │  └─ newHash = sha256(oldHash:threadID:stepID:idempotencyKey:timestamp)
       │       │
       │       └─> Script 2: Update thread with new hash
       │           ├─ Valkey GET thread:{threadID}
       │           ├─ Update JSON: threadObj.lastHash = newHash
       │           ├─ Valkey SET thread:{threadID} {updatedJSON} EX 86400
       │           └─> Return newHash
       │
       │       └─> 8b. ASYNC ACTIVITY LOG (goroutine)
       │           ├─ Subject: "activity.log"
       │           ├─ Payload: {
       │           │    type: "step_recorded",
       │           │    thread_id, step_id, step_name, step_uuid,
       │           │    idempotency_key, timestamp, context,
       │           │    actor, actor_service, status,
       │           │    hash: newHash, prev_hash: oldHash
       │           │  }
       │           └─> NATS → Archiver → PostgreSQL (activity_log table)
       │
       └─> 9. ASYNC VALIDATION (if status in [success, failed, error])
           ├─ Call NotificationService.PerformAsyncValidation(threadID, stepID, stepName, ownerID, req, thread, graph, stepNode)
           │
           └─> 9a. VALIDATION GOROUTINE (notification_service.go:60-220)
               │
               ├─ IF no contract:
               │  ├─ Create immediate notification (passed/failed/error based on status)
               │  └─> Publish to NATS: "notifications.{threadID}.{stepName}"
               │      └─> NotificationRouter → WebSocket clients
               │
               ├─ IF contract exists:
               │  │
               │  ├─> 9b. STEP STATE UPDATE (Atomic Lua Script)
               │  │   ├─ Call StepStateRepository.UpdateStepState(threadID, stepName, idempotencyKey, status)
               │  │   ├─ Lua Script Logic:
               │  │   │  ├─ Key: thread:{threadID}:steps:{stepName}:{idempotencyKey}
               │  │   │  ├─ IF status == "success":
               │  │   │  │  ├─ HSET status="completed", retryCount=0
               │  │   │  │  ├─ ZADD thread:{threadID}:current_steps {score} {stepName}
               │  │   │  │  └─ Update previousStep tracking
               │  │   │  ├─ IF status == "failed" or "error":
               │  │   │  │  ├─ HINCRBY retryCount +1
               │  │   │  │  ├─ Check retry limit (from contract)
               │  │   │  │  └─ IF exceeded → HSET status="violated"
               │  │   │  └─ HSET firstSeenAt, lastUpdatedAt, id (stepID)
               │  │   └─> Returns: {status, retryCount, exceeded}
               │  │
               │  ├─> 9c. NON-BLOCKING VALIDATIONS (notification_service.go:160-220)
               │  │   │
               │  │   ├─ STRUCTURAL VALIDATIONS (run for ALL statuses):
               │  │   │  1. CheckStepTimeout() → Check step duration
               │  │   │  2. CheckMaxDuration() → Check thread total duration
               │  │   │  3. CheckMultipleTerminalStates() → Detect multiple terminal steps
               │  │   │  4. Retry limit → Already checked in Lua script above
               │  │   │
               │  │   ├─ BUSINESS VALIDATIONS (only if status == "success"):
               │  │   │  5. CheckInvalidTransition() → Validate step order against contract graph
               │  │   │  6. CheckMissingOptionalFields() → Check optional context fields
               │  │   │
               │  │   └─ Generate ValidationNotification[] for each violation
               │  │
               │  ├─> 9d. TERMINAL STEP DETECTION
               │  │   ├─ IF stepNode.IsTerminal && status == "success":
               │  │   │  ├─ Update thread status to "completed"
               │  │   │  ├─ Valkey: Update thread:{threadID} JSON
               │  │   │  └─> ASYNC: Archive thread metadata to NATS
               │  │   │      └─> Subject: "metadata.thread"
               │  │   │          └─> NATS → Archiver → PostgreSQL (threads.status = "completed")
               │  │   │
               │  │   └─ Add notification: {type: "step_completed", severity: "info"}
               │  │
               │  ├─> 9e. ARCHIVE VALIDATION RESULTS
               │  │   ├─ Call ActivityRepository.ArchiveValidationResults(threadID, stepID, notifications)
               │  │   └─> NATS PUBLISH
               │  │       ├─ Subject: "validations.thread"
               │  │       ├─ Payload: {
               │  │       │    validationID, threadID, stepID, stepName, idempotencyKey,
               │  │       │    validations: JSON array of all notifications,
               │  │       │    overallStatus, hasCriticalViolation,
               │  │       │    criticalCount, warningCount, minorCount, infoCount
               │  │       │  }
               │  │       └─> NATS → Archiver → PostgreSQL (validation_results table)
               │  │
               │  └─> 9f. PUBLISH NOTIFICATIONS TO CLIENTS
               │      ├─ FOR EACH notification in notifications:
               │      │  ├─ Call NATSPublisher.PublishNotification(notification)
               │      │  ├─ Subject: "notifications.{threadID}.{stepName}"
               │      │  ├─ Payload: {
               │      │  │    notificationID, threadID, stepID, stepName,
               │      │  │    ownerID, contractName, stepStatus, status,
               │      │  │    violationType, severity, message, details, timestamp
               │      │  │  }
               │      │  └─> NATS → NotificationRouter → WebSocket clients
               │      │      └─> Filtered by client subscriptions (stepName, eventType)
               │      │
               │      └─ Log: "[ASYNC-VALIDATION] Published {count} notifications"

10. WEBSOCKET HANDLER (post-response)
    └─ Send RecordEventResponse{threadID, stepID, status: "success"}
```

**Database Impact**:

**Valkey (Immediate Writes)**:
1. `thread:{threadID}` → Updated with new lastHash
2. `thread:{threadID}:refs` → Hash of refs (if provided)
3. `thread:{threadID}:steps:{stepName}:{idempotencyKey}` → Step state hash (async in validation)
4. `thread:{threadID}:current_steps` → Sorted set of active steps (async in validation)

**NATS JetStream (Async Publish)**:
1. Subject: `activity.log` → Step recorded event (with hash chain)
2. Subject: `metadata.thread` → Refs (if provided)
3. Subject: `validations.thread` → Validation results (async)
4. Subject: `notifications.{threadID}.{stepName}` → Real-time notifications (async)
5. Subject: `metadata.thread` → Thread completion (if terminal step)

**PostgreSQL (Via Archiver - Eventually Consistent)**:
1. `activity_log` table → INSERT step_recorded event
2. `thread_refs` table → INSERT/UPDATE refs
3. `validation_results` table → INSERT validation summary
4. `threads` table → UPDATE status="completed" (if terminal step)

---

### **CASE 4: `inviteParty`**

**Handler Entry Point**: `/internal/handlers/thread.go:172-175`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:172-175)
   ├─ Unmarshal InvitePartyRequest from message
   ├─ Call handleInviteParty(session, req)
   │
   └─> 2. HANDLER HELPER (thread.go:219-246)
       ├─ Validate: req.Action == "inviteParty"
       ├─ Get threadIDs from session (use first thread)
       ├─ Call ThreadService.HandleInviteParty(req, ownerID, companyID, threadIDs)
       │
       └─> 3. THREAD SERVICE (thread.go:702-775)
           ├─ Set default permissions if not provided: "read,write"
           ├─ Parse expiry: InvitationService.ParseExpiry(req.ExpiresIn)
           │  └─> Convert "1h", "24h", "7d" to time.Duration
           │
           ├─ Get threadID from session's threadIDs (first available)
           ├─ Get thread object (cache-aside pattern)
           │
           ├─> 4. CONTRACT VALIDATION
           │   ├─ Get contract graph for thread
           │   ├─ Validate: role exists in contract.Parties
           │   └─ IF role not in parties → Error
           │
           ├─> 5. PERMISSION VALIDATION
           │   ├─ Call InvitationService.ValidatePermissions(permissions)
           │   └─> Check permissions in ["read", "write", "invite", "manage"]
           │
           ├─> 6. JWT TOKEN GENERATION (invitation_service.go)
           │   ├─ Call InvitationService.CreateToken(threadID, contractID, ownerID, role, permissions, expiry)
           │   ├─ Create JWT claims:
           │   │  {
           │   │    ThreadID: threadID,
           │   │    ContractID: contractID,
           │   │    InvitedBy: ownerID,
           │   │    Role: role,
           │   │    Permissions: permissions,
           │   │    ExpiresAt: time.Now() + expiry
           │   │  }
           │   └─> Sign with HMAC-SHA256 using secret key
           │       └─> Return JWT token string
           │
           └─ Return InvitePartyResponse{threadToken, role, permissions, expiresAt}

7. WEBSOCKET HANDLER (post-response)
   └─ Send InvitePartyResponse to client
```

**Database Impact**:
- **NONE** - Pure JWT token generation (stateless)
- Token is validated later during `joinThread`

---

### **CASE 5: `joinThread`**

**Handler Entry Point**: `/internal/handlers/thread.go:177-180`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:177-180)
   ├─ Unmarshal JoinThreadRequest from message
   ├─ Call handleJoinThread(session, req)
   │
   └─> 2. HANDLER HELPER (thread.go:248-281)
       ├─ Validate: req.Action == "joinThread"
       ├─ Call ThreadService.HandleJoinThread(req, ownerID, companyID)
       │
       └─> 3. THREAD SERVICE (thread.go:777-867)
           │
           ├─> MODE 1: TOKEN-BASED JOIN (invitation)
           │   ├─ IF req.ThreadToken != "":
           │   │  ├─ Call InvitationService.ValidateToken(threadToken)
           │   │  │  └─> Verify JWT signature, check expiry
           │   │  │      └─> Extract claims: {threadID, role, permissions, invitedBy}
           │   │  └─ Set: threadID, role, permissions[], invitedBy from token
           │   │
           │   └─> MODE 2: DIRECT JOIN (same company)
           │       ├─ ELSE IF req.ThreadID != "":
           │       │  ├─ Get thread object
           │       │  ├─ Validate: thread.CompanyID == companyID
           │       │  ├─ Validate: role is valid
           │       │  └─ Set: threadID, role, permissions=["read","write"], invitedBy=companyID
           │       │
           │       └─ ELSE: Error (either threadToken or threadID+role required)
           │
           ├─ Get thread object (if not already loaded)
           │
           ├─> 4. CONTRACT ROLE VALIDATION
           │   ├─ Get contract graph for thread
           │   ├─ IF contract has parties defined:
           │   │  ├─ Validate: role exists in contract.Parties
           │   │  └─ IF role not in parties → Error
           │   └─ Continue
           │
           ├─> 5. GRANT ACCESS (same as startThread creator access)
           │   ├─ Call GrantOrUpdateThreadAccess(threadID, ownerID, role, permissions, invitedBy, isCreator=false)
           │   │
           │   └─> 5a. SCOPE RESOLUTION
           │       ├─ Call ScopeResolver.ResolveScope(threadID, userID, role, isCreator=false)
           │       ├─ Logic: Check contract for role-based scope, default to "participant"
           │       │
           │       └─> 5b. VALKEY WRITE - Access Repository (atomic Lua script)
           │           ├─ Key: thread:{threadID}:access:{ownerID}
           │           ├─ Hash fields:
           │           │  {
           │           │    roles: JSON array [role],
           │           │    permissions: comma-separated string,
           │           │    grantedBy: invitedBy,
           │           │    grantedAt: timestamp,
           │           │    status: "active"
           │           │  }
           │           ├─ Command: HSET + SADD (via Lua script for atomicity)
           │           │  - HSET thread:{threadID}:access:{ownerID} {fields}
           │           │  - SADD thread:{threadID}:users {ownerID}
           │           │
           │           └─> 5c. ASYNC ACTIVITY LOG (goroutine)
           │               ├─ Write to Valkey LIST:
           │               │  ├─ Key: thread:{threadID}:activity
           │               │  ├─ Command: LPUSH thread:{threadID}:activity {json}
           │               │  ├─ Event: {type: "access_granted", threadID, userID, role, permissions, scope}
           │               │
           │               └─ Publish to NATS JetStream
           │                  ├─ Subject: "access.thread"
           │                  ├─ Payload: {threadID, userId, roles, permissions, grantedBy, scope}
           │                  └─> NATS → Archiver → PostgreSQL (thread_access table)
           │
           │               └─ Publish to NATS JetStream
           │                  ├─ Subject: "activity.log"
           │                  ├─ Payload: {type: "access_granted", thread_id, user_id, actor, role, permissions}
           │                  └─> NATS → Archiver → PostgreSQL (activity_log table)
           │
           └─ Return JoinThreadResponse{threadID, role, permissions, status: "success"}

6. WEBSOCKET HANDLER (post-response)
   ├─ Add threadID to session.threadIDs
   ├─ Subscribe to notifications via NotificationRouter
   │  └─> NotificationRouter.SubscribeToThread(clientID, threadID)
   │
   └─ Send JoinThreadResponse to client
```

**Database Impact**:

**Valkey (Immediate Writes)**:
1. `thread:{threadID}:access:{ownerID}` → Access hash
2. `thread:{threadID}:users` → Set of user IDs (add new user)
3. `thread:{threadID}:activity` → List of activity events (TTL: 7 days)

**NATS JetStream (Async Publish)**:
1. Subject: `access.thread` → Access granted event
2. Subject: `activity.log` → Access granted activity

**PostgreSQL (Via Archiver - Eventually Consistent)**:
1. `thread_access` table → INSERT access record
2. `activity_log` table → INSERT access_granted event

---

### **CASE 6: `closeConnection`**

**Handler Entry Point**: `/internal/handlers/thread.go:167-170`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:167-170)
   ├─ Call ThreadService.HandleClose(ownerID)
   │
   └─> 2. THREAD SERVICE (thread.go:870-880)
       ├─ IF ownerID != "":
       │  └─ Call ConnectionManager.Disconnect(ownerID)
       │     └─> Remove from in-memory map: delete(connections[ownerID])
       │
       └─ Return CloseConnectionResponse{status: "success"}

3. WEBSOCKET HANDLER (main loop cleanup - thread.go:95-106)
   ├─ Delete session from sessions map: sessions.Delete(ownerID)
   ├─ Call ThreadService.HandleClose(ownerID) (redundant, already called)
   │
   ├─> 4. UNSUBSCRIBE FROM NOTIFICATIONS
   │   ├─ Call unsubscribeFromNotifications(session)
   │   │  └─> FOR EACH threadID in session.threadIDs:
   │   │      └─ Call NotificationConsumer.Unsubscribe(threadID, ownerID)
   │   │         └─> NATS queue unsubscribe
   │   │
   │   └─ Close notification handler
   │
   ├─> 5. UNREGISTER FROM NOTIFICATION ROUTER
   │   └─ Call NotificationRouter.UnregisterClient(clientID)
   │      └─> Remove from clients map: delete(clients[clientID])
   │          └─> Remove from thread subscriptions
   │
   └─ Close WebSocket connection
```

**Database Impact**:
- **NONE** - Pure in-memory cleanup
- **In-Memory State Removed:**
  - ConnectionManager: `delete(connections[ownerID])`
  - WebSocketHandler.sessions: `delete(sessions[ownerID])`
  - NotificationRouter.clients: `delete(clients[clientID])`

---

### **CASE 7: `ack_notification`**

**Handler Entry Point**: `/internal/handlers/thread.go:182-185`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:182-185)
   ├─ Unmarshal NotificationACKMessage from message
   ├─ Call handleNotificationAck(session, ackMsg)
   │
   └─> 2. HANDLER HELPER (thread.go:508-546)
       ├─ Get WebSocketClient from NotificationRouter.clients[clientID]
       ├─ Call client.HandleClientAck(notificationID)
       │
       └─> 3. WEBSOCKET CLIENT (notification_router.go)
           ├─ Lock client.mu
           ├─ Get pending notification from client.pendingAcks[notificationID]
           ├─ IF not found → Error
           │
           ├─ Delete from pending: delete(client.pendingAcks[notificationID])
           ├─ Update metrics: totalAcked++, avgAckTime
           ├─ Unlock client.mu
           │
           └─ Log: "[WS-ACK] Client acknowledged notification {notificationID}"

4. WEBSOCKET HANDLER (post-response)
   └─ Send ACK response{notificationID, status: "success"}
```

**Database Impact**:
- **NONE** - Pure in-memory state management
- **In-Memory State Updated:**
  - WebSocketClient.pendingAcks: `delete(pendingAcks[notificationID])`
  - Metrics: `totalAcked++`, `avgAckTime` updated

---

### **CASE 8: `subscribe` (Step-Level Notification Subscription)**

**Handler Entry Point**: `/internal/handlers/thread.go:187-194`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:187-194)
   ├─ Unmarshal subscribe request {stepName, eventTypes[]}
   ├─ Call handleSubscribe(session, req)
   │
   └─> 2. HANDLER HELPER (thread.go:337-458)
       ├─ Validate: stepName is not empty
       ├─ Get WebSocketClient from NotificationRouter.clients[clientID]
       │
       ├─> 3. PARSE STEP NAME
       │   ├─ IF stepName contains "@":
       │   │  ├─ Split: "contract@stepName" → contractName, stepName
       │   │  └─ Validate format
       │   └─ ELSE: stepName only, contractName = ""
       │
       ├─> 4. VALIDATE EVENT TYPES
       │   ├─ Valid types: ["violation", "completed", "failed"]
       │   ├─ Deduplicate event types
       │   └─ IF invalid type → Error
       │
       ├─> 5. UPDATE CLIENT SUBSCRIPTIONS
       │   ├─ Lock client.mu
       │   ├─ IF subscription exists for stepName:
       │   │  └─ Merge event types (union)
       │   ├─ ELSE:
       │   │  └─ Create new subscription:
       │   │     {
       │   │       StepName: stepName,
       │   │       ContractName: contractName,
       │   │       EventTypes: eventTypes[]
       │   │     }
       │   ├─ Store: client.Subscriptions[stepName] = subscription
       │   └─ Unlock client.mu
       │
       └─ Return {action: "subscribe", status: "success"}

6. WEBSOCKET HANDLER (post-response)
   └─ Send subscribe response to client
```

**Database Impact**:
- **NONE** - Pure in-memory subscription management
- **In-Memory State Updated:**
  - WebSocketClient.Subscriptions: `map[stepName] → ClientSubscription{stepName, contractName, eventTypes[]}`

---

### **CASE 9: `unsubscribe`**

**Handler Entry Point**: `/internal/handlers/thread.go:196-202`

**Flow Pseudocode**:

```pseudo
1. WEBSOCKET HANDLER (thread.go:196-202)
   ├─ Unmarshal unsubscribe request {stepName}
   ├─ Call handleUnsubscribe(session, req)
   │
   └─> 2. HANDLER HELPER (thread.go:461-505)
       ├─ Validate: stepName is not empty
       ├─ Get WebSocketClient from NotificationRouter.clients[clientID]
       │
       ├─> 3. REMOVE SUBSCRIPTION
       │   ├─ Lock client.mu
       │   ├─ Delete: delete(client.Subscriptions[stepName])
       │   └─ Unlock client.mu
       │
       └─ Return {action: "unsubscribe", status: "success"}

4. WEBSOCKET HANDLER (post-response)
   └─ Send unsubscribe response to client
```

**Database Impact**:
- **NONE** - Pure in-memory subscription management
- **In-Memory State Updated:**
  - WebSocketClient.Subscriptions: `delete(subscriptions[stepName])`

---

## Summary: Data Flow Architecture

### **Hot Path (Valkey - Immediate)**
1. Thread metadata: `thread:{id}`
2. Access control: `thread:{id}:access:{userId}`, `thread:{id}:users`
3. Step state: `thread:{id}:steps:{name}:{idemp}`
4. Activity log (temporary): `thread:{id}:activity` (7-day TTL)
5. Thread refs: `thread:{id}:refs`

### **Warm Path (NATS JetStream - Async)**
1. `metadata.thread` → Thread/refs archival
2. `access.thread` → Access events
3. `activity.log` → All activity events (step_recorded, thread_created, access_granted, etc.)
4. `validations.thread` → Validation results
5. `notifications.{threadID}.{stepName}` → Real-time notifications to WebSocket clients

### **Cold Path (PostgreSQL - Eventually Consistent)**
1. `threads` table → Thread metadata
2. `thread_refs` table → External references
3. `thread_access` table → Access control records
4. `activity_log` table → Complete audit trail
5. `validation_results` table → Validation summaries
6. `step_events` table → Step execution history (if archiver writes to it)

### **In-Memory State (Session/Cache)**
1. ConnectionManager → Active connections
2. WebSocketHandler.sessions → WebSocket sessions
3. NotificationRouter.clients → Notification subscriptions
4. CacheManager → Thread/contract cache
5. WebSocketClient.pendingAcks → Pending notification ACKs

---

## Architecture Benefits

This architecture provides:
- **Low latency** via Valkey hot cache
- **Reliability** via NATS JetStream (at-least-once delivery)
- **Scalability** via async archival and partitioned streams
- **Real-time** via WebSocket notifications with subscription filtering
- **Auditability** via PostgreSQL cold storage with complete event history
- **Tamper detection** via cryptographic hash chains
- **Graceful degradation** when NATS is unavailable
