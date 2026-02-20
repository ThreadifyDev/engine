# CASE 6: `closeConnection` - Connection Cleanup & Resource Deallocation

**Handler Entry Point**: `/internal/handlers/thread.go:177-180`

**Purpose**: Gracefully close a WebSocket connection, clean up all associated resources, unsubscribe from notifications, and remove session state.

---

## Complete Flow Diagram

```
Client Request (closeConnection) OR WebSocket Close Event
    ↓
WebSocket Handler
    ↓
├─→ Unsubscribe from all notifications
│   ├─→ NATS unsubscribe (old consumer)
│   └─→ NotificationRouter unregister
│
├─→ ConnectionManager cleanup
│   └─→ Remove from connections map
│
├─→ Session cleanup
│   └─→ Remove from sessions map
│
└─→ Close WebSocket connection
```

---

## Request Structure

```json
{
  "action": "closeConnection"
}
```

**OR** WebSocket close event (no explicit message)

---

## Detailed Flow

### **Step 1: WebSocket Handler - Close Trigger**

**Location**: `/internal/handlers/thread.go:177-180, 90-110`

```
Two ways to trigger connection close:

METHOD 1: Explicit closeConnection message
├─ Client sends: {action: "closeConnection"}
├─ Handler receives via conn.ReadJSON(&msg)
├─ Extract action: "closeConnection"
└─> Proceed to cleanup

METHOD 2: WebSocket connection closed by client
├─ conn.ReadJSON() returns error (EOF or close error)
├─ Handler detects connection closed
└─> Proceed to cleanup

Both methods lead to same cleanup flow
```

---

### **Step 2: Notification Unsubscribe**

**Location**: `/internal/handlers/thread.go:180-200`

```
┌─────────────────────────────────────────────────────────────────┐
│ 2.1: NATS Unsubscribe (Old Consumer)                            │
└─────────────────────────────────────────────────────────────────┘

IF session.notificationHandler != nil:

  Get all thread subscriptions:
  ├─ FOR EACH threadID in session.threadIDs:
  │  │
  │  ├─ Get NATS subscription for thread
  │  │  └─> Subject: "notifications.{threadID}"
  │  │
  │  ├─ Call subscription.Unsubscribe()
  │  │  └─> NATS removes consumer from queue group
  │  │
  │  └─ Log: "Unsubscribed from notifications for thread {threadID}"
  │
  └─ Clear notification handler: session.notificationHandler = nil


┌─────────────────────────────────────────────────────────────────┐
│ 2.2: NotificationRouter Unregister (New Router)                 │
└─────────────────────────────────────────────────────────────────┘

Call notificationRouter.UnregisterClient(session.clientID):

├─ Get client from router.clients[clientID]
│
├─ IF client exists:
│  │
│  ├─ Lock router: router.mu.Lock()
│  │
│  ├─ Remove client from map:
│  │  delete(router.clients, clientID)
│  │
│  ├─ Unlock router: router.mu.Unlock()
│  │
│  ├─ Clear client's thread subscriptions:
│  │  client.ThreadIDs = nil
│  │
│  ├─ Clear client's step subscriptions:
│  │  client.Subscriptions = nil
│  │
│  ├─ Clear pending ACKs:
│  │  client.pendingAcks = nil
│  │
│  └─ Log: "Unregistered client {clientID} from notification router"
│
└─ ELSE: Log warning "Client not found in router"
```

---

### **Step 3: ConnectionManager Cleanup**

**Location**: `/internal/handlers/thread.go:202-210`

```
Call ConnectionManager.Disconnect(session.ownerID):

├─ Lock connections map: connections.mu.Lock()
│
├─ Remove from map:
│  delete(connections, session.ownerID)
│  └─> Removes: Client{
│       ApiKey: "...",
│       ServiceName: "merchant-service",
│       CompanyID: "company-456",
│       ConnectedAt: time.Time
│     }
│
├─ Unlock: connections.mu.Unlock()
│
└─ Log: "Disconnected user {ownerID}"
```

---

### **Step 4: Session Cleanup**

**Location**: `/internal/handlers/thread.go:212-220`

```
Remove session from handler's sessions map:

├─ Lock sessions: sessions.mu.Lock()
│
├─ Remove from map:
│  delete(sessions, session.ownerID)
│  └─> Removes: &Session{
│       conn: *websocket.Conn,
│       clientID: "uuid",
│       ownerID: "user-123",
│       companyID: "company-456",
│       threadIDs: []string{...},
│       notificationHandler: nil
│     }
│
├─ Unlock: sessions.mu.Unlock()
│
└─ Log: "Removed session for user {ownerID}"
```

---

### **Step 5: WebSocket Connection Close**

**Location**: `/internal/handlers/thread.go:222-230`

```
Close WebSocket connection:

├─ Send close frame (if not already closed):
│  conn.WriteMessage(websocket.CloseMessage, 
│    websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Connection closed"))
│
├─ Close underlying TCP connection:
│  conn.Close()
│
└─ Log: "WebSocket connection closed for user {ownerID}"
```

---

### **Step 6: Response (if explicit closeConnection)**

**Location**: `/internal/handlers/thread.go:180`

```
IF triggered by explicit closeConnection message:

  Send response before closing:
  conn.WriteJSON(CloseConnectionResponse{
    action: "closeConnection",
    status: "success",
    message: "Connection closed successfully"
  })

  Then proceed with connection close

ELSE (if triggered by connection error):
  Skip response (connection already closed)
```

