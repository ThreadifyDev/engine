# CASE 7: `ack_notification` - Two-Phase Notification Acknowledgment

**Handler Entry Point**: `/internal/handlers/thread.go:182-185`

**Purpose**: Acknowledge receipt and processing of a notification, implementing the client-side of the two-phase ACK system for reliable notification delivery.

---

## Complete Flow Diagram

```
Client Request (ack_notification)
    ↓
WebSocket Handler
    ↓
NotificationRouter.AcknowledgeNotification
    ↓
├─→ Lookup pending notification
├─→ Remove from pendingAcks map
├─→ Update metrics
└─→ Log acknowledgment
```

---

## Two-Phase ACK System Overview

### **Phase 1: Server ACK (Immediate)**

```
NATS → Threadify Server
    ↓
Server ACKs NATS immediately
    ↓
Store in pendingClientAcks
    ↓
Send to specific client via sessionID
```

### **Phase 2: Client ACK (Delayed)** ← **THIS CASE**

```
Client receives notification
    ↓
Client processes notification
    ↓
Client sends ack_notification
    ↓
Server removes from pendingClientAcks
    ↓
Update metrics
```

---

## Request Structure

```json
{
  "action": "ack_notification",
  "notificationId": "notif-uuid-123"
}
```

---

## Detailed Flow

### **Step 1: WebSocket Handler**

**Location**: `/internal/handlers/thread.go:182-185`

```
Client sends WebSocket message:
{
  action: "ack_notification",
  notificationId: "notif-uuid-123"
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "ack_notification"
├─ Unmarshal into AckNotificationRequest struct:
│  {
│    Action: "ack_notification",
│    NotificationID: "notif-uuid-123"
│  }
│
└─ Call notificationRouter.AcknowledgeNotification(session.clientID, req.NotificationID)
```

---

### **Step 2: NotificationRouter - Acknowledgment Processing**

**Location**: `/internal/notification/router.go` (conceptual)

```
NotificationRouter.AcknowledgeNotification(clientID, notificationID):

┌─────────────────────────────────────────────────────────────────┐
│ 2.1: Lookup Pending Notification                                │
└─────────────────────────────────────────────────────────────────┘

├─ Get client from router.clients[clientID]
│
├─ IF client == nil:
│  └─> Return error "Client not found"
│
├─ Lock client: client.mu.Lock()
│
├─ Get pending notification:
│  pendingNotif := client.pendingAcks[notificationID]
│
├─ IF pendingNotif == nil:
│  ├─ Unlock: client.mu.Unlock()
│  └─> Return error "Notification not found or already acknowledged"
│
└─ Continue with acknowledgment


┌─────────────────────────────────────────────────────────────────┐
│ 2.2: Remove from Pending ACKs                                   │
└─────────────────────────────────────────────────────────────────┘

├─ Delete from map:
│  delete(client.pendingAcks, notificationID)
│  └─> Removes: &PendingNotification{
│       NotificationID: "notif-uuid-123",
│       ThreadID: "thread-uuid",
│       StepName: "order_placed",
│       SentAt: time.Time,
│       RetryCount: 0
│     }
│
├─ Unlock: client.mu.Unlock()
│
└─ Log: "Client {clientID} acknowledged notification {notificationID}"


┌─────────────────────────────────────────────────────────────────┐
│ 2.3: Update Metrics                                             │
└─────────────────────────────────────────────────────────────────┘

├─ Calculate acknowledgment latency:
│  latency = time.Now().Sub(pendingNotif.SentAt)
│  └─> Example: 150ms
│
├─ Update metrics:
│  ├─ notificationAckCount.Inc()
│  ├─ notificationAckLatency.Observe(latency.Seconds())
│  └─ pendingNotificationCount.Dec()
│
└─ Log metrics: "Notification acknowledged in {latency}ms"


┌─────────────────────────────────────────────────────────────────┐
│ 2.4: Optional Callback (if configured)                          │
└─────────────────────────────────────────────────────────────────┘

IF router.ackCallback != nil:
  ├─ Call router.ackCallback(notificationID, clientID, latency)
  └─> Can trigger custom logic (e.g., analytics, logging)
```

---

### **Step 3: Response to Client**

**Location**: `/internal/handlers/thread.go:185`

```
Send AckNotificationResponse to client:
conn.WriteJSON(AckNotificationResponse{
  action: "ack_notification",
  status: "success",
  message: "Notification acknowledged",
  notificationId: "notif-uuid-123"
})
```

---

## Database Impact Summary

### **Valkey**

**NO WRITES** - All state is in-memory only

**NO READS** - No database queries needed

### **NATS**

**NO OPERATIONS** - NATS already ACKed in Phase 1

### **PostgreSQL**

**NO WRITES** - Acknowledgments not persisted

**Note**: Only notification delivery is archived, not ACKs

---

## Memory Impact

### **Resources Freed**

1. **client.pendingAcks[notificationID]**
   - Size: ~500 bytes per pending notification
   - Freed immediately on ACK

2. **Metrics Updated**
   - pendingNotificationCount decremented
   - Memory for pending notification freed

---

## Performance Characteristics

**Total Response Time**: ~100-500μs

1. **Map lookup**: ~10μs
2. **Map delete**: ~10μs
3. **Metrics update**: ~50μs
4. **Response send**: ~100-500μs

**No blocking operations** - All in-memory

---

## Timeout Handling

### **Pending ACK Timeout Monitor**

**Background goroutine runs every 5 seconds**:

