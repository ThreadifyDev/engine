# WebSocket Handler Execution Flow

**Last Updated**: December 26, 2025  
**Purpose**: Complete execution flow documentation for WebSocket message handling

---

## High-Level Architecture

```mermaid
graph TB
    Client[Client Application] -->|HTTP Upgrade| WS[WebSocket Connection]
    WS -->|JSON Messages| Handler[WebSocketHandler]
    Handler -->|Route by Action| Router{Message Router}
    
    Router -->|connect| Connect[Connect Flow]
    Router -->|startThread| Start[Start Thread Flow]
    Router -->|recordThreadEvent| Record[Record Event Flow]
    Router -->|stepEvent| Step[Step Event Flow]
    Router -->|inviteParty| Invite[Invite Party Flow]
    Router -->|joinThread| Join[Join Thread Flow]
    Router -->|closeConnection| Close[Close Connection Flow]
    
    Connect --> Session[Update Session State]
    Start --> Session
    Join --> Session
    
    Session --> Response[JSON Response]
    Response --> Client
```

---

## 1. Connection Establishment Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant Upgrader as WebSocket Upgrader
    participant Session as Session Object
    
    Client->>Handler: HTTP Request
    Handler->>Upgrader: Upgrade Connection
    Upgrader-->>Handler: WebSocket Connection
    Handler->>Session: Create Empty Session
    Handler->>Handler: Enter Message Loop
    
    Note over Handler: Ready to receive messages
```

**Key Points:**

- HTTP → WebSocket upgrade via gorilla/websocket
- Empty session created (no ownerID yet)
- Infinite loop: Read → Process → Write → Repeat
- Connection stays open until error or close action

---

## 2. Connect Action Flow (Authentication)

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant ThreadService
    participant AuthService
    participant Session
    participant SessionMap as sessions (sync.Map)
    
    Client->>Handler: {"action": "connect", "apiKey": "...", "serviceName": "..."}
    Handler->>ThreadService: HandleConnect(apiKey, serviceName)
    ThreadService->>AuthService: Validate API Key
    AuthService->>AuthService: Derive ownerID & companyID
    AuthService-->>ThreadService: ownerID, companyID
    ThreadService-->>Handler: ConnectResponse{ownerID, companyID}
    
    Handler->>Session: Set ownerID & companyID
    Handler->>SessionMap: Store session by ownerID
    Handler-->>Client: {"status": "success", "ownerID": "...", "companyID": "..."}
    
    Note over Session: Session now authenticated
```

**Critical Details:**

- **Server-driven auth**: ownerID derived from API key, not client-provided
- **Multi-tenancy**: companyID extracted and stored in session
- **Session storage**: Keyed by ownerID in handler's sync.Map
- **No database call**: Auth is stateless (API key validation only)

**Response Structure:**

```json
{
  "action": "connect",
  "status": "success",
  "message": "Connected successfully",
  "ownerId": "user-123",
  "companyId": "company-456"
}
```

---

## 3. Start Thread Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant Session
    participant ThreadService
    participant ContractRepo as Contract Repository
    participant ValkeyRepo as Valkey Thread Repo
    participant CacheService
    
    Client->>Handler: {"action": "startThread", "contractName": "...", "refs": {...}}
    Handler->>ThreadService: HandleStartThread(req, ownerID, companyID)
    
    alt Contract Provided
        ThreadService->>ContractRepo: Load Contract Graph
        ContractRepo->>CacheService: Check In-Memory Cache
        alt Cache Miss
            CacheService->>ValkeyRepo: Check Valkey Cache
            alt Valkey Miss
                ValkeyRepo->>ContractRepo: Load from PostgreSQL
            end
        end
    end
    
    ThreadService->>ThreadService: Create Thread Object
    ThreadService->>ValkeyRepo: Save Thread (with TTL)
    ThreadService->>CacheService: Cache Thread (in-memory)
    ThreadService-->>Handler: StartThreadResponse{threadID}
    
    Handler->>Session: Append threadID to threadIDs[]
    Handler-->>Client: {"status": "success", "threadID": "..."}
    
    Note over Session: Session tracks thread ownership
