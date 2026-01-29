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

## **CASE 1: `connect` - Authentication & Session Setup**

### **Handler Entry Point**

```
Location: /internal/handlers/thread.go:115-138
```

### **Complete Flow**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. WEBSOCKET HANDLER (thread.go:115-138)                                    │
└─────────────────────────────────────────────────────────────────────────────┘
   │
   ├─ Client sends WebSocket message: 
   │  {
   │    action: "connect",
   │    apiKey: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
   │    serviceName: "merchant-service",
   │    subscribedEvents: []
   │  }
   │
   ├─ Handler receives message via conn.ReadJSON(&msg)
   ├─ Extract action field: action = msg["action"].(string)
   ├─ Unmarshal into ConnectRequest struct
   ├─ Call ThreadService.HandleConnect(req)
   │
   └─> ┌────────────────────────────────────────────────────────────────────┐
       │ 2. THREAD SERVICE (thread.go:152-188)                              │
       └────────────────────────────────────────────────────────────────────┘
       │
       ├─ VALIDATION: Check apiKey is not empty
       │  └─ IF req.ApiKey == "" → Return ConnectResponse{status: "error", message: "API key is required"}
       │
       ├─> ┌──────────────────────────────────────────────────────────────┐
       │   │ 3. AUTH SERVICE - API Key Validation                         │
       │   └──────────────────────────────────────────────────────────────┘
       │   │
       │   ├─ Call AuthService.ValidateApiKey(apiKey)
       │   │
       │   ├─ JWT Token Decoding:
       │   │  ├─ Split token into parts: header.payload.signature
       │   │  ├─ Base64 decode payload
       │   │  ├─ Verify signature using HMAC-SHA256 with secret key
       │   │  └─ IF signature invalid → Return error "Invalid API key"
       │   │
       │   ├─ Check expiration:
       │   │  ├─ Extract exp claim from payload
       │   │  ├─ Compare with current time
       │   │  └─ IF expired → Return error "API key expired"
       │   │
       │   ├─ Extract claims from payload:
       │   │  {
       │   │    ownerID: "user-123",
       │   │    companyID: "company-456",
       │   │    serviceName: "merchant-service",
       │   │    iss: "threadify",
       │   │    aud: "threadify-api",
       │   │    exp: 1736683200,
       │   │    iat: 1736596800
       │   │  }
       │   │
       │   └─ Return UserInfo{
       │        OwnerID: "user-123",
       │        CompanyID: "company-456",
       │        ServiceName: "merchant-service"
       │      }
       │
       ├─> ┌──────────────────────────────────────────────────────────────┐
       │   │ 4. CONNECTION MANAGER - In-Memory State                      │
       │   └──────────────────────────────────────────────────────────────┘
       │   │
       │   ├─ Call ConnectionManager.ConnectWithOwnerAndCompany(ownerID, apiKey, serviceName, companyID)
       │   │
       │   ├─ Store in-memory map (sync.Map):
       │   │  connections[ownerID] = Client{
       │   │    ApiKey: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
       │   │    ServiceName: "merchant-service",
       │   │    CompanyID: "company-456",
       │   │    ConnectedAt: time.Now()
       │   │  }
       │   │
       │   └─ No database write - pure in-memory operation
       │      └─> This allows fast IsConnected() checks
       │
       └─ Return ConnectResponse{
            Action: "connect",
            Status: "success",
            Message: "Connected successfully",
            OwnerID: "user-123",
            CompanyID: "company-456",
            SubscribedEvents: []
          }

