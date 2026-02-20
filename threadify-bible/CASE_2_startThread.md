# CASE 2: `startThread` - Thread Creation with Contract Validation

**Handler Entry Point**: `/internal/handlers/thread.go:140-160`

**Purpose**: Create a new thread with optional contract validation, initialize access control, and archive metadata to NATS for PostgreSQL persistence.

---

## Complete Flow Diagram

```
Client Request
    ↓
WebSocket Handler (unmarshal, dispatch)
    ↓
ThreadService.HandleStartThread
    ↓
├─→ Contract Validation (3-tier cache)
│   ├─→ Memory Cache (fastest)
│   ├─→ Valkey Cache (fast)
│   └─→ PostgreSQL (slowest, populates caches)
│
├─→ Thread Creation
│   └─→ Valkey Write: thread:{id}
│
├─→ Access Control (creator)
│   ├─→ Scope Resolution
│   ├─→ Valkey Write: thread:{id}:access:{userId}
│   └─→ NATS Publish: access.thread
│
└─→ Async Archival (goroutine)
    ├─→ NATS Publish: metadata.thread (thread)
    ├─→ NATS Publish: metadata.thread (refs)
    └─→ NATS Publish: activity.log (thread_created)
```

---

## Detailed Step-by-Step Flow

### **Step 1: WebSocket Handler Receives Request**

**Location**: `/internal/handlers/thread.go:140-160`

```
Client sends WebSocket message:
{
  "action": "startThread",
  "contractName": "order_flow",      // Optional
  "role": "merchant",                 // Required if contractName provided
  "refs": {                           // Optional
    "orderId": "12345",
    "customerId": "cust-789"
  }
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "startThread"
├─ Unmarshal into StartThreadRequest struct:
│  {
│    Action: "startThread",
│    ContractName: "order_flow",
│    Role: "merchant",
│    Refs: map[string]string{"orderId": "12345", "customerId": "cust-789"}
│  }
│
└─ Call ThreadService.HandleStartThread(req, session.ownerID, session.companyID)
   └─> ownerID and companyID from session (set during connect)
```

---

### **Step 2: ThreadService - Initial Validations**

**Location**: `/internal/service/thread.go:190-220`

```
ThreadService.HandleStartThread(req, ownerID, companyID):

├─ VALIDATION 1: Check ownerID is connected
│  ├─ Call ConnectionManager.IsConnected(ownerID)
│  │  └─> Check if connections[ownerID] exists in sync.Map
│  │
│  └─ IF not connected:
│     └─> Return StartThreadResponse{
│           status: "error",
│           message: "Not authenticated. Please connect first."
│         }

├─ VALIDATION 2: Contract name requires role
│  ├─ IF req.ContractName != "" && req.Role == "":
│  │  └─> Return error "Role is required when contract name is provided"
│  │
│  └─ Continue

└─ Proceed to contract validation (if contractName provided)
```

---

### **Step 3: Contract Validation (Three-Tier Cache)**

**Location**: `/internal/service/thread.go:222-270`

**Only executed if `req.ContractName != ""`**