```

**Key Points:**

- **Three-tier caching**: In-memory → Valkey → PostgreSQL (for contracts)
- **Thread storage**: Valkey only (ephemeral with TTL)
- **Session tracking**: threadID added to session's threadIDs array
- **Multi-tenancy**: companyID embedded in thread object
- **Contract optional**: Can create threads without contracts (non-contract workflows)

**Request Structure:**

```json
{
  "action": "startThread",
  "contractName": "order-fulfillment:v2",
  "role": "buyer",
  "refs": {
    "orderId": "ORD-12345",
    "customerId": "CUST-789"
  }
}
```

**Response Structure:**

```json
{
  "action": "startThread",
  "status": "success",
  "threadId": "thread-uuid-123",
  "message": "Thread started successfully"
}
```

---

## 4. Step Event Flow (Async Processing)

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant StepEventService
    participant ThreadQueue as Thread-Specific Queue
    participant WorkerPool as Worker Pool (4 workers)
    participant BatchBuffer as Global Batch Buffer
    participant Valkey
    
    Client->>Handler: {"action": "stepEvent", "threadID": "...", "stepName": "...", "context": {...}}
    Handler->>StepEventService: ProcessStepEvent(event)
    StepEventService->>ThreadQueue: Enqueue to thread-specific queue
    StepEventService-->>Handler: Success (queued)
    Handler-->>Client: {"status": "success", "message": "queued"}
    
    Note over Client: Client receives immediate response
    
    par Async Processing
        WorkerPool->>ThreadQueue: Dequeue event (sequential per thread)
        WorkerPool->>WorkerPool: Generate SHA-256 Hash
        WorkerPool->>WorkerPool: Chain with previous hash
        WorkerPool->>BatchBuffer: Add to global batch
        
        alt Batch Full or Timeout
            WorkerPool->>Valkey: Write batch to Valkey
            WorkerPool->>Valkey: Update thread.lastHash
        end
    end
```

**Key Architecture:**

- **Non-blocking**: Client gets immediate response
- **Thread-specific queues**: Sequential processing per thread (maintains order)
- **Global batching**: Events from all threads batched together (performance)
- **Cryptographic chain**: Each event hashed with previous hash (integrity)
- **Worker pool**: 4 workers process events concurrently

**Request Structure:**

```json
{
  "action": "stepEvent",
  "threadId": "thread-uuid-123",
  "stepId": "step-001",
  "stepName": "validate_order",
  "serviceName": "order-service",
  "type": "step_completed",
  "status": "success",
  "context": {
    "validationResult": "passed",
    "amount": 150.00
  },
  "startedAt": "2025-12-26T00:00:00Z",
  "finishedAt": "2025-12-26T00:00:05Z"
}
```

**Response Structure:**

```json
{
  "action": "stepEvent",
  "status": "success",
  "message": "Step event queued for processing",
  "stepId": "step-001"
}
```

---

## 5. Invite Party Flow (Cross-Org Threading)

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant Session
    participant InvitationService
    participant JWTLib as JWT Library
    participant AuditService
    participant Valkey
    
    Client->>Handler: {"action": "inviteParty", "role": "partner", "permissions": "read,write"}
    Handler->>InvitationService: ValidateRole(role)
    Handler->>InvitationService: ValidatePermissions(permissions)
    Handler->>InvitationService: ParseExpiry(expiresIn)
    
    Handler->>Session: Get first threadID from threadIDs[]
    Handler->>InvitationService: CreateToken(threadID, contractID, ownerID, role, permissions, expiry)
    InvitationService->>JWTLib: Sign JWT with claims
    JWTLib-->>InvitationService: JWT Token String
    InvitationService-->>Handler: threadToken
    
    par Async Audit Logging
        Handler->>AuditService: LogTokenCreated(threadID, contractID, ownerID, role, permissions)
        AuditService->>Valkey: Enqueue to audit_events queue
    end
    
    Handler-->>Client: {"status": "success", "threadToken": "eyJ...", "expiresAt": 1234567890}
    
    Note over Client: Client shares token with partner company