┌─────────────────────────────────────────────────────────────────────────────┐
│ 5. WEBSOCKET HANDLER - Post-Response Actions (thread.go:119-137)            │
└─────────────────────────────────────────────────────────────────────────────┘
   │
   ├─ IF response.Status == "success":
   │  │
   │  ├─ Update session object (thread-safe with mutex):
   │  │  session.mu.Lock()
   │  │  session.ownerID = response.OwnerID      // "user-123"
   │  │  session.companyID = response.CompanyID  // "company-456"
   │  │  session.mu.Unlock()
   │  │
   │  ├─ Store session in handler's sessions map (sync.Map):
   │  │  sessions.Store(response.OwnerID, session)
   │  │  └─> Key: "user-123"
   │  │      Value: &Session{
   │  │        conn: *websocket.Conn,
   │  │        clientID: "550e8400-e29b-41d4-a716-446655440000",
   │  │        ownerID: "user-123",
   │  │        companyID: "company-456",
   │  │        threadIDs: []string{},
   │  │        notificationHandler: nil
   │  │      }
   │  │
   │  └─> ┌──────────────────────────────────────────────────────────────┐
   │      │ 6. NOTIFICATION ROUTER - Client Registration                 │
   │      └──────────────────────────────────────────────────────────────┘
   │      │
   │      ├─ Create WebSocketClient object:
   │      │  wsClient = &WebSocketClient{
   │      │    ID: session.clientID,              // UUID
   │      │    Conn: session.conn,                // WebSocket connection
   │      │    OwnerID: session.ownerID,          // "user-123"
   │      │    ThreadIDs: map[string]bool{},      // Empty initially
   │      │    pendingAcks: map[string]*PendingNotification{},
   │      │    Subscriptions: map[string]*ClientSubscription{},
   │      │    mu: sync.RWMutex{}
   │      │  }
   │      │
   │      ├─ Register with NotificationRouter:
   │      │  notificationRouter.RegisterClient(wsClient)
   │      │  │
   │      │  └─> Store in router's clients map:
   │      │      notificationRouter.mu.Lock()
   │      │      notificationRouter.clients[clientID] = wsClient
   │      │      notificationRouter.mu.Unlock()
   │      │
   │      └─ Client now ready to receive notifications
   │         └─> Can subscribe to threads and steps
   │
   └─ Send ConnectResponse to client via WebSocket:
      conn.WriteJSON(response)