```
┌─────────────────────────────────────────────────────────────────┐
│ 3.1: Parse Contract Identifier                                  │
└─────────────────────────────────────────────────────────────────┘

Call parseContractIdentifier(req.ContractName):

├─ Input examples:
│  ├─ "order_flow" → name="order_flow", version=0 (latest)
│  └─ "order_flow:2" → name="order_flow", version=2
│
├─ Split by ":" delimiter
│  ├─ IF no ":" → version = 0 (means "get latest")
│  └─ IF has ":" → parse version number
│
└─ Return: (parsedContractName, contractVersion)


┌─────────────────────────────────────────────────────────────────┐
│ 3.2: Load Contract Graph (Three-Tier Cache)                     │
└─────────────────────────────────────────────────────────────────┘

Call ContractValidator.LoadContractGraphIntoCache(name, version, companyID):

┌─────────────────────────────────────────────────────────────────┐
│ TIER 1: In-Memory Cache (Fastest - ~1μs)                        │
└─────────────────────────────────────────────────────────────────┘

├─ Cache key: "graph:{companyID}:{contractName}:{version}"
│  Example: "graph:company-456:order_flow:2"
│
├─ Check: cacheService.Get(key)
│  └─> Lookup in sync.Map (thread-safe)
│
├─ IF found:
│  ├─ Return cached ContractGraph struct
│  └─> FASTEST PATH - No network I/O
│
└─ IF not found → Continue to Tier 2


┌─────────────────────────────────────────────────────────────────┐
│ TIER 2: Valkey Cache (Fast - ~1-5ms)                            │
└─────────────────────────────────────────────────────────────────┘

├─ Cache key: "graph:{companyID}:{contractName}:{version}"
│
├─ Command: GET graph:company-456:order_flow:2
│
├─ IF found:
│  ├─ Deserialize JSON to ContractGraph struct:
│  │  {
│  │    Name: "order_flow",
│  │    Version: 2,
│  │    Parties: ["merchant", "customer", "logistics"],
│  │    Graph: {
│  │      EntryPoints: ["order_placed"],
│  │      TerminalSteps: ["order_delivered", "order_cancelled"],
│  │      Nodes: map[string]GraphNode{...}
│  │    },
│  │    MaxDuration: 86400
│  │  }
│  │
│  ├─ Store in memory cache (Tier 1)
│  │  └─> cacheService.Set(key, graph)
│  │
│  └─ Return graph
│
└─ IF not found → Continue to Tier 3


┌─────────────────────────────────────────────────────────────────┐
│ TIER 3: PostgreSQL (Slowest - ~10-50ms)                         │
└─────────────────────────────────────────────────────────────────┘

├─ Query logic:
│  │
│  ├─ IF version == 0 (latest):
│  │  └─ Query:
│  │     SELECT * FROM contracts
│  │     WHERE name=$1 AND company_id=$2
│  │     ORDER BY version DESC LIMIT 1
│  │
│  └─ ELSE (specific version):
│     └─ Query:
│        SELECT * FROM contracts
│        WHERE name=$1 AND company_id=$2 AND version=$3
│
├─ Result row:
│  {
│    id: "contract-uuid",
│    name: "order_flow",
│    version: 2,
│    company_id: "company-456",
│    definition: "name: order_flow\nversion: 2\n...",  // YAML
│    created_at: "2026-01-01T00:00:00Z"
│  }
│
├─ Parse YAML definition:
│  ```yaml
│  name: order_flow
│  version: 2
│  parties: [merchant, customer, logistics]
│  entry_points: [order_placed]
│  terminal_steps: [order_delivered, order_cancelled]
│  max_duration: 86400
│  steps:
│    order_placed:
│      owner: merchant
│      next_steps: [payment_processed, order_cancelled]
│      required_fields: [orderId, amount]
│      optional_fields: [notes]
│      timeout: 300
│    payment_processed:
│      owner: merchant
│      next_steps: [order_shipped, order_cancelled]
│      required_fields: [paymentId]
│      timeout: 600
│    order_shipped:
│      owner: logistics
│      next_steps: [order_delivered]
│      required_fields: [trackingNumber]
│    order_delivered:
│      owner: logistics
│      is_terminal: true
│    order_cancelled:
│      owner: merchant
│      is_terminal: true
│  ```
│
├─ Build ContractGraph data structure:
│  graph = ContractGraph{
│    Name: "order_flow",
│    Version: 2,
│    Parties: ["merchant", "customer", "logistics"],
│    Graph: {
│      EntryPoints: ["order_placed"],
│      TerminalSteps: ["order_delivered", "order_cancelled"],
│      Nodes: map[string]GraphNode{
│        "order_placed": {
│          Name: "order_placed",
│          Owner: "merchant",
│          NextSteps: ["payment_processed", "order_cancelled"],
│          RequiredFields: ["orderId", "amount"],
│          OptionalFields: ["notes"],
│          Timeout: 300,
│          IsTerminal: false
│        },
│        "payment_processed": {
│          Name: "payment_processed",
│          Owner: "merchant",
│          NextSteps: ["order_shipped", "order_cancelled"],
│          RequiredFields: ["paymentId"],
│          Timeout: 600,
│          IsTerminal: false
│        },
│        "order_shipped": {
│          Name: "order_shipped",
│          Owner: "logistics",
│          NextSteps: ["order_delivered"],
│          RequiredFields: ["trackingNumber"],
│          IsTerminal: false
│        },
│        "order_delivered": {
│          Name: "order_delivered",
│          Owner: "logistics",
│          NextSteps: [],
│          IsTerminal: true
│        },
│        "order_cancelled": {
│          Name: "order_cancelled",
│          Owner: "merchant",
│          NextSteps: [],
│          IsTerminal: true
│        }
│      }
│    },
│    MaxDuration: 86400
│  }
│
├─ Cache in Valkey (Tier 2):
│  ├─ Serialize graph to JSON
│  ├─ Command: SET graph:company-456:order_flow:2 {json} EX {contractTTLSeconds}
│  │  └─> TTL: Configurable (e.g., 3600 seconds = 1 hour)
│  └─> Future requests hit Tier 2 cache
│
├─ Cache in memory (Tier 1):
│  └─> cacheService.Set(key, graph)
│
└─ Return graph with actual version number
   └─> actualVersion = 2 (even if version=0 was requested)
```

