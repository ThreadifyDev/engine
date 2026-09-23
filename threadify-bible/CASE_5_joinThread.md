# CASE 5: `joinThread` - Thread Joining with Token or Direct Access

**Handler Entry Point**: `/internal/handlers/thread.go:172-175`

**Purpose**: Allow users to join a thread either via invitation token or direct access (same company), granting appropriate access rights and recording the join event.

---

## Complete Flow Diagram

```
Client Request (joinThread)
    ↓
WebSocket Handler
    ↓
ThreadService.HandleJoinThread
    ↓
├─→ Token-Based Join (if token provided)
│   ├─→ JWT Token Validation
│   ├─→ Extract claims (threadID, role, permissions)
│   └─→ Contract validation (role in parties)
│
├─→ Direct Join (if no token, same company)
│   ├─→ Thread retrieval
│   ├─→ Company match validation
│   └─→ Contract validation (role in parties)
│
└─→ GrantOrUpdateThreadAccess
    ├─→ Scope resolution
    ├─→ Valkey write (access hash)
    └─→ NATS publish (access event + activity log)
```

---

## Request Structures

### **Token-Based Join**

```json
{
  "action": "joinThread",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### **Direct Join (Same Company)**

```json
{
  "action": "joinThread",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "role": "customer"
}
```

---

## Detailed Flow

### **Step 1: WebSocket Handler**

**Location**: `/internal/handlers/thread.go:172-175`

```
Client sends WebSocket message:
{
  action: "joinThread",
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." OR threadId: "...", role: "..."
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "joinThread"
├─ Unmarshal into JoinThreadRequest struct
└─ Call ThreadService.HandleJoinThread(req, ownerID, companyID)
```

---

### **Step 2: ThreadService - Route Selection**

**Location**: `/internal/service/thread.go:822-900`

```
ThreadService.HandleJoinThread(req, ownerID, companyID):

VALIDATION 1: Authentication
├─ Call ConnectionManager.IsConnected(ownerID)
└─ IF not connected → Return error "Not authenticated"

ROUTE SELECTION:
├─ IF req.Token != "":
│  └─> Go to Token-Based Join Flow
│
└─ ELSE IF req.ThreadID != "" && req.Role != "":
   └─> Go to Direct Join Flow
   
ELSE:
└─> Return error "Either token or (threadId + role) must be provided"
```

---

### **Path A: Token-Based Join**

**Location**: `/internal/service/thread.go:822-860`

```
┌─────────────────────────────────────────────────────────────────┐
│ A.1: JWT Token Validation                                       │
└─────────────────────────────────────────────────────────────────┘

Call InvitationService.ValidateToken(req.Token):

JWT Validation Steps:
├─ Split token into parts: header.payload.signature
│
├─ Base64 decode header and payload
│
├─ Verify signature:
│  ├─ Recalculate: HMAC-SHA256(header + "." + payload, secretKey)
│  ├─ Compare with provided signature
│  └─ IF mismatch → Return error "Invalid token signature"
│
├─ Parse claims from payload:
│  {
│    threadID: "550e8400-e29b-41d4-a716-446655440000",
│    contractID: "order_flow",
│    inviterID: "user-123",
│    role: "logistics",
│    permissions: ["read", "write"],
│    exp: 1736769600,
│    iat: 1736683200,
│    jti: "token-uuid"
│  }
│
├─ Check expiration:
│  ├─ currentTime = time.Now().Unix()
│  ├─ IF currentTime > claims.exp:
│  │  └─> Return error "Token has expired"
│  └─ ELSE: Continue
│
└─ Return TokenClaims{
     ThreadID: "550e8400-e29b-41d4-a716-446655440000",
     Role: "logistics",
     Permissions: ["read", "write"],
     InviterID: "user-123"
   }


┌─────────────────────────────────────────────────────────────────┐
│ A.2: Thread Retrieval                                           │
└─────────────────────────────────────────────────────────────────┘

├─ Extract threadID from token claims
├─ Call ThreadService.GetThread(threadID)
│  └─> Cache-aside pattern (memory → Valkey → error)
└─ Thread retrieved successfully


┌─────────────────────────────────────────────────────────────────┐
│ A.3: Contract Validation (if contract exists)                   │
└─────────────────────────────────────────────────────────────────┘

IF thread.ContractName != "":

  Get Contract Graph:
  ├─ Call ContractValidator.GetContractGraph(contractName, version, companyID)
  └─> Returns cached ContractGraph

  VALIDATION: Role in Contract Parties
  ├─ Extract role from token claims
  ├─ Check if role in graph.Parties
  │  Example: Check if "logistics" in ["merchant", "customer", "logistics"]
  │
  └─ IF not found → Return error "Role '{role}' is not valid for this contract"

ELSE:
  └─ Skip contract validation


┌─────────────────────────────────────────────────────────────────┐
│ A.4: Grant Access                                               │
└─────────────────────────────────────────────────────────────────┘

Call GrantOrUpdateThreadAccess(
  threadID,
  ownerID,
  role,                    // From token claims
  permissions,             // From token claims
  invitedBy,               // From token claims (inviterID)
  isCreator=false,
  explicitScope=nil
)

└─> Continue to Step 3 (Access Grant Flow)
```

---

### **Path B: Direct Join (Same Company)**

**Location**: `/internal/service/thread.go:862-900`

```
┌─────────────────────────────────────────────────────────────────┐
│ B.1: Required Fields Validation                                 │
└─────────────────────────────────────────────────────────────────┘

├─ Check req.ThreadID != ""
├─ Check req.Role != ""
└─ IF any missing → Return error "Missing required field"


┌─────────────────────────────────────────────────────────────────┐
│ B.2: Thread Retrieval                                           │
└─────────────────────────────────────────────────────────────────┘

├─ Call ThreadService.GetThread(req.ThreadID)
│  └─> Cache-aside pattern (memory → Valkey → error)
└─ Thread retrieved successfully


┌─────────────────────────────────────────────────────────────────┐
│ B.3: Company Match Validation                                   │
└─────────────────────────────────────────────────────────────────┘

├─ Check if thread.CompanyID == companyID (from session)
│
└─ IF not match → Return error "Cannot join thread from different company without invitation token"


┌─────────────────────────────────────────────────────────────────┐
│ B.4: Contract Validation (if contract exists)                   │
└─────────────────────────────────────────────────────────────────┘

IF thread.ContractName != "":

  Get Contract Graph:
  ├─ Call ContractValidator.GetContractGraph(contractName, version, companyID)
  └─> Returns cached ContractGraph

  VALIDATION: Role in Contract Parties
  ├─ Check if req.Role in graph.Parties
  │  Example: Check if "customer" in ["merchant", "customer", "logistics"]
  │
  └─ IF not found → Return error "Role '{role}' is not valid for this contract"

ELSE:
  └─ Skip contract validation


┌─────────────────────────────────────────────────────────────────┐
│ B.5: Grant Access with Default Permissions                      │
└─────────────────────────────────────────────────────────────────┘

Set default permissions based on role:
├─ IF role == "owner" OR role == "admin":
│  └─> permissions = ["read", "write", "invite", "manage"]
│
└─ ELSE:
   └─> permissions = ["read", "write"]

Call GrantOrUpdateThreadAccess(
  threadID,
  ownerID,
  req.Role,
  permissions,
  invitedBy="self",        // Direct join, not invited
  isCreator=false,
  explicitScope=nil
)

└─> Continue to Step 3 (Access Grant Flow)
```

---

### **Step 3: Access Grant Flow**

**Location**: `/internal/service/thread.go:950-1022`

**This is the same flow as in CASE 2 (thread creation), Step 6**

```
┌─────────────────────────────────────────────────────────────────┐
│ 3.1: Scope Resolution                                           │
└─────────────────────────────────────────────────────────────────┘

Call ScopeResolver.ResolveScope(threadID, userID, role, isCreator=false, explicitScope=nil):

Resolution logic:
├─ IF isCreator == true:
│  └─> scope = "owner"
│
├─ ELSE IF explicitScope != nil:
│  └─> scope = *explicitScope
│
├─ ELSE IF contract has scope mapping for role:
│  └─> scope = contract.scopeForRole[role]
│      Example: "logistics" → "participant"
│
└─ ELSE:
   └─> scope = "participant"  // Default

Return scope = "participant"


┌─────────────────────────────────────────────────────────────────┐
│ 3.2: Valkey Write - Access Repository (Atomic Lua Script)       │
└─────────────────────────────────────────────────────────────────┘

Call AccessRepository.GrantOrUpdateAccess(ctx, threadID, userID, role, permissions, invitedBy, luaScripts):

Lua Script (same as CASE 2):
```lua
-- KEYS[1]: thread:{threadID}:access:{userID}
-- KEYS[2]: thread:{threadID}:users
-- ARGV[1]: role
-- ARGV[2]: permissions
-- ARGV[3]: grantedBy (inviterID or "self")
-- ARGV[4]: grantedAt (timestamp)
-- ARGV[5]: userID

local existing = redis.call('HGETALL', KEYS[1])
local existingRoles = {}

if existing and existing.roles then
  existingRoles = cjson.decode(existing.roles)
end

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

redis.call('HSET', KEYS[1],
  'roles', cjson.encode(uniqueRoles),
  'permissions', ARGV[2],
  'grantedBy', ARGV[3],
  'grantedAt', ARGV[4],
  'status', 'active'
)

redis.call('SADD', KEYS[2], ARGV[5])

return cjson.encode({
  roles = uniqueRoles,
  permissions = ARGV[2],
  grantedBy = ARGV[3],
  grantedAt = ARGV[4],
  status = 'active'
})
```

Result in Valkey:

Key: thread:{threadID}:access:{ownerID}
Hash:
{
  roles: '["logistics"]',
  permissions: "read,write",
  grantedBy: "user-123",  // Inviter ID (or "self" for direct join)
  grantedAt: "2026-01-12T12:00:00Z",
  status: "active"
}

Key: thread:{threadID}:users
Set: {"user-123", "user-456"}  // Added new user


┌─────────────────────────────────────────────────────────────────┐
│ 3.3: Async Activity Log (Goroutine)                             │
└─────────────────────────────────────────────────────────────────┘

Spawn goroutine:
go func() {
  defer func() {
    if r := recover(); r != nil {
      log.Printf("❌ PANIC in join activity log: %v", r)
    }
  }()

  Get serviceName:
  ├─ Call ConnectionManager.GetClient(ownerID)
  └─> Returns Client{ServiceName: "logistics-service"}

  Build Activity Event:
  activityValues = {
    type: "access_granted",
    thread_id: threadID,
    user_id: ownerID,
    actor: ownerID,
    actor_service: "logistics-service",
    role: "logistics",
    permissions: "read,write",
    granted_by: "user-123",  // Inviter or "self"
    scope: "participant",
    timestamp: "2026-01-12T12:00:00Z",
    join_method: "token" OR "direct"
  }

  VALKEY WRITE - Activity List:
  ├─ Key: thread:{threadID}:activity
  ├─ Command: LPUSH thread:{threadID}:activity {json}
  └─ Command: EXPIRE thread:{threadID}:activity 604800

  NATS PUBLISH - Access Event:
  ├─ Call: natsArchivalPublisher.PublishThreadAccess(ctx, activityValues)
  ├─ Subject: "access.thread"
  └─> NATS JetStream → Archiver → PostgreSQL thread_access table

  NATS PUBLISH - Activity Log:
  ├─ Call: natsArchivalPublisher.PublishActivityLog(ctx, activityValues)
  ├─ Subject: "activity.log"
  └─> NATS JetStream → Archiver → PostgreSQL thread_activities table
}()
```

---

### **Step 4: WebSocket Handler - Post-Response**

**Location**: `/internal/handlers/thread.go:175`

```
IF response.Status == "success":

  Add threadID to session:
  ├─ session.mu.Lock()
  ├─ session.threadIDs = append(session.threadIDs, response.ThreadID)
  └─ session.mu.Unlock()

  Subscribe to notifications:
  ├─ Call subscribeToNotifications(session, threadID, scope)
  └─ Call notificationRouter.SubscribeToThread(clientID, threadID)

Send JoinThreadResponse to client:
conn.WriteJSON(JoinThreadResponse{
  action: "joinThread",
  status: "success",
  message: "Successfully joined thread",
  threadId: threadID,
  role: role,
  permissions: permissions
})
```

---

## Database Impact Summary

### **Valkey (Immediate Writes)**

1. **`thread:{threadID}:access:{ownerID}`** → Access hash (new user)
2. **`thread:{threadID}:users`** → Set (add new user)
3. **`thread:{threadID}:activity`** → List (access granted event)

### **NATS JetStream (Async Publish)**

1. **Stream: `thread_access`** → Access granted event
2. **Stream: `activity_log`** → Join activity event

### **PostgreSQL (Eventually Consistent)**

1. **`thread_access` table** → INSERT new access record
2. **`thread_activities` table** → INSERT join event

---

## Performance Characteristics

**Total Response Time**: ~5-10ms

**Token-Based Join**:
1. JWT validation: ~1-2ms
2. Thread retrieval: ~1ms (cache hit)
3. Contract validation: ~1μs (memory cache)
4. Access grant: ~2-3ms (Lua script)
5. Total: ~5-7ms

**Direct Join**:
1. Thread retrieval: ~1ms (cache hit)
2. Company validation: ~1μs
3. Contract validation: ~1μs (memory cache)
4. Access grant: ~2-3ms (Lua script)
5. Total: ~4-6ms

**Async Operations** (non-blocking):
- Activity log archival: ~5-10ms
- Access event archival: ~5-10ms

---

## Error Scenarios

### **Token-Based Join Errors**

1. **Invalid signature**: "Invalid token signature"
2. **Expired token**: "Token has expired"
3. **Invalid role**: "Role '{role}' is not valid for this contract"
4. **Thread not found**: "Thread not found"

### **Direct Join Errors**

1. **Company mismatch**: "Cannot join thread from different company without invitation token"
2. **Invalid role**: "Role '{role}' is not valid for this contract"
3. **Missing fields**: "Missing required field: {field}"
4. **Thread not found**: "Thread not found"

---

## Use Cases

### **Use Case 1: Logistics Partner Joins via Token**

```
Flow:
1. Merchant invites logistics partner (CASE 4)
2. Merchant shares token with logistics partner
3. Logistics partner calls joinThread with token
4. System validates token, grants access
5. Logistics partner can now record steps and receive notifications

Request:
{
  action: "joinThread",
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}

Response:
{
  status: "success",
  message: "Successfully joined thread",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  role: "logistics",
  permissions: ["read", "write"]
}
```

### **Use Case 2: Customer Joins Directly (Same Company)**

```
Flow:
1. Customer from same company wants to join thread
2. Customer calls joinThread with threadID and role
3. System validates company match and role
4. Customer granted access with default permissions

Request:
{
  action: "joinThread",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  role: "customer"
}

Response:
{
  status: "success",
  message: "Successfully joined thread",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  role: "customer",
  permissions: ["read", "write"]
}
```

---

## Security Considerations

1. **Token validation**: Signature and expiration checked
2. **Company isolation**: Direct join only allowed within same company
3. **Role validation**: Role must be in contract parties
4. **Audit trail**: All joins recorded in activity log
5. **Idempotent**: Multiple joins with same token don't create duplicate access

---

*This completes the detailed flow for CASE 5: joinThread*