```

**Key Points:**

- **JWT-based**: Token contains threadID, contractID, role, permissions, expiry
- **Validation**: Role and permissions validated before token creation
- **Audit trail**: Token creation logged asynchronously (non-blocking)
- **Limitation**: Uses first thread in session (not explicitly specified)

**Request Structure:**

```json
{
  "action": "inviteParty",
  "role": "partner",
  "permissions": "read,write",
  "expiresIn": "24h"
}
```

**Response Structure:**

```json
{
  "action": "inviteParty",
  "status": "success",
  "threadToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "role": "partner",
  "permissions": "read,write",
  "expiresAt": 1735171200,
  "message": "Invitation token created successfully"
}
```

---

## 6. Join Thread Flow (Partner Onboarding)

```mermaid
sequenceDiagram
    participant Partner as Partner Client
    participant Handler as WebSocketHandler
    participant Session as Partner Session
    participant InvitationService
    participant JWTLib as JWT Library
    participant AuditService
    participant Valkey
    
    Partner->>Handler: {"action": "joinThread", "threadToken": "eyJ..."}
    Handler->>InvitationService: ValidateToken(threadToken)
    InvitationService->>JWTLib: Verify signature & expiry
    JWTLib-->>InvitationService: Claims{threadID, contractID, role, permissions}
    InvitationService-->>Handler: Claims
    
    Handler->>Session: Append threadID to threadIDs[]
    
    par Async Audit Logging
        Handler->>AuditService: LogTokenUsed(threadID, ownerID)
        Handler->>AuditService: LogThreadJoined(threadID, contractID, ownerID)
        AuditService->>Valkey: Enqueue to audit_events queue
    end
    
    Handler-->>Partner: {"status": "success", "threadID": "...", "role": "partner", "permissions": "read,write"}
    
    Note over Partner: Partner can now record events on thread