---

### **Step 4: Thread Object Creation**

**Location**: `/internal/service/thread.go:239-262`

```
├─ Store actualVersion from contract validation
│  └─> If version=0 was requested, actualVersion = latest version from DB
│
├─ Generate unique thread ID:
│  threadID = uuid.New().String()
│  └─> Example: "550e8400-e29b-41d4-a716-446655440000"
│
├─ Create contract version pointer (lines 247-251):
│  var contractVersionPtr *int
│  if parsedContractName != "" && contractVersion > 0 {
│    contractVersionPtr = &contractVersion  // Store actual loaded version
│  }
│
└─ Create Thread object (lines 253-262):
   thread = &models.Thread{
     ID: "550e8400-e29b-41d4-a716-446655440000",
     ContractID: &contractUUID,           // Pointer to contract UUID (for referential integrity)
     ContractName: "order_flow",          // Contract name for display/filtering
     ContractVersion: &actualVersion,     // Pointer to 2 (actual loaded version)
     OwnerID: "user-123",
     CompanyID: "company-456",
     Status: "active",
     StartedAt: time.Now(),               // "2026-01-12T12:00:00Z"
     CompletedAt: nil,                    // NULL initially
     LastHash: "",                         // Empty initially, updated on first step
     Refs: map[string]string{
       "orderId": "12345",
       "customerId": "cust-789"
     }
   }

   ⚠️ IMPORTANT: ContractVersion must be set to the actualVersion returned from
      LoadContractGraphIntoCache() to ensure threads lock to the correct version.
      If version=0 was requested, actualVersion will be the latest version number.
```

---

### **Step 5: Valkey Write - Thread Repository**

**Location**: `/internal/repository/valkey/thread.go`

```
Call ThreadRepository.Save(ctx, thread):

├─ Serialize thread to JSON:
│  threadJSON = {
│    "id": "550e8400-e29b-41d4-a716-446655440000",
│    "contract_id": "order_flow",
│    "contract_name": "order_flow",
│    "contract_version": 2,
│    "owner_id": "user-123",
│    "company_id": "company-456",
│    "status": "active",
│    "started_at": "2026-01-12T12:00:00Z",
│    "completed_at": null,
│    "last_hash": "",
│    "refs": {
│      "orderId": "12345",
│      "customerId": "cust-789"
│    }
│  }
│
├─ Valkey key: thread:{threadID}
│  Example: "thread:550e8400-e29b-41d4-a716-446655440000"
│
├─ Command: SET thread:550e8400-e29b-41d4-a716-446655440000 {threadJSON} EX {threadTTLSeconds}
│  ├─> Value: JSON string (~500 bytes - 2KB)
│  └─> TTL: 86400 seconds (24 hours, configurable)
│
├─ Log: "Successfully saved thread 550e8400-e29b-41d4-a716-446655440000 to Valkey"
│
└─ Return nil (success)
```