```
NotificationRouter.MonitorPendingAcks():

FOR EACH client in router.clients:
  FOR EACH notificationID, pendingNotif in client.pendingAcks:
    
    timeSinceSent = time.Now().Sub(pendingNotif.SentAt)
    
    IF timeSinceSent > 30 seconds:
      
      IF pendingNotif.RetryCount < maxRetries:
        ├─ Increment retry count
        ├─ Resend notification to client
        └─ Log: "Retrying notification {notificationID} (attempt {retryCount})"
      
      ELSE:
        ├─ Remove from pendingAcks
        ├─ Mark as failed
        ├─ Update metrics: notificationFailedCount.Inc()
        └─ Log: "Notification {notificationID} failed after {maxRetries} retries"
```

---

## Error Scenarios

### **Error 1: Notification Not Found**

```
Cause: Client ACKs notification that doesn't exist or was already ACKed

Response:
{
  action: "ack_notification",
  status: "error",
  message: "Notification not found or already acknowledged"
}

Action: Log warning, no retry needed
```

### **Error 2: Client Not Found**

```
Cause: Client disconnected before ACK received

Response: (no response, connection closed)

Action: 
- Pending notification cleaned up on disconnect
- No action needed
```

### **Error 3: Duplicate ACK**

```
Cause: Client sends ACK multiple times for same notification

Response:
{
  action: "ack_notification",
  status: "error",
  message: "Notification already acknowledged"
}

Action: Idempotent, no side effects
```

---

## Use Cases

### **Use Case 1: Normal Acknowledgment**

```
Flow:
1. Server sends notification to client
2. Client receives and processes notification
3. Client sends ack_notification
4. Server removes from pending
5. Metrics updated

Timeline:
T+0ms: Notification sent
T+150ms: Client ACK received
T+150ms: Pending notification removed

Result: Success, 150ms latency
```

### **Use Case 2: Delayed Acknowledgment**

```
Flow:
1. Server sends notification to client
2. Client processing takes 25 seconds
3. Client sends ack_notification
4. Server removes from pending (just before timeout)

Timeline:
T+0ms: Notification sent
T+25s: Client ACK received (5s before timeout)
T+25s: Pending notification removed

Result: Success, 25s latency (high but within timeout)
```

### **Use Case 3: Timeout and Retry**

```
Flow:
1. Server sends notification to client
2. Client doesn't ACK (network issue)
3. Timeout monitor detects (30s)
4. Server resends notification
5. Client ACKs on retry

Timeline:
T+0ms: Notification sent (attempt 1)
T+30s: Timeout detected
T+30s: Notification resent (attempt 2)
T+30.2s: Client ACK received
T+30.2s: Pending notification removed

Result: Success after retry
```

### **Use Case 4: Max Retries Exceeded**

```
Flow:
1. Server sends notification to client
2. Client never ACKs (disconnected)
3. Timeout monitor retries 3 times
4. All retries fail
5. Notification marked as failed

Timeline:
T+0ms: Notification sent (attempt 1)
T+30s: Retry 1
T+60s: Retry 2
T+90s: Retry 3
T+120s: Marked as failed

Result: Failed, client likely disconnected
```

---

## Metrics & Monitoring

### **Key Metrics**

1. **notificationAckCount**: Total ACKs received
2. **notificationAckLatency**: Time from send to ACK
3. **pendingNotificationCount**: Current pending ACKs
4. **notificationFailedCount**: Failed after max retries
5. **notificationRetryCount**: Total retry attempts

### **Alerting Thresholds**

- **High latency**: ACK latency > 10 seconds
- **High pending**: Pending count > 1000
- **High failure rate**: Failed count > 5% of sent

### **Logging**

```
INFO: Client {clientID} acknowledged notification {notificationID} in {latency}ms
WARN: Notification {notificationID} not acknowledged after 30s, retrying
ERROR: Notification {notificationID} failed after 3 retries
```

---

## Comparison with NATS ACK

### **NATS ACK (Phase 1)**

- **When**: Immediately on receive from NATS
- **Purpose**: Prevent redelivery to other server instances
- **Scope**: Server-level reliability
- **Latency**: <1ms

### **Client ACK (Phase 2)**

- **When**: After client processes notification
- **Purpose**: Confirm client received and processed
- **Scope**: Client-level reliability
- **Latency**: 50-500ms (typical)

### **Why Two Phases?**

1. **Exactly-once delivery**: NATS ensures one server gets it
2. **Client confirmation**: Server knows client processed it
3. **Retry capability**: Can resend if client doesn't ACK
4. **Metrics**: Track end-to-end delivery success

---

## Security Considerations

### **ACK Validation**

1. **Client ID check**: Only client who received notification can ACK
2. **Notification ID validation**: Must be valid UUID
3. **No replay attacks**: ACK is idempotent, duplicate ACKs harmless

### **Resource Limits**

1. **Max pending per client**: Prevent memory exhaustion
2. **Max retry attempts**: Prevent infinite retries
3. **Timeout enforcement**: Prevent indefinite pending state

---

## Implementation Status

**Current Status**: Partially implemented

**Implemented**:
- ✅ Basic ACK handling in WebSocket handler
- ✅ Pending notification tracking
- ✅ Metrics collection

**Not Yet Implemented**:
- ⏳ Timeout monitor with retry logic
- ⏳ Max retry enforcement
- ⏳ Failed notification handling
- ⏳ Session-based routing (sticky sessions)

**Planned**:
- Two-phase ACK system fully operational
- Retry with exponential backoff
- Dead letter queue for failed notifications

---

*This completes the detailed flow for CASE 7: ack_notification*
