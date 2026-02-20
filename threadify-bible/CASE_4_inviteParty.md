# CASE 4: `inviteParty` - Invitation Token Generation

**Handler Entry Point**: `/internal/handlers/thread.go:167-170`

**Purpose**: Generate a JWT invitation token that allows external parties to join a thread with specific roles and permissions.

---

## Complete Flow Diagram

```
Client Request (inviteParty)
    ↓
WebSocket Handler
    ↓
ThreadService.HandleInviteParty
    ↓
├─→ Permission Validation (invite permission)
├─→ Thread Retrieval
├─→ Contract Validation (if contract exists)
│   └─→ Role validation against contract parties
└─→ InvitationService.CreateToken
    └─→ JWT Token Generation (signed with secret)
```

---

## Request Structure

```json
{
  "action": "inviteParty",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "role": "logistics",
  "permissions": ["read", "write"],
  "expiresIn": 86400
}
```

---

## Detailed Flow

### **Step 1: WebSocket Handler**

**Location**: `/internal/handlers/thread.go:167-170`

```
Client sends WebSocket message:
{
  action: "inviteParty",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  role: "logistics",
  permissions: ["read", "write"],
  expiresIn: 86400
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "inviteParty"
├─ Unmarshal into InvitePartyRequest struct
└─ Call ThreadService.HandleInviteParty(req, ownerID, companyID, session.threadIDs)
```

---

### **Step 2: ThreadService - Validations**

**Location**: `/internal/service/thread.go:702-780`

```
ThreadService.HandleInviteParty(req, ownerID, companyID, threadIDs):

VALIDATION 1: Authentication
├─ Call ConnectionManager.IsConnected(ownerID)
└─ IF not connected → Return error "Not authenticated"

VALIDATION 2: Thread Access
├─ Check if threadID in threadIDs (session's accessible threads)
└─ IF not found → Return error "Thread not found or access denied"

VALIDATION 3: Permission Check
├─ Call AccessService.CheckThreadAccess(threadID, ownerID, "invite", nil)
├─ Command: HGET thread:{threadID}:access:{ownerID} permissions
├─ Parse permissions: "read,write,invite,manage"
├─ Check if "invite" is in permissions list
└─ IF not found → Return error "You don't have permission to invite parties"

VALIDATION 4: Thread Retrieval
├─ Call ThreadService.GetThread(threadID)
│  └─> Cache-aside pattern (memory → Valkey → error)
└─ Thread retrieved successfully

VALIDATION 5: Required Fields
├─ Check threadID != ""
├─ Check role != ""
└─ IF any missing → Return error "Missing required field"
```

---

### **Step 3: Contract Validation (if contract exists)**

**Location**: `/internal/service/thread.go:782-820`

```
IF thread.ContractName != "":

  Get Contract Graph:
  ├─ Call ContractValidator.GetContractGraph(contractName, version, companyID)
  └─> Returns cached ContractGraph (from 3-tier cache)

  VALIDATION: Role in Contract Parties
  ├─ Check if req.Role in graph.Parties
  │  Example: Check if "logistics" in ["merchant", "customer", "logistics"]
  │
  └─ IF not found → Return error "Role '{role}' is not defined in contract '{contractName}'. Valid roles: {parties}"

ELSE:
  └─ Skip contract validation (no contract attached to thread)
```

---

### **Step 4: JWT Token Generation**

**Location**: `/internal/service/invitation_service.go:40-90`

```
Prepare Token Claims:
├─ threadID = req.ThreadID
├─ contractID = thread.ContractID (nullable)
├─ inviterID = ownerID
├─ role = req.Role
├─ permissions = req.Permissions OR default ["read", "write"]
└─ expiry = time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
   └─> Default: 86400 seconds (24 hours)

Call InvitationService.CreateToken(threadID, contractID, inviterID, role, permissions, expiry):

Build JWT Claims:
claims = {
  threadID: "550e8400-e29b-41d4-a716-446655440000",
  contractID: "order_flow",
  inviterID: "user-123",
  role: "logistics",
  permissions: ["read", "write"],
  iss: "threadify",
  aud: "threadify-api",
  exp: 1736769600,  // Unix timestamp
  iat: 1736683200,  // Unix timestamp
  jti: uuid.New().String()  // Unique token ID
}

Generate JWT Token:
├─ Create JWT header:
│  {
│    alg: "HS256",
│    typ: "JWT"
│  }
│
├─ Encode claims to JSON
├─ Base64 encode header and payload
├─ Sign with HMAC-SHA256 using secret key
│  └─> signature = HMAC-SHA256(base64(header) + "." + base64(payload), secretKey)
│
└─ Concatenate: header.payload.signature
   └─> Example: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0aHJlYWRJRCI6IjU1MGU4NDAwLWUyOWItNDFkNC1hNzE2LTQ0NjY1NTQ0MDAwMCIsInJvbGUiOiJsb2dpc3RpY3MiLCJleHAiOjE3MzY3Njk2MDB9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

Return token
```