**In-Memory Cache Update**:

```
CacheManager.SetThread(threadID, thread):
├─ Store in sync.Map for fast GetThread() calls
└─> Avoids Valkey lookup on immediate subsequent requests
```

---

### **Step 6: Access Control - Grant Creator Access**

**Location**: `/internal/service/thread.go:292-330`

```
┌─────────────────────────────────────────────────────────────────┐
│ 6.1: Determine Creator Role and Permissions                     │
└─────────────────────────────────────────────────────────────────┘

├─ Determine creator role:
│  ├─ IF req.Role != "":
│  │  └─> creatorRole = req.Role  // "merchant"
│  └─ ELSE:
│     └─> creatorRole = "owner"   // Default for threads without contract
│
├─ Set creator permissions:
│  creatorPermissions = ["read", "write", "invite", "manage"]
│  └─> Creator gets full permissions
│
└─ Call GrantOrUpdateThreadAccess(
     threadID,
     ownerID,
     creatorRole,
     creatorPermissions,
     invitedBy="self",
     isCreator=true,
     explicitScope=nil
   )


┌─────────────────────────────────────────────────────────────────┐
│ 6.2: Scope Resolution                                           │
└─────────────────────────────────────────────────────────────────┘

Call ScopeResolver.ResolveScope(threadID, userID, role, isCreator=true, explicitScope=nil):

├─ Resolution logic:
│  │
│  ├─ IF isCreator == true:
│  │  └─> scope = "owner"
│  │      └─> Creator always gets "owner" scope for notifications
│  │
│  ├─ ELSE IF explicitScope != nil:
│  │  └─> scope = *explicitScope
│  │      └─> Use explicitly provided scope
│  │
│  ├─ ELSE IF contract has scope mapping for role:
│  │  └─> scope = contract.scopeForRole[role]
│  │      └─> Use contract-defined scope
│  │
│  └─ ELSE:
│     └─> scope = "participant"
│         └─> Default scope
│
└─ Return scope = "owner"


┌─────────────────────────────────────────────────────────────────┐
│ 6.3: Valkey Write - Access Repository (Atomic Lua Script)       │
└─────────────────────────────────────────────────────────────────┘

Call AccessRepository.GrantOrUpdateAccess(ctx, threadID, userID, role, permissions, invitedBy, luaScripts):

ATOMIC LUA SCRIPT EXECUTION:

Script inputs:
├─ KEYS[1]: thread:{threadID}:access:{userID}
├─ KEYS[2]: thread:{threadID}:users
├─ ARGV[1]: role (e.g., "merchant")
├─ ARGV[2]: permissions (e.g., "read,write,invite,manage")
├─ ARGV[3]: grantedBy (e.g., "self")
├─ ARGV[4]: grantedAt (timestamp)
└─ ARGV[5]: userID

Lua script logic:
```lua
-- Get existing access (if any)
local existing = redis.call('HGETALL', KEYS[1])
local existingRoles = {}

-- Parse existing roles if present
if existing and existing.roles then
  existingRoles = cjson.decode(existing.roles)
end

-- Add new role to roles array
table.insert(existingRoles, ARGV[1])

-- Remove duplicates
local uniqueRoles = {}
local seen = {}
for _, role in ipairs(existingRoles) do
  if not seen[role] then
    table.insert(uniqueRoles, role)
    seen[role] = true
  end
end