```

### **Database Impact**

**NONE** - Completely in-memory operation

**In-Memory State Created:**

1. **ConnectionManager.connections[ownerID]** → Client metadata
   - Purpose: Fast authentication checks
   - Lifetime: Until disconnect or timeout

2. **WebSocketHandler.sessions[ownerID]** → Session object
   - Purpose: WebSocket connection management
   - Lifetime: Until WebSocket closes

3. **NotificationRouter.clients[clientID]** → WebSocket client for notifications
   - Purpose: Real-time notification delivery
   - Lifetime: Until client unregisters

---

## **CASE 2: `startThread` - Thread Creation with Contract Validation**

### **Handler Entry Point**

```
Location: /internal/handlers/thread.go:140-160
```

### **Complete Flow**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ 1. WEBSOCKET HANDLER (thread.go:140-160)                                    │
└─────────────────────────────────────────────────────────────────────────────┘
   │
   ├─ Client sends WebSocket message:
   │  {
   │    action: "startThread",
   │    contractName: "order_flow",
   │    role: "merchant",
   │    refs: {
   │      orderId: "12345",
   │      customerId: "cust-789"
   │    }
   │  }
   │
   ├─ Handler receives message via conn.ReadJSON(&msg)
   ├─ Unmarshal into StartThreadRequest struct
   ├─ Call ThreadService.HandleStartThread(req, ownerID, companyID)
   │  └─> ownerID and companyID from session (set during connect)
   │
   └─> ┌────────────────────────────────────────────────────────────────────┐
       │ 2. THREAD SERVICE (thread.go:190-394)                              │
       └────────────────────────────────────────────────────────────────────┘
       │
       ├─ VALIDATION: Check ownerID is connected
       │  ├─ Call ConnectionManager.IsConnected(ownerID)
       │  │  └─> Check if connections[ownerID] exists
       │  └─ IF not connected → Return StartThreadResponse{status: "error", message: "Not authenticated"}
       │
       ├─ VALIDATION: IF contractName provided, role is required
       │  └─ IF req.ContractName != "" && req.Role == "" 
       │     → Return error "Role is required when contract name is provided"
       │
       ├─> ┌──────────────────────────────────────────────────────────────┐
       │   │ 3. CONTRACT VALIDATION (if contractName provided)            │
       │   └──────────────────────────────────────────────────────────────┘
       │   │
       │   ├─ Parse contract identifier:
       │   │  ├─ Input: "order_flow" → name="order_flow", version=0 (latest)
       │   │  ├─ Input: "order_flow:2" → name="order_flow", version=2
       │   │  └─ Call parseContractIdentifier(req.ContractName)
       │   │     └─> Returns: (parsedContractName, contractVersion)
       │   │
       │   └─> ┌────────────────────────────────────────────────────────┐
       │       │ 3a. THREE-TIER CACHE CHECK FOR CONTRACT GRAPH          │
       │       └────────────────────────────────────────────────────────┘
       │       │
       │       ├─ Call ContractValidator.LoadContractGraphIntoCache(name, version, companyID)
       │       │
       │       ├─ TIER 1: In-Memory Cache (CacheService)
       │       │  ├─ Cache key: "graph:{companyID}:{contractName}:{version}"
       │       │  │  Example: "graph:company-456:order_flow:2"
       │       │  │
       │       │  ├─ Check: cacheService.Get(key)
       │       │  │  └─> Lookup in sync.Map
       │       │  │
       │       │  └─ IF found:
       │       │     ├─ Return cached ContractGraph (fastest path ~1μs)
       │       │     └─> Skip Tier 2 and Tier 3
       │       │
       │       ├─ TIER 2: Valkey Cache (if Tier 1 miss)
       │       │  ├─ Cache key: "graph:{companyID}:{contractName}:{version}"
       │       │  │
       │       │  ├─ Command: GET graph:company-456:order_flow:2
       │       │  │
       │       │  ├─ IF found:
       │       │  │  ├─ Deserialize JSON to ContractGraph struct
       │       │  │  ├─ Store in memory cache (Tier 1)
       │       │  │  └─ Return graph (~1-5ms)
       │       │  │
       │       │  └─ IF not found → Continue to Tier 3
       │       │
       │       └─ TIER 3: PostgreSQL (Cold Storage - if Tier 1 & 2 miss)
       │          │
       │          ├─ IF version == 0 (latest):
       │          │  └─ Query: 
       │          │     SELECT * FROM contracts 
       │          │     WHERE name=$1 AND company_id=$2 
       │          │     ORDER BY version DESC LIMIT 1
       │          │
       │          ├─ ELSE (specific version):
       │          │  └─ Query:
       │          │     SELECT * FROM contracts 
       │          │     WHERE name=$1 AND company_id=$2 AND version=$3
       │          │
       │          ├─ Parse YAML contract definition from database:
       │          │  contract_yaml = row["definition"]
       │          │  └─> YAML structure:
       │          │      ```yaml
       │          │      name: order_flow
       │          │      version: 2
       │          │      parties: [merchant, customer, logistics]
       │          │      entry_points: [order_placed]
       │          │      terminal_steps: [order_delivered, order_cancelled]
       │          │      max_duration: 86400
       │          │      steps:
       │          │        order_placed:
       │          │          owner: merchant
       │          │          next_steps: [payment_processed, order_cancelled]
       │          │          required_fields: [orderId, amount]
       │          │          optional_fields: [notes]
       │          │          timeout: 300
       │          │        payment_processed:
       │          │          owner: merchant
       │          │          next_steps: [order_shipped, order_cancelled]
       │          │          required_fields: [paymentId]
       │          │      ```
       │          │
       │          ├─ Build ContractGraph data structure:
       │          │  graph = ContractGraph{
       │          │    Name: "order_flow",
       │          │    Version: 2,
       │          │    Parties: ["merchant", "customer", "logistics"],
       │          │    Graph: {
       │          │      EntryPoints: ["order_placed"],
       │          │      TerminalSteps: ["order_delivered", "order_cancelled"],
       │          │      Nodes: map[string]GraphNode{
       │          │        "order_placed": {
       │          │          Name: "order_placed",
       │          │          Owner: "merchant",
       │          │          NextSteps: ["payment_processed", "order_cancelled"],
       │          │          RequiredFields: ["orderId", "amount"],
       │          │          OptionalFields: ["notes"],
       │          │          Timeout: 300,
       │          │          IsTerminal: false
       │          │        },
       │          │        "payment_processed": {...},
       │          │        ...
       │          │      }
       │          │    },
       │          │    MaxDuration: 86400
       │          │  }
       │          │
       │          ├─ Store in Valkey (Tier 2):
       │          │  ├─ Serialize graph to JSON
       │          │  ├─ Command: SET graph:company-456:order_flow:2 {json} EX {contractTTLSeconds}
       │          │  │  └─> TTL: Configurable (e.g., 3600 seconds = 1 hour)
       │          │  └─> Future requests hit Tier 2 cache
       │          │
       │          ├─ Store in memory cache (Tier 1):
       │          │  └─> cacheService.Set(key, graph)
       │          │
       │          └─ Return graph with actual version number (~10-50ms)
       │
       ├─ Store actualVersion = version returned from cache
       │  └─> If version=0 was requested, actualVersion = latest version from DB
       │
       ├─ Generate threadID = uuid.New().String()
       │  └─> Example: "550e8400-e29b-41d4-a716-446655440000"
       │
       ├─ Create Thread object:
       │  thread = &models.Thread{
       │    ID: "550e8400-e29b-41d4-a716-446655440000",
       │    ContractID: &parsedContractName,     // Pointer to "order_flow"
       │    ContractName: "order_flow",
       │    ContractVersion: &actualVersion,     // Pointer to 2
       │    OwnerID: "user-123",
       │    CompanyID: "company-456",
       │    Status: "active",
       │    StartedAt: time.Now(),               // "2026-01-12T12:00:00Z"
       │    CompletedAt: nil,
       │    LastHash: "",                         // Empty initially
       │    Refs: map[string]string{
       │      "orderId": "12345",
       │      "customerId": "cust-789"
       │    }
       │  }
       │
       ├─> ┌──────────────────────────────────────────────────────────────┐
       │   │ 4. VALKEY WRITE - Thread Repository                          │
       │   └──────────────────────────────────────────────────────────────┘
       │   │
       │   ├─ Call ThreadRepository.Save(ctx, thread)
       │   │
       │   ├─ Serialize thread to JSON:
       │   │  threadJSON = {
       │   │    "id": "550e8400-e29b-41d4-a716-446655440000",
       │   │    "contract_id": "order_flow",
       │   │    "contract_name": "order_flow",
       │   │    "contract_version": 2,
       │   │    "owner_id": "user-123",
       │   │    "company_id": "company-456",
       │   │    "status": "active",
       │   │    "started_at": "2026-01-12T12:00:00Z",
       │   │    "completed_at": null,
       │   │    "last_hash": "",
       │   │    "refs": {"orderId": "12345", "customerId": "cust-789"}
       │   │  }
       │   │
       │   ├─ Valkey key: thread:{threadID}
       │   │  Example: "thread:550e8400-e29b-41d4-a716-446655440000"
       │   │
       │   ├─ Command: SET thread:550e8400-e29b-41d4-a716-446655440000 {threadJSON} EX {threadTTLSeconds}
       │   │  └─> TTL: 86400 seconds (24 hours, configurable)
       │   │
       │   └─ Log: "Successfully saved thread 550e8400-e29b-41d4-a716-446655440000 to Valkey"
       │
       ├─ Cache thread in memory:
       │  └─ CacheManager.SetThread(threadID, thread)
       │     └─> Stored in sync.Map for fast GetThread() calls
       │
       ├─> ┌──────────────────────────────────────────────────────────────┐
       │   │ 5. ACCESS CONTROL - Grant Creator Access                     │
       │   └──────────────────────────────────────────────────────────────┘
       │   │
       │   ├─ Determine creator role:
       │   │  ├─ IF req.Role != "": creatorRole = req.Role  // "merchant"
       │   │  └─ ELSE: creatorRole = "owner"  // Default
       │   │
       │   ├─ Set creator permissions:
       │   │  creatorPermissions = ["read", "write", "invite", "manage"]
       │   │
       │   ├─ Call GrantOrUpdateThreadAccess(
       │   │    threadID,
       │   │    ownerID,
       │   │    creatorRole,
       │   │    creatorPermissions,
       │   │    invitedBy="self",
       │   │    isCreator=true,
       │   │    explicitScope=nil
       │   │  )
       │   │
       │   └─> ┌────────────────────────────────────────────────────────┐
       │       │ 5a. SCOPE RESOLUTION                                   │
       │       └────────────────────────────────────────────────────────┘
       │       │
       │       ├─ Call ScopeResolver.ResolveScope(
       │       │    threadID,
       │       │    userID,
       │       │    role,
       │       │    isCreator=true,
       │       │    explicitScope=nil
       │       │  )
       │       │
       │       ├─ Resolution logic:
       │       │  IF isCreator == true:
       │       │    scope = "owner"
       │       │  ELSE IF explicitScope != nil:
       │       │    scope = *explicitScope
       │       │  ELSE IF contract has scope mapping for role:
       │       │    scope = contract.scopeForRole[role]
       │       │  ELSE:
       │       │    scope = "participant"  // Default
       │       │
       │       └─ Return scope = "owner"
       │          └─> Creator always gets "owner" scope for notifications
       │       │
       │       └─> ┌────────────────────────────────────────────────────┐
       │           │ 5b. VALKEY WRITE - Access Repository (Lua Script)  │
       │           └────────────────────────────────────────────────────┘
       │           │
       │           ├─ Call AccessRepository.GrantOrUpdateAccess(
       │           │    ctx,
       │           │    threadID,
       │           │    userID,
       │           │    role,
       │           │    permissions,
       │           │    invitedBy,
       │           │    luaScripts
       │           │  )
       │           │
       │           ├─ ATOMIC LUA SCRIPT EXECUTION:
       │           │  ```lua
       │           │  -- KEYS[1]: thread:{threadID}:access:{userID}
       │           │  -- KEYS[2]: thread:{threadID}:users
       │           │  -- ARGV[1]: role (e.g., "merchant")
       │           │  -- ARGV[2]: permissions (e.g., "read,write,invite,manage")
       │           │  -- ARGV[3]: grantedBy (e.g., "self")
       │           │  -- ARGV[4]: grantedAt (timestamp)
       │           │  -- ARGV[5]: userID
       │           │  
       │           │  -- Get existing access (if any)
       │           │  local existing = redis.call('HGETALL', KEYS[1])
       │           │  local existingRoles = {}
       │           │  
       │           │  -- Parse existing roles if present
       │           │  if existing and existing.roles then
       │           │    existingRoles = cjson.decode(existing.roles)
       │           │  end
       │           │  
       │           │  -- Add new role to roles array
       │           │  table.insert(existingRoles, ARGV[1])
       │           │  
       │           │  -- Remove duplicates
       │           │  local uniqueRoles = {}
       │           │  local seen = {}
       │           │  for _, role in ipairs(existingRoles) do
       │           │    if not seen[role] then
       │           │      table.insert(uniqueRoles, role)
       │           │      seen[role] = true
       │           │    end
       │           │  end
       │           │  
       │           │  -- Set access hash
       │           │  redis.call('HSET', KEYS[1],
       │           │    'roles', cjson.encode(uniqueRoles),
       │           │    'permissions', ARGV[2],
       │           │    'grantedBy', ARGV[3],
       │           │    'grantedAt', ARGV[4],
       │           │    'status', 'active'
       │           │  )
       │           │  
       │           │  -- Add user to thread's user set
       │           │  redis.call('SADD', KEYS[2], ARGV[5])
       │           │  
       │           │  -- Return access object
       │           │  return cjson.encode({
       │           │    roles = uniqueRoles,
       │           │    permissions = ARGV[2],
       │           │    grantedBy = ARGV[3],
       │           │    grantedAt = ARGV[4],
       │           │    status = 'active'
       │           │  })
       │           │  ```
       │           │
       │           ├─ Result - Key: thread:{threadID}:access:{ownerID}
       │           │  Hash fields:
       │           │  {
       │           │    roles: '["merchant"]',
       │           │    permissions: "read,write,invite,manage",
       │           │    grantedBy: "self",
       │           │    grantedAt: "2026-01-12T12:00:00Z",
       │           │    status: "active"
       │           │  }
       │           │
       │           ├─ Result - Key: thread:{threadID}:users
       │           │  Set members: {"user-123"}
       │           │
       │           └─> ┌────────────────────────────────────────────────┐
       │               │ 5c. ASYNC ACTIVITY LOG (goroutine)            │
       │               └────────────────────────────────────────────────┘
       │               │
       │               ├─ Get serviceName from ConnectionManager:
       │               │  ├─ Call ConnectionManager.GetClient(ownerID)
       │               │  └─> Returns Client{ServiceName: "merchant-service"}
       │               │
       │               ├─ Build activity event:
       │               │  activityValues = {
       │               │    type: "access_granted",
       │               │    thread_id: "550e8400-e29b-41d4-a716-446655440000",
       │               │    user_id: "user-123",
       │               │    actor: "user-123",
       │               │    actor_service: "merchant-service",
       │               │    role: "merchant",
       │               │    permissions: "read,write,invite,manage",
       │               │    granted_by: "self",
       │               │    scope: "owner",
       │               │    timestamp: "2026-01-12T12:00:00Z"
       │               │  }
       │               │
       │               ├─ VALKEY WRITE - Activity List:
       │               │  ├─ Key: thread:{threadID}:activity
       │               │  ├─ Serialize event to JSON
       │               │  ├─ Command: LPUSH thread:550e8400-e29b-41d4-a716-446655440000:activity {json}
       │               │  └─ Command: EXPIRE thread:550e8400-e29b-41d4-a716-446655440000:activity 604800
       │               │     └─> TTL: 604800 seconds (7 days)
       │               │
       │               └─ NATS PUBLISH - Access Event:
       │                  ├─ Call: natsArchivalPublisher.PublishThreadAccess(ctx, activityValues)
       │                  │
       │                  ├─ Subject: "access.thread"
       │                  │
       │                  ├─ Payload (JSON):
       │                  │  {
       │                  │    threadId: "550e8400-e29b-41d4-a716-446655440000",
       │                  │    userId: "user-123",
       │                  │    roles: '["merchant"]',
       │                  │    permissions: "read,write,invite,manage",
       │                  │    grantedBy: "self",
       │                  │    grantedAt: "2026-01-12T12:00:00Z",
       │                  │    status: "active",
       │                  │    scope: "owner"
       │                  │  }
       │                  │
       │                  └─> NATS JetStream:
       │                      ├─ Publish to stream "thread_access"
       │                      ├─ Message persisted to disk
       │                      └─> Archiver will consume and write to PostgreSQL
       │
       └─> ┌──────────────────────────────────────────────────────────────┐
           │ 6. ASYNC THREAD METADATA ARCHIVAL (goroutine)                │
           └──────────────────────────────────────────────────────────────┘
           │
           ├─ Spawn goroutine with panic recovery:
           │  go func() {
           │    defer func() {
           │      if r := recover(); r != nil {
           │        fmt.Printf("❌ PANIC in thread metadata goroutine: %v\n", r)
           │      }
           │    }()
           │    // ... archival logic ...
           │  }()
           │
           ├─> ┌────────────────────────────────────────────────────────┐
           │   │ 6a. NATS PUBLISH - Thread Metadata                     │
           │   └────────────────────────────────────────────────────────┘
           │   │
           │   ├─ Build metadata payload:
           │   │  streamValues = {
           │   │    threadId: "550e8400-e29b-41d4-a716-446655440000",
           │   │    ownerId: "user-123",
           │   │    companyId: "company-456",
           │   │    contractId: "order_flow",
           │   │    contractName: "order_flow",
           │   │    contractVersion: "2",
           │   │    error: "",
           │   │    startedAt: "2026-01-12T12:00:00Z",
           │   │    maxlen: "~",
           │   │    limit: 100000
           │   │  }
           │   │
           │   ├─ Call: natsArchivalPublisher.PublishThreadMetadata(ctx, streamValues)
           │   │  └─> Timeout: 5 seconds
           │   │
           │   ├─ Subject: "metadata.thread"
           │   │
           │   └─> NATS JetStream:
           │       ├─ Publish to stream "thread_metadata"
           │       ├─ Message persisted to disk
           │       └─> Archiver consumes → PostgreSQL threads table
           │           └─> INSERT INTO threads (id, company_id, contract_id, ...)
           │
           ├─> ┌────────────────────────────────────────────────────────┐
           │   │ 6b. NATS PUBLISH - Thread Refs (if provided)           │
           │   └────────────────────────────────────────────────────────┘
           │   │
           │   ├─ IF thread.Refs != nil && len(thread.Refs) > 0:
           │   │  │
           │   │  └─ FOR EACH key, value in thread.Refs:
           │   │     │
           │   │     ├─ Build ref event:
           │   │     │  refEvent = {
           │   │     │    threadId: "550e8400-e29b-41d4-a716-446655440000",
           │   │     │    refKey: "orderId",
           │   │     │    refValue: "12345",
           │   │     │    action: "ref_added"
           │   │     │  }
           │   │     │
           │   │     ├─ Call: natsArchivalPublisher.PublishThreadMetadata(ctx, refEvent)
           │   │     │  └─> Timeout: 5 seconds
           │   │     │
           │   │     ├─ Subject: "metadata.thread"
           │   │     │
           │   │     └─> NATS JetStream:
           │   │         ├─ Publish to stream "thread_metadata"
           │   │         └─> Archiver consumes → PostgreSQL thread_refs table
           │   │             └─> INSERT INTO thread_refs (thread_id, ref_key, ref_value, ...)
           │   │
           │   └─ Log: "✅ SUCCESS: Published {count} refs to NATS for thread {threadID}"
           │
           ├─> ┌────────────────────────────────────────────────────────┐
           │   │ 6c. VALKEY WRITE - Activity List (thread_created)      │
           │   └────────────────────────────────────────────────────────┘
           │   │
           │   ├─ Get serviceName from ConnectionManager
           │   │
           │   ├─ Build activity event:
           │   │  activityValues = {
           │   │    type: "thread_created",
           │   │    thread_id: "550e8400-e29b-41d4-a716-446655440000",
           │   │    owner_id: "user-123",
           │   │    actor: "user-123",
           │   │    actor_service: "merchant-service",
           │   │    contract_id: "order_flow",
           │   │    contract_name: "order_flow",
           │   │    contract_version: "2",
           │   │    role: "merchant",
           │   │    timestamp: "2026-01-12T12:00:00Z"
           │   │  }
           │   │
           │   ├─ Serialize to JSON
           │   │
           │   ├─ Command: LPUSH thread:550e8400-e29b-41d4-a716-446655440000:activity {json}
           │   │
           │   └─ Command: EXPIRE thread:550e8400-e29b-41d4-a716-446655440000:activity 604800
           │
           └─> ┌────────────────────────────────────────────────────────┐
               │ 6d. NATS PUBLISH - Activity Log                        │
               └────────────────────────────────────────────────────────┘
               │
               ├─ Call: natsArchivalPublisher.PublishActivityLog(ctx, activityValues)
               │  └─> Timeout: 5 seconds
               │
               ├─ Subject: "activity.log"
               │
               ├─ Payload: {same as 6c activityValues}
               │
               └─> NATS JetStream:
                   ├─ Publish to stream "activity_log"
                   ├─ Message persisted to disk
                   └─> Archiver consumes → PostgreSQL thread_activities table
                       └─> INSERT INTO thread_activities (thread_id, activity_type="thread_created", ...)

┌─────────────────────────────────────────────────────────────────────────────┐
│ 7. WEBSOCKET HANDLER - Post-Response Actions (thread.go:146-160)            │
└─────────────────────────────────────────────────────────────────────────────┘
   │
   ├─ IF response.Status == "success":
   │  │
   │  ├─ Add threadID to session (thread-safe):
   │  │  session.mu.Lock()
   │  │  IF !utils.Contains(session.threadIDs, response.ThreadID):
   │  │    session.threadIDs = append(session.threadIDs, response.ThreadID)
   │  │  session.mu.Unlock()
   │  │
   │  ├─ Subscribe to notifications (old NATS consumer):
   │  │  └─ Call subscribeToNotifications(session, threadID, "owner")
   │  │     ├─ Create WebSocketNotificationHandler for session
   │  │     └─> NATS queue subscription: "notifications.{threadID}"
   │  │         └─> Messages delivered to this specific session
   │  │
   │  └─ Subscribe via NotificationRouter (new router):
   │     └─ Call notificationRouter.SubscribeToThread(clientID, threadID)
   │        ├─ Get client from router.clients[clientID]
   │        ├─ client.mu.Lock()
   │        ├─ client.ThreadIDs[threadID] = true
   │        ├─ client.mu.Unlock()
   │        └─> Client now receives notifications for this thread
   │
   └─ Send StartThreadResponse to client:
      conn.WriteJSON(StartThreadResponse{
        action: "startThread",
        status: "success",
        message: "Thread started successfully",
        threadId: "550e8400-e29b-41d4-a716-446655440000"
      })
```