---

## Database Impact Summary

### **Valkey**

**NO WRITES** - All cleanup is in-memory only

**NO READS** - No database queries needed

### **NATS**

**Unsubscribe Operations**:
- Remove consumer from queue groups
- Stop receiving messages on subscribed subjects
- No persistent state changes

### **PostgreSQL**

**NO WRITES** - No archival of disconnection events

**Note**: Connection events are transient and not persisted

---

## Memory Impact

### **Resources Freed**

1. **ConnectionManager.connections[ownerID]**
   - Size: ~200 bytes per connection
   - Freed immediately

2. **WebSocketHandler.sessions[ownerID]**
   - Size: ~500 bytes per session
   - Freed immediately

3. **NotificationRouter.clients[clientID]**
   - Size: ~1KB per client (includes subscriptions and pending ACKs)
   - Freed immediately

4. **NATS Subscriptions**
   - N subscriptions (one per thread)
   - Each subscription has buffered messages
   - Freed on unsubscribe

**Total Memory Freed**: ~2KB + (N × subscription overhead)

---

## Performance Characteristics

**Total Cleanup Time**: ~1-5ms

1. **Notification unsubscribe**: ~1-2ms (N NATS operations)
2. **ConnectionManager cleanup**: ~100μs (map delete)
3. **Session cleanup**: ~100μs (map delete)
4. **WebSocket close**: ~1-2ms (TCP close)

**No blocking operations** - All cleanup is synchronous and fast

---

## Error Handling

### **Graceful Degradation**

1. **NATS unsubscribe fails**: Log warning, continue cleanup
2. **Client not in router**: Log warning, continue cleanup
3. **Connection already closed**: Skip close frame, continue cleanup
4. **Session not found**: Log warning, skip session cleanup

**Principle**: Best-effort cleanup, never fail on disconnect

---

## Connection Close Scenarios

### **Scenario 1: Explicit Client Disconnect**

```
Flow:
1. Client sends {action: "closeConnection"}
2. Server receives message
3. Server sends success response
4. Server performs cleanup
5. Server closes WebSocket

User Experience:
- Clean disconnect
- Confirmation received
- Resources freed immediately
```

### **Scenario 2: Network Failure**

```
Flow:
1. Network connection lost
2. conn.ReadJSON() returns error
3. Server detects disconnection
4. Server performs cleanup
5. WebSocket already closed

User Experience:
- No confirmation (connection lost)
- Server cleans up automatically
- Client can reconnect
```

### **Scenario 3: Server Shutdown**

```
Flow:
1. Server receives shutdown signal
2. Server iterates all active sessions
3. For each session:
   - Send close frame
   - Perform cleanup
   - Close connection
4. Server waits for all connections to close
5. Server exits

User Experience:
- Receives close frame with reason
- Can reconnect to another server instance
```

### **Scenario 4: Idle Timeout**

```
Flow:
1. No messages received for X minutes
2. Server timeout handler triggers
3. Server sends close frame (reason: timeout)
4. Server performs cleanup
5. Server closes connection

User Experience:
- Receives timeout notification
- Must reconnect to resume
```

---

## Reconnection Handling

### **Client Reconnection Flow**

```
After disconnect, client can reconnect:

1. Client calls connect action with same API key
2. Server creates new session (new clientID)
3. Server registers in ConnectionManager
4. Client re-subscribes to threads
5. Client resumes operations

State Preserved:
- Thread access (in Valkey)
- Thread data (in Valkey)
- Pending notifications (in NATS)

State Lost:
- Session ID (new ID assigned)
- In-memory subscriptions (must re-subscribe)
- Pending ACKs (must re-ACK)
```

---

## Monitoring & Metrics

### **Metrics to Track**

1. **Active Connections**: Decrement on disconnect
2. **Connection Duration**: Calculate from ConnectedAt
3. **Disconnect Reason**: Explicit, error, timeout, shutdown
4. **Cleanup Duration**: Time taken for full cleanup
5. **Failed Cleanups**: Count of cleanup errors

### **Logging**

```
INFO: User {ownerID} disconnected (reason: explicit)
INFO: Unsubscribed from {N} thread notifications
INFO: Removed client {clientID} from notification router
INFO: Connection cleanup completed in {duration}ms
```

---

## Security Considerations

### **Resource Cleanup**

1. **Memory leaks prevented**: All maps cleaned up
2. **NATS subscriptions closed**: No orphaned consumers
3. **WebSocket closed**: No dangling connections
4. **Session invalidated**: Cannot reuse old session

### **Access Revocation**

**NOT PERFORMED** on disconnect:
- Thread access remains in Valkey
- User can reconnect and resume
- Access must be explicitly revoked via separate action

**Rationale**: Disconnect is transient, access is persistent

---

## Comparison with Other Cases

### **Simplicity**

- **No database writes**: Purely in-memory cleanup
- **No async operations**: All synchronous
- **No validations**: Always succeeds (best-effort)
- **Fast execution**: ~1-5ms total

### **Contrast with connect (CASE 1)**

- **connect**: Adds resources, validates API key
- **closeConnection**: Removes resources, no validation

### **Idempotency**

- Multiple close attempts are safe
- Cleanup operations are idempotent
- No side effects if already closed

---

*This completes the detailed flow for CASE 6: closeConnection*