-- Set access hash
redis.call('HSET', KEYS[1],
  'roles', cjson.encode(uniqueRoles),
  'permissions', ARGV[2],
  'grantedBy', ARGV[3],
  'grantedAt', ARGV[4],
  'status', 'active'
)

-- Add user to thread's user set
redis.call('SADD', KEYS[2], ARGV[5])

-- Return access object
return cjson.encode({
  roles = uniqueRoles,
  permissions = ARGV[2],
  grantedBy = ARGV[3],
  grantedAt = ARGV[4],
  status = 'active'
})
```

Result in Valkey:

Key: thread:550e8400-e29b-41d4-a716-446655440000:access:user-123
Type: Hash
Fields:
{
  "roles": '["merchant"]',
  "permissions": "read,write,invite,manage",
  "grantedBy": "self",
  "grantedAt": "2026-01-12T12:00:00Z",
  "status": "active"
}

Key: thread:550e8400-e29b-41d4-a716-446655440000:users
Type: Set
Members: {"user-123"}


┌─────────────────────────────────────────────────────────────────┐
│ 6.4: Async Activity Log (Goroutine)                             │
└─────────────────────────────────────────────────────────────────┘

Spawn goroutine:
go func() {
  defer func() {
    if r := recover(); r != nil {
      log.Printf("❌ PANIC in access activity log: %v", r)
    }
  }()

  ├─ Get serviceName from ConnectionManager:
  │  ├─ Call ConnectionManager.GetClient(ownerID)
  │  └─> Returns Client{ServiceName: "merchant-service"}
  │
  ├─ Build activity event:
  │  activityValues = {
  │    type: "access_granted",
  │    thread_id: "550e8400-e29b-41d4-a716-446655440000",
  │    user_id: "user-123",
  │    actor: "user-123",
  │    actor_service: "merchant-service",
  │    role: "merchant",
  │    permissions: "read,write,invite,manage",
  │    granted_by: "self",
  │    scope: "owner",
  │    timestamp: "2026-01-12T12:00:00Z"
  │  }
  │
  ├─ VALKEY WRITE - Activity List:
  │  ├─ Key: thread:550e8400-e29b-41d4-a716-446655440000:activity
  │  ├─ Serialize event to JSON
  │  ├─ Command: LPUSH thread:550e8400-e29b-41d4-a716-446655440000:activity {json}
  │  └─ Command: EXPIRE thread:550e8400-e29b-41d4-a716-446655440000:activity 604800
  │     └─> TTL: 604800 seconds (7 days)
  │
  └─ NATS PUBLISH - Access Event:
     ├─ Call: natsArchivalPublisher.PublishThreadAccess(ctx, activityValues)
     │  └─> Timeout: 5 seconds
     │
     ├─ Subject: "access.thread"
     │
     ├─ Payload (JSON):
     │  {
     │    "threadId": "550e8400-e29b-41d4-a716-446655440000",
     │    "userId": "user-123",
     │    "roles": "[\"merchant\"]",
     │    "permissions": "read,write,invite,manage",
     │    "grantedBy": "self",
     │    "grantedAt": "2026-01-12T12:00:00Z",
     │    "status": "active",
     │    "scope": "owner"
     │  }
     │
     └─> NATS JetStream:
         ├─ Publish to stream "thread_access"
         ├─ Message persisted to disk
         ├─ Consumer group: "archivers"
         └─> Archiver will consume and write to PostgreSQL thread_access table
}()
```

---

### **Step 7: Async Thread Metadata Archival**

**Location**: `/internal/service/thread.go:332-394`