### **Database Impact Summary**

**Valkey (Immediate Writes - Synchronous)**:

1. **`thread:{threadID}`** → Thread JSON
   - TTL: 86400 seconds (24 hours)
   - Size: ~500 bytes - 2KB depending on refs
   - Purpose: Hot cache for active threads

2. **`thread:{threadID}:access:{ownerID}`** → Access hash
   - Fields: roles, permissions, grantedBy, grantedAt, status
   - No TTL (persists with thread)
   - Purpose: Fast permission checks

3. **`thread:{threadID}:users`** → Set of user IDs
   - Members: {ownerID}
   - No TTL
   - Purpose: Quick user lookup for thread

4. **`thread:{threadID}:activity`** → List of activity events
   - TTL: 604800 seconds (7 days)
   - Purpose: Recent activity history for debugging

**NATS JetStream (Async Publish - Fire & Forget)**:

1. **Stream: `thread_metadata`** → Thread metadata event
   - Subject: "metadata.thread"
   - Retention: Until archived to PostgreSQL
   - Consumer: Archiver service

2. **Stream: `thread_metadata`** → Ref events (one per ref)
   - Subject: "metadata.thread"
   - Count: N events for N refs
   - Consumer: Archiver service

3. **Stream: `thread_access`** → Access granted event
   - Subject: "access.thread"
   - Consumer: Archiver service

4. **Stream: `activity_log`** → Thread created event
   - Subject: "activity.log"
   - Consumer: Archiver service

**PostgreSQL (Eventually Consistent via Archiver)**:

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

*Due to length constraints, I'll create this as a new file. The complete document with all 9 cases would be extremely long. Should I:*

1. *Replace the existing ThreadifyServer.md with this detailed version for cases 1-2, then continue with cases 3-9?*
2. *Create a new file ThreadifyServer_COMPLETE.md with all cases fully detailed?*
3. *Keep the current summary version and create detailed case-by-case files?*

*Which approach would you prefer?*