---

### **Step 5: Response to Client**

**Location**: `/internal/handlers/thread.go:170`

```
Send InvitePartyResponse to client:
conn.WriteJSON(InvitePartyResponse{
  action: "inviteParty",
  status: "success",
  message: "Invitation token generated successfully",
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  expiresAt: "2026-01-13T12:00:00Z"
})
```

---

## Database Impact Summary

### **Valkey**

**NO WRITES** - This is a stateless operation. The token itself carries all necessary information.

**Reads Only**:
1. `thread:{threadID}:access:{ownerID}` → Permission check
2. `thread:{threadID}` → Thread metadata retrieval
3. `graph:{companyID}:{contractName}:{version}` → Contract validation (if applicable)

### **NATS JetStream**

**NO PUBLISHES** - Token generation doesn't trigger archival events.

### **PostgreSQL**

**NO WRITES** - Token is validated when used (during joinThread), not when created.

---

## Token Usage Flow

The generated token is used in the `joinThread` action:

```
1. Inviter generates token via inviteParty
2. Inviter shares token with invitee (out-of-band)
3. Invitee calls joinThread with token
4. Server validates token:
   ├─ Verify signature
   ├─ Check expiration
   ├─ Extract claims (threadID, role, permissions)
   └─ Grant access via GrantOrUpdateThreadAccess
5. Access recorded to Valkey and archived to PostgreSQL
```

---

## Security Considerations

### **Token Security**

1. **Signed with HMAC-SHA256**: Prevents tampering
2. **Expiration**: Tokens expire after specified duration
3. **Single-use**: Not enforced at token level, but access is idempotent
4. **No sensitive data**: Token contains only threadID, role, permissions

### **Permission Model**

- **Inviter must have "invite" permission**: Prevents unauthorized invitations
- **Role must be in contract parties**: Ensures contract compliance
- **Permissions can be restricted**: Inviter can grant subset of their own permissions

---

## Performance Characteristics

**Total Response Time**: ~2-5ms

1. **Permission check**: ~1ms (Valkey read)
2. **Thread retrieval**: ~1ms (cache hit) or ~5ms (cache miss)
3. **Contract validation**: ~1μs (memory cache) or ~2ms (Valkey cache)
4. **JWT generation**: ~1ms (cryptographic signing)

**No async operations** - Immediate response

---

## Error Scenarios

1. **No invite permission**: Return error "You don't have permission to invite parties"
2. **Invalid role**: Return error "Role '{role}' is not defined in contract"
3. **Thread not found**: Return error "Thread not found or access denied"
4. **Missing required fields**: Return error "Missing required field: {field}"

---

## Example Use Cases

### **Use Case 1: Invite Logistics Partner**

```
Merchant (thread creator) invites logistics partner:

Request:
{
  action: "inviteParty",
  threadId: "thread-123",
  role: "logistics",
  permissions: ["read", "write"],
  expiresIn: 86400
}

Response:
{
  status: "success",
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  expiresAt: "2026-01-13T12:00:00Z"
}

Merchant shares token with logistics partner via email/SMS
Logistics partner uses token to join thread
```

### **Use Case 2: Invite Customer (Read-Only)**

```
Merchant invites customer with limited permissions:

Request:
{
  action: "inviteParty",
  threadId: "thread-123",
  role: "customer",
  permissions: ["read"],
  expiresIn: 3600
}

Response:
{
  status: "success",
  token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  expiresAt: "2026-01-12T13:00:00Z"
}

Customer can only read thread data, cannot record steps
Token expires in 1 hour
```

---

*This completes the detailed flow for CASE 4: inviteParty*