```
Spawn goroutine:
go func() {
  defer func() {
    if r := recover(); r != nil {
      log.Printf("❌ PANIC in thread metadata goroutine: %v", r)
    }
  }()

  ┌───────────────────────────────────────────────────────────────┐
  │ 7.1: NATS PUBLISH - Thread Metadata                           │
  └───────────────────────────────────────────────────────────────┘

  ├─ Build metadata payload:
  │  streamValues = {
  │    threadId: "550e8400-e29b-41d4-a716-446655440000",
  │    ownerId: "user-123",
  │    companyId: "company-456",
  │    contractId: "order_flow",
  │    contractName: "order_flow",
  │    contractVersion: "2",
  │    error: "",
  │    startedAt: "2026-01-12T12:00:00Z",
  │    maxlen: "~",
  │    limit: 100000
  │  }
  │
  ├─ Call: natsArchivalPublisher.PublishThreadMetadata(ctx, streamValues)
  │  ├─> Context with 5-second timeout
  │  └─> Graceful degradation if NATS unavailable
  │
  ├─ Subject: "metadata.thread"
  │
  ├─ NATS JetStream processing:
  │  ├─ Add timestamp: streamValues["timestamp"] = time.Now()
  │  ├─ Marshal to JSON
  │  ├─ Publish to stream "thread_metadata"
  │  ├─ Message persisted to disk
  │  └─> Consumer group: "archivers"
  │
  └─> Archiver consumes → PostgreSQL threads table:
      INSERT INTO threads (
        id, company_id, contract_id, contract_name, contract_version,
        owner_id, error, created_at, updated_at
      ) VALUES (
        '550e8400-e29b-41d4-a716-446655440000',
        'company-456',
        'order_flow',
        'order_flow',
        2,
        'user-123',
        '',
        '2026-01-12T12:00:00Z',
        '2026-01-12T12:00:00Z'
      )


  ┌───────────────────────────────────────────────────────────────┐
  │ 7.2: NATS PUBLISH - Thread Refs (if provided)                 │
  └───────────────────────────────────────────────────────────────┘

  IF thread.Refs != nil && len(thread.Refs) > 0:

    FOR EACH key, value in thread.Refs:
      │
      ├─ Build ref event:
      │  refEvent = {
      │    threadId: "550e8400-e29b-41d4-a716-446655440000",
      │    refKey: "orderId",
      │    refValue: "12345",
      │    action: "ref_added"
      │  }
      │
      ├─ Call: natsArchivalPublisher.PublishThreadMetadata(ctx, refEvent)
      │  └─> Timeout: 5 seconds
      │
      ├─ Subject: "metadata.thread"
      │
      ├─ NATS JetStream processing:
      │  ├─ Publish to stream "thread_metadata"
      │  └─> Consumer group: "archivers"
      │
      └─> Archiver consumes → PostgreSQL thread_refs table:
          INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
          VALUES (
            '550e8400-e29b-41d4-a716-446655440000',
            'orderId',
            '12345',
            NOW(),
            NOW()
          )
          ON CONFLICT (thread_id, ref_key) DO UPDATE SET
            ref_value = EXCLUDED.ref_value,
            updated_at = NOW()

    Log: "✅ SUCCESS: Published 2 refs to NATS for thread 550e8400-e29b-41d4-a716-446655440000"


  ┌───────────────────────────────────────────────────────────────┐
  │ 7.3: VALKEY WRITE - Activity List (thread_created)            │
  └───────────────────────────────────────────────────────────────┘

  ├─ Get serviceName from ConnectionManager
  │
  ├─ Build activity event:
  │  activityValues = {
  │    type: "thread_created",
  │    thread_id: "550e8400-e29b-41d4-a716-446655440000",
  │    owner_id: "user-123",
  │    actor: "user-123",
  │    actor_service: "merchant-service",
  │    contract_id: "order_flow",
  │    contract_name: "order_flow",
  │    contract_version: "2",
  │    role: "merchant",
  │    timestamp: "2026-01-12T12:00:00Z"
  │  }
  │
  ├─ Serialize to JSON
  │
  ├─ Command: LPUSH thread:550e8400-e29b-41d4-a716-446655440000:activity {json}
  │
  └─ Command: EXPIRE thread:550e8400-e29b-41d4-a716-446655440000:activity 604800


  ┌───────────────────────────────────────────────────────────────┐
  │ 7.4: NATS PUBLISH - Activity Log                              │
  └───────────────────────────────────────────────────────────────┘

  ├─ Call: natsArchivalPublisher.PublishActivityLog(ctx, activityValues)
  │  └─> Timeout: 5 seconds
  │
  ├─ Subject: "activity.log"
  │
  ├─ Payload: {same as 7.3 activityValues}
  │
  ├─ NATS JetStream processing:
  │  ├─ Publish to stream "activity_log"
  │  └─> Consumer group: "archivers"
  │
  └─> Archiver consumes → PostgreSQL thread_activities table:
      INSERT INTO thread_activities (
        thread_id, activity_type, step_id, actor, actor_service, payload, recorded_at
      ) VALUES (
        '550e8400-e29b-41d4-a716-446655440000',
        'thread_created',
        NULL,
        'user-123',
        'merchant-service',
        '{"contract_id":"order_flow","contract_name":"order_flow","contract_version":"2","role":"merchant"}'::jsonb,
        '2026-01-12T12:00:00Z'
      )
}()
```