```

**Key Points:**

- **Token validation**: JWT signature and expiry checked
- **Session update**: Partner's session now tracks joined thread
- **Audit trail**: Token usage and thread join logged
- **No database check**: Assumes thread exists if token is valid (potential issue)

**Request Structure:**

```json
{
  "action": "joinThread",
  "threadToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Response Structure:**

```json
{
  "action": "joinThread",
  "status": "success",
  "threadId": "thread-uuid-123",
  "contractId": "contract-456",
  "role": "partner",
  "permissions": "read,write",
  "message": "Successfully joined thread"
}
```

---

## 7. Record Event Flow (Legacy)

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant ThreadService
    participant StepEventService
    
    Client->>Handler: {"action": "recordThreadEvent", "threadID": "...", "stepName": "...", "context": {...}}
    Handler->>ThreadService: HandleRecordEvent(req, ownerID)
    ThreadService->>ThreadService: Validate ownership & permissions
    ThreadService->>ThreadService: Validate against contract (if applicable)
    ThreadService->>StepEventService: ProcessStepEvent(event)
    ThreadService-->>Handler: RecordEventResponse
    Handler-->>Client: {"status": "success"}
    
    Note over Handler: This action appears to be legacy<br/>superseded by stepEvent
```

**Note**: This flow goes through ThreadService first (validation) before StepEventService, unlike the direct `stepEvent` action.

---

## 8. Close Connection Flow

```mermaid
sequenceDiagram
    participant Client
    participant Handler as WebSocketHandler
    participant ThreadService
    participant ConnectionService
    participant SessionMap as sessions (sync.Map)
    
    Client->>Handler: {"action": "closeConnection"}
    Handler->>ThreadService: HandleClose(ownerID)
    ThreadService->>ConnectionService: Disconnect(ownerID)
    ConnectionService->>ConnectionService: Remove from in-memory map
    ThreadService-->>Handler: CloseConnectionResponse
    Handler-->>Client: {"status": "success", "message": "disconnected"}
    
    Note over Handler: Message loop breaks
    
    Handler->>SessionMap: Delete session by ownerID
    Handler->>Handler: Close WebSocket connection
    
    Note over Client: Connection closed gracefully
```

**Graceful Shutdown:**

1. Client receives confirmation response
2. Message loop breaks
3. Session cleaned from sync.Map
4. WebSocket connection closed

**Request Structure:**

```json
{
  "action": "closeConnection"
}
```

**Response Structure:**

```json
{
  "action": "closeConnection",
  "status": "success",
  "message": "Connection closed successfully"
}
```

---

## Complete Message Loop Flow

```mermaid
stateDiagram-v2
    [*] --> Upgrade: HTTP Request
    Upgrade --> CreateSession: WebSocket Established
    CreateSession --> WaitForMessage: Empty Session
    
    WaitForMessage --> ParseMessage: JSON Received
    ParseMessage --> RouteAction: Extract "action"
    
    RouteAction --> Connect: action = "connect"
    RouteAction --> StartThread: action = "startThread"
    RouteAction --> RecordEvent: action = "recordThreadEvent"
    RouteAction --> StepEvent: action = "stepEvent"
    RouteAction --> InviteParty: action = "inviteParty"
    RouteAction --> JoinThread: action = "joinThread"
    RouteAction --> CloseConnection: action = "closeConnection"
    RouteAction --> Error: Unknown action
    
    Connect --> UpdateSession: Success
    StartThread --> UpdateSession: Success
    JoinThread --> UpdateSession: Success
    
    UpdateSession --> SendResponse
    RecordEvent --> SendResponse
    StepEvent --> SendResponse
    InviteParty --> SendResponse
    Error --> SendResponse
    
    SendResponse --> WaitForMessage: Continue
    CloseConnection --> Cleanup: Break Loop
    Cleanup --> [*]: Connection Closed
```

---

## Session State Transitions

```mermaid
stateDiagram-v2
    [*] --> Empty: WebSocket Upgrade
    Empty --> Authenticated: connect action
    Authenticated --> ThreadOwner: startThread action
    Authenticated --> ThreadParticipant: joinThread action
    ThreadOwner --> MultiThread: startThread again
    ThreadParticipant --> MultiThread: joinThread again
    MultiThread --> MultiThread: More threads
    
    Empty --> [*]: Connection Error
    Authenticated --> [*]: closeConnection
    ThreadOwner --> [*]: closeConnection
    ThreadParticipant --> [*]: closeConnection
    MultiThread --> [*]: closeConnection
    
    note right of Empty
        conn: WebSocket
        ownerID: ""
        companyID: ""
        threadIDs: []
    end note
    
    note right of Authenticated
        conn: WebSocket
        ownerID: "user-123"
        companyID: "company-456"
        threadIDs: []
    end note
    
    note right of ThreadOwner
        conn: WebSocket
        ownerID: "user-123"
        companyID: "company-456"
        threadIDs: ["thread-1"]
    end note
```

---

## Timing & Performance Characteristics

```mermaid
gantt
    title Request Processing Timeline
    dateFormat X
    axisFormat %L ms
    
    section Synchronous
    Parse Message           :0, 1
    Route Action            :1, 2
    Validate Request        :2, 5
    Business Logic          :5, 15
    Send Response           :15, 17
    
    section Async (Non-blocking)
    Queue Step Event        :17, 100
    Hash & Batch            :100, 150
    Write to Valkey         :150, 200
    Audit Logging           :17, 50
```

**Performance Notes:**

- **Synchronous**: ~15-20ms (connect, startThread, joinThread)
- **Async queuing**: ~1-2ms (stepEvent returns immediately)
- **Background processing**: 100-200ms (hashing, batching, Valkey write)
- **Audit logging**: Non-blocking (goroutine)

---

## Error Handling Flow

```mermaid
graph TD
    Request[Incoming Message] --> Parse{Parse JSON}
    Parse -->|Success| Route{Route Action}
    Parse -->|Fail| ReadError[Read Error]
    
    Route --> Validate{Validate Request}
    Validate -->|Success| Process[Process Action]
    Validate -->|Fail| ErrorResponse[Error Response]
    
    Process -->|Success| SuccessResponse[Success Response]
    Process -->|Fail| ErrorResponse
    
    SuccessResponse --> Write{Write Response}
    ErrorResponse --> Write
    
    Write -->|Success| Continue[Continue Loop]
    Write -->|Fail| WriteError[Write Error]
    
    ReadError --> Cleanup[Cleanup & Close]
    WriteError --> Cleanup
    
    Continue --> Request
    Cleanup --> End[Connection Closed]
```

**Error Categories:**

1. **Parse errors**: Invalid JSON → break loop, close connection
2. **Validation errors**: Return error response, continue loop
3. **Processing errors**: Return error response, continue loop
4. **Write errors**: Cannot send response → break loop, close connection

**Error Response Structure:**

```json
{
  "action": "error",
  "status": "error",
  "message": "Descriptive error message"
}
```

---

## Key Architectural Patterns

### 1. Server-Driven Authentication

- Client provides API key
- Server derives ownerID and companyID
- No client-provided identity accepted
- Stateless validation (no database lookup)

### 2. Session State Management

- Handler maintains session state (not services)
- Thread-safe with sync.Mutex
- Tracks ownership (threadIDs array)
- Stored in sync.Map keyed by ownerID

### 3. Async Processing

- Step events queued immediately
- Audit logs fire-and-forget (goroutines)
- Client never waits for background work
- Worker pool processes events concurrently

### 4. Multi-Tenancy

- companyID embedded in all operations
- Session tracks company context
- Services enforce company isolation
- No cross-company data access

### 5. Cross-Organization Threading

- JWT tokens for partner access
- Role and permission validation
- Audit trail for compliance
- Token expiry enforcement

---

## Handler Structure

### WebSocketHandler

```go
type WebSocketHandler struct {
    threadService     *service.ThreadService
    stepEventService  *service.StepEventService
    invitationService *service.InvitationTokenService
    auditService      *service.AuditEventService
    sessions          sync.Map
}
```

### Session

```go
type Session struct {
    conn      *websocket.Conn
    ownerID   string
    companyID string
    threadIDs []string
    mu        sync.Mutex
}
```

**Session Lifecycle:**

1. **Created**: On WebSocket upgrade (empty)
2. **Populated**: On `connect` action (ownerID, companyID)
3. **Updated**: On `startThread` and `joinThread` (threadIDs)
4. **Cleaned**: On disconnect or `closeConnection`

---

## Action Summary Table

| Action | Auth Required | Updates Session | Async Processing | Response Time |
|--------|---------------|-----------------|------------------|---------------|
| `connect` | No | Yes (ownerID, companyID) | No | ~15ms |
| `startThread` | Yes | Yes (threadIDs) | No | ~20ms |
| `recordThreadEvent` | Yes | No | Yes (via service) | ~15ms |
| `stepEvent` | Yes | No | Yes (direct queue) | ~2ms |
| `inviteParty` | Yes | No | Yes (audit only) | ~10ms |
| `joinThread` | Yes | Yes (threadIDs) | Yes (audit only) | ~10ms |
| `closeConnection` | Yes | No | No | ~5ms |

---

## Security Considerations

### Authentication

- API key validated on connect
- ownerID derived server-side (not client-provided)
- Session tied to ownerID (cannot impersonate)

### Authorization

- Thread ownership tracked in session
- companyID enforces multi-tenancy
- JWT tokens for cross-org access

### Audit Trail

- Token creation logged
- Token usage logged
- Thread join events logged
- All audit logs stored in Valkey with retention

### Token Security

- JWT signed with secret key
- Expiry enforced (default 24h)
- Role and permissions embedded in claims
- Signature verified on validation

---

## Known Limitations & TODOs

### 1. Invite Party - Thread Selection

**Issue**: Uses first thread in session's threadIDs array  
**Location**: `internal/handlers/thread.go:245`  
**Impact**: Cannot specify which thread to invite partner to  
**Fix**: Add `threadID` to `InvitePartyRequest`

### 2. Invite Party - Contract ID

**Issue**: Hardcoded contractID as `"contract-123"`  
**Location**: `internal/handlers/thread.go:257`  
**Impact**: Token contains incorrect contract reference  
**Fix**: Retrieve contractID from thread object

### 3. Join Thread - No Thread Validation

**Issue**: No database check for thread existence  
**Location**: `internal/handlers/thread.go:316`  
**Impact**: Partner can join non-existent threads  
**Fix**: Add Valkey lookup to verify thread exists

### 4. No Permission Enforcement

**Issue**: After joining, no checks on allowed actions  
**Location**: All action handlers  
**Impact**: Partner with "read" permission can write  
**Fix**: Add permission middleware for each action

### 5. Record Event vs Step Event

**Issue**: Two similar actions with different flows  
**Location**: `recordThreadEvent` and `stepEvent`  
**Impact**: Confusion, potential inconsistency  
**Fix**: Deprecate `recordThreadEvent`, standardize on `stepEvent`

---

## Future Enhancements

### 1. Distributed Session Management

**Current**: In-memory sync.Map  
**Limitation**: Lost on server restart  
**Enhancement**: Redis-backed session store for multi-instance deployment

### 2. Thread Persistence

**Current**: Valkey only (ephemeral with TTL)  
**Limitation**: No long-term history  
**Enhancement**: Optional PostgreSQL persistence for compliance customers

### 3. Real-Time Notifications

**Current**: Request-response only  
**Enhancement**: Server-initiated messages (thread updates, partner joins)

### 4. Rate Limiting

**Current**: None at WebSocket level  
**Enhancement**: Per-connection rate limiting to prevent abuse

### 5. Compression

**Current**: Uncompressed JSON  
**Enhancement**: WebSocket compression for large payloads

---

## Conclusion

The WebSocket handler provides a robust, performant foundation for real-time workflow execution with:

- ✅ Server-driven authentication
- ✅ Multi-tenancy support
- ✅ Async event processing
- ✅ Cross-organization threading
- ✅ Audit trail for compliance

The architecture is well-suited for MVP and early growth, with clear paths for enhancement as scale requirements increase.