---

### **Step 8: WebSocket Handler - Post-Response Actions**

**Location**: `/internal/handlers/thread.go:146-160`

```
IF response.Status == "success":

  ├─ Add threadID to session (thread-safe):
  │  session.mu.Lock()
  │  IF !utils.Contains(session.threadIDs, response.ThreadID):
  │    session.threadIDs = append(session.threadIDs, response.ThreadID)
  │  session.mu.Unlock()
  │
  ├─ Subscribe to notifications (old NATS consumer):
  │  └─ Call subscribeToNotifications(session, threadID, "owner")
  │     ├─ Create WebSocketNotificationHandler for session
  │     ├─ NATS queue subscription: "notifications.{threadID}"
  │     └─> Messages delivered to this specific session
  │
  └─ Subscribe via NotificationRouter (new router):
     └─ Call notificationRouter.SubscribeToThread(clientID, threadID)
        ├─ Get client from router.clients[clientID]
        ├─ client.mu.Lock()
        ├─ client.ThreadIDs[threadID] = true
        ├─ client.mu.Unlock()
        └─> Client now receives notifications for this thread

Send response to client:
conn.WriteJSON(StartThreadResponse{
  action: "startThread",
  status: "success",
  message: "Thread started successfully",
  threadId: "550e8400-e29b-41d4-a716-446655440000"
})
```

---

## Database Impact Summary

### **Valkey (Immediate Writes - Synchronous)**

1. **`thread:{threadID}`** → Thread JSON
   - **Key**: `thread:550e8400-e29b-41d4-a716-446655440000`
   - **Value**: JSON (~500 bytes - 2KB)
   - **TTL**: 86400 seconds (24 hours)
   - **Purpose**: Hot cache for active threads

2. **`thread:{threadID}:access:{ownerID}`** → Access hash
   - **Key**: `thread:550e8400-e29b-41d4-a716-446655440000:access:user-123`
   - **Type**: Hash
   - **Fields**: roles, permissions, grantedBy, grantedAt, status
   - **No TTL**: Persists with thread
   - **Purpose**: Fast permission checks

3. **`thread:{threadID}:users`** → Set of user IDs
   - **Key**: `thread:550e8400-e29b-41d4-a716-446655440000:users`
   - **Type**: Set
   - **Members**: {"user-123"}
   - **No TTL**
   - **Purpose**: Quick user lookup for thread

4. **`thread:{threadID}:activity`** → List of activity events
   - **Key**: `thread:550e8400-e29b-41d4-a716-446655440000:activity`
   - **Type**: List
   - **TTL**: 604800 seconds (7 days)
   - **Purpose**: Recent activity history for debugging

### **NATS JetStream (Async Publish - Fire & Forget)**

1. **Stream: `thread_metadata`** → Thread metadata event
   - **Subject**: "metadata.thread"
   - **Retention**: Until archived to PostgreSQL
   - **Consumer**: Archiver service (consumer group: "archivers")

2. **Stream: `thread_metadata`** → Ref events (one per ref)
   - **Subject**: "metadata.thread"
   - **Count**: N events for N refs (2 in this example)
   - **Consumer**: Archiver service

3. **Stream: `thread_access`** → Access granted event
   - **Subject**: "access.thread"
   - **Consumer**: Archiver service

4. **Stream: `activity_log`** → Thread created event
   - **Subject**: "activity.log"
   - **Consumer**: Archiver service

### **PostgreSQL (Eventually Consistent via Archiver)**

1. **`threads` table** → INSERT
   ```sql
   INSERT INTO threads (
     id, company_id, contract_id, contract_name, contract_version,
     owner_id, error, created_at, updated_at
   ) VALUES (
     '550e8400-e29b-41d4-a716-446655440000',
     'company-456',
     'order_flow',
     'order_flow',
     2,
     'user-123',
     '',
     '2026-01-12T12:00:00Z',
     '2026-01-12T12:00:00Z'
   )
   ```

2. **`thread_refs` table** → INSERT multiple rows
   ```sql
   INSERT INTO thread_refs (thread_id, ref_key, ref_value, created_at, updated_at)
   VALUES 
     ('550e8400-e29b-41d4-a716-446655440000', 'orderId', '12345', NOW(), NOW()),
     ('550e8400-e29b-41d4-a716-446655440000', 'customerId', 'cust-789', NOW(), NOW())
   ON CONFLICT (thread_id, ref_key) DO UPDATE SET
     ref_value = EXCLUDED.ref_value,
     updated_at = NOW()
   ```

3. **`thread_access` table** → INSERT
   ```sql
   INSERT INTO thread_access (
     thread_id, user_id, roles, permissions, granted_by, granted_at, status
   ) VALUES (
     '550e8400-e29b-41d4-a716-446655440000',
     'user-123',
     '["merchant"]'::jsonb,
     'read,write,invite,manage',
     'self',
     '2026-01-12T12:00:00Z',
     'active'
   )
   ```

4. **`thread_activities` table** → INSERT
   ```sql
   INSERT INTO thread_activities (
     thread_id, activity_type, step_id, actor, actor_service, payload, recorded_at
   ) VALUES (
     '550e8400-e29b-41d4-a716-446655440000',
     'thread_created',
     NULL,
     'user-123',
     'merchant-service',
     '{"contract_id":"order_flow","contract_name":"order_flow","contract_version":"2","role":"merchant"}'::jsonb,
     '2026-01-12T12:00:00Z'
   )
   ```

---

## Performance Characteristics

### **Latency Breakdown**

**Total Response Time**: ~10-50ms (depending on cache hits)

1. **Contract Validation**:
   - Memory cache hit: ~1μs
   - Valkey cache hit: ~1-5ms
   - PostgreSQL miss: ~10-50ms (only first time)

2. **Valkey Writes**: ~1-3ms total
   - Thread write: ~1ms
   - Access write (Lua script): ~1-2ms

3. **NATS Publishes**: <1ms (async, non-blocking)

4. **Total**: 2-8ms (cached contract) or 12-58ms (uncached contract)

### **Scalability**

- **Concurrent thread creation**: Limited by Valkey write throughput (~50,000 ops/sec)
- **Contract cache**: Reduces PostgreSQL load by 99%+
- **Async archival**: Decouples response time from PostgreSQL performance

---

## Error Handling

### **Validation Errors**

1. **Not authenticated**: Return error immediately
2. **Contract not found**: Return error with contract name
3. **Invalid role**: Return error with valid roles from contract
4. **Valkey write failure**: Return error, no partial state

### **Archival Failures**

- **NATS unavailable**: Log warning, continue (graceful degradation)
- **Timeout**: Log error, message will be retried by NATS
- **PostgreSQL down**: Archiver retries with exponential backoff

---

*This completes the detailed flow for CASE 2: startThread*
