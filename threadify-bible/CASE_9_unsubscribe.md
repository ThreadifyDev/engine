# CASE 9: `unsubscribe` - Remove Step-Level Subscription

**Handler Entry Point**: `/internal/handlers/thread.go:192-195`

**Purpose**: Remove a specific step-level subscription, stopping filtered notifications for that step while maintaining thread-level subscription.

---

## Complete Flow Diagram

```
Client Request (unsubscribe)
    ↓
WebSocket Handler
    ↓
NotificationRouter.UnsubscribeFromStep
    ↓
├─→ Validate unsubscribe parameters
├─→ Remove from client.Subscriptions map
├─→ Update reverse index (if exists)
└─→ Log unsubscription
```

---

## Request Structure

```json
{
  "action": "unsubscribe",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "stepName": "order_placed"
}
```

**OR** unsubscribe by subscription ID:

```json
{
  "action": "unsubscribe",
  "subscriptionId": "sub-uuid-123"
}
```

---

## Detailed Flow

### **Step 1: WebSocket Handler**

**Location**: `/internal/handlers/thread.go:192-195`

```
Client sends WebSocket message:
{
  action: "unsubscribe",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  stepName: "order_placed"
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "unsubscribe"
├─ Unmarshal into UnsubscribeRequest struct:
│  {
│    Action: "unsubscribe",
│    ThreadID: "550e8400-e29b-41d4-a716-446655440000",
│    StepName: "order_placed",
│    SubscriptionID: ""  // Optional alternative
│  }
│
└─ Call notificationRouter.UnsubscribeFromStep(session.clientID, req)
```

---

### **Step 2: NotificationRouter - Unsubscribe Processing**

**Location**: `/internal/notification/router.go` (conceptual)

```
NotificationRouter.UnsubscribeFromStep(clientID, req):

┌─────────────────────────────────────────────────────────────────┐
│ 2.1: Validate Unsubscribe Parameters                            │
└─────────────────────────────────────────────────────────────────┘

├─ Get client from router.clients[clientID]
│
├─ IF client == nil:
│  └─> Return error "Client not found"
│
├─ Determine unsubscribe method:
│  │
│  ├─ METHOD 1: By threadID + stepName
│  │  ├─ Validate: req.ThreadID != "" && req.StepName != ""
│  │  └─ Build key: req.ThreadID + ":" + req.StepName
│  │
│  └─ METHOD 2: By subscriptionID
│     ├─ Validate: req.SubscriptionID != ""
│     └─ Find subscription by ID in client.Subscriptions
│
└─ Continue with removal


┌─────────────────────────────────────────────────────────────────┐
│ 2.2: Remove Subscription                                        │
└─────────────────────────────────────────────────────────────────┘

├─ Lock client: client.mu.Lock()
│
├─ METHOD 1: Remove by key
│  ├─ key = threadID + ":" + stepName
│  ├─ Get subscription: subscription := client.Subscriptions[key]
│  ├─ IF subscription == nil:
│  │  ├─ Unlock: client.mu.Unlock()
│  │  └─> Return error "Subscription not found"
│  │
│  └─ Delete from map: delete(client.Subscriptions, key)
│
├─ METHOD 2: Remove by ID
│  ├─ FOR EACH key, sub in client.Subscriptions:
│  │  └─ IF sub.SubscriptionID == req.SubscriptionID:
│  │     ├─ Delete: delete(client.Subscriptions, key)
│  │     └─ Break
│  │
│  └─ IF not found:
│     ├─ Unlock: client.mu.Unlock()
│     └─> Return error "Subscription not found"
│
├─ Unlock: client.mu.Unlock()
│
└─ Log: "Client {clientID} unsubscribed from step {stepName} in thread {threadID}"


┌─────────────────────────────────────────────────────────────────┐
│ 2.3: Update Reverse Index (Optional Optimization)               │
└─────────────────────────────────────────────────────────────────┘

IF router maintains reverse index:
  ├─ Lock router: router.mu.Lock()
  │
  ├─ Remove from index:
  │  ├─ Get list: clients := router.stepSubscriptions[threadID][stepName]
  │  ├─ Remove clientID from list
  │  └─ IF list empty: delete(router.stepSubscriptions[threadID], stepName)
  │
  └─ Unlock: router.mu.Unlock()
```

---

### **Step 3: Response to Client**

**Location**: `/internal/handlers/thread.go:195`

```
Send UnsubscribeResponse to client:
conn.WriteJSON(UnsubscribeResponse{
  action: "unsubscribe",
  status: "success",
  message: "Successfully unsubscribed from step notifications",
  threadId: req.ThreadID,
  stepName: req.StepName
})
```

---

## Database Impact Summary

### **Valkey**

**NO WRITES** - All subscription state is in-memory only

**NO READS** - No database queries needed

### **NATS**

**NO OPERATIONS** - Subscriptions are server-side filters

### **PostgreSQL**

**NO WRITES** - Subscriptions not persisted

**Note**: Unsubscribe is purely in-memory operation

---

## Memory Impact

### **Resources Freed**

1. **client.Subscriptions[key]**
   - Size: ~300 bytes per subscription
   - Freed immediately on unsubscribe

2. **router.stepSubscriptions (if indexed)**
   - Size: ~100 bytes per client per step
   - Freed on unsubscribe

**Total Memory Freed**: ~400 bytes per subscription

---

## Performance Characteristics

**Total Response Time**: ~50-200μs

1. **Validation**: ~10μs
2. **Map lookup**: ~10μs
3. **Map delete**: ~10μs
4. **Response send**: ~50-200μs

**No blocking operations** - All in-memory

---

## Use Cases

### **Use Case 1: Stop Monitoring Specific Step**

```
Scenario: Stop receiving notifications for completed payment step

Request:
{
  action: "unsubscribe",
  threadId: "thread-123",
  stepName: "payment_processed"
}

Result:
- Stops receiving step-level notifications for payment_processed
- Thread-level notifications continue
- Can re-subscribe later if needed
```

### **Use Case 2: Cleanup After Step Completion**

```
Scenario: Unsubscribe after step completes successfully

Flow:
1. Subscribe to "order_shipped" with eventTypes: ["completed"]
2. Receive completion notification
3. Unsubscribe from "order_shipped"
4. No longer receive notifications for that step

Result:
- Automatic cleanup after goal achieved
- Reduces notification volume
- Frees memory
```

### **Use Case 3: Unsubscribe All Step Subscriptions**

```
Scenario: Remove all step-level subscriptions for a thread

Implementation:
FOR EACH subscription in client.Subscriptions:
  IF subscription.ThreadID == targetThreadID:
    Send unsubscribe request for subscription

Result:
- All step-level subscriptions removed
- Thread-level subscription remains
- Back to default notification behavior
```

### **Use Case 4: Unsubscribe by ID**

```
Scenario: Remove subscription using subscription ID from subscribe response

Request:
{
  action: "unsubscribe",
  subscriptionId: "sub-uuid-123"
}

Result:
- Removes specific subscription by ID
- Useful when tracking subscriptions client-side
- No need to remember threadID + stepName
```

---

## Unsubscribe Methods Comparison

### **Method 1: By ThreadID + StepName**

**Advantages**:
- Simple, intuitive
- No need to track subscription ID
- Direct mapping to business logic

**Disadvantages**:
- Requires both threadID and stepName
- Cannot unsubscribe if values unknown

**Use When**:
- Client knows thread and step
- Simple unsubscribe logic

### **Method 2: By Subscription ID**

**Advantages**:
- Single identifier
- Can track subscriptions client-side
- Supports complex subscription management

**Disadvantages**:
- Must store subscription ID from subscribe response
- Additional client-side state

**Use When**:
- Managing multiple subscriptions
- Need precise subscription control
- Building subscription management UI

---

## Error Scenarios

### **Error 1: Subscription Not Found**

```
Cause: Client tries to unsubscribe from non-existent subscription

Response:
{
  action: "unsubscribe",
  status: "error",
  message: "Subscription not found"
}

Action: Log warning, no retry needed (idempotent)
```

### **Error 2: Client Not Found**

```
Cause: Client disconnected before unsubscribe processed

Response: (no response, connection closed)

Action: 
- All subscriptions cleaned up on disconnect
- No action needed
```

### **Error 3: Missing Parameters**

```
Cause: Neither (threadID + stepName) nor subscriptionID provided

Response:
{
  action: "unsubscribe",
  status: "error",
  message: "Either (threadId + stepName) or subscriptionId must be provided"
}

Action: Client must provide valid parameters
```

### **Error 4: Duplicate Unsubscribe**

```
Cause: Client unsubscribes twice from same subscription

Response:
{
  action: "unsubscribe",
  status: "error",
  message: "Subscription not found"
}

Action: Idempotent, no side effects
```

---

## Subscription Lifecycle Review

### **1. Creation (CASE 8: subscribe)**

```
Client → subscribe → Server creates subscription → Confirmation
```

### **2. Active**

```
Notifications filtered → Matching sent to client → Client ACKs
```

### **3. Removal (CASE 9: unsubscribe)** ← **THIS CASE**

```
Client → unsubscribe → Server removes subscription → Confirmation
```

### **4. Automatic Cleanup**

```
Client disconnect → Server removes all subscriptions → Memory freed
```

---

## Notification Behavior After Unsubscribe

### **Before Unsubscribe**

```
Notifications for thread:
├─ Thread-level: ALL notifications
└─ Step-level: Filtered notifications for subscribed steps

Client receives:
- All thread notifications (broad)
- Filtered step notifications (narrow)
```

### **After Unsubscribe**

```
Notifications for thread:
├─ Thread-level: ALL notifications (unchanged)
└─ Step-level: No filtered notifications for unsubscribed step

Client receives:
- All thread notifications (broad)
- No step-level filtering for unsubscribed step
```

**Note**: Thread-level subscription is NOT affected by step-level unsubscribe

---

## Monitoring & Metrics

### **Key Metrics**

1. **unsubscribeCount**: Total unsubscribe operations
2. **subscriptionLifetime**: Time from subscribe to unsubscribe
3. **activeSubscriptionCount**: Current active subscriptions
4. **unsubscribeErrorRate**: Failed unsubscribe attempts

### **Logging**

```
INFO: Client {clientID} unsubscribed from step {stepName} in thread {threadID}
DEBUG: Subscription lifetime: {duration}ms
WARN: Unsubscribe failed: subscription not found
```

---

## Comparison with Other Cleanup Operations

### **unsubscribe (This Case)**

- **Scope**: Single step-level subscription
- **Impact**: Stops filtered notifications for one step
- **Thread Access**: Maintained
- **Use Case**: Fine-grained subscription management

### **closeConnection (CASE 6)**

- **Scope**: All subscriptions and connections
- **Impact**: Stops all notifications, disconnects
- **Thread Access**: Maintained (can reconnect)
- **Use Case**: Session termination

### **Leave Thread (Not Implemented)**

- **Scope**: All subscriptions for one thread
- **Impact**: Stops all notifications for thread
- **Thread Access**: Revoked
- **Use Case**: Exit thread completely

---

## Best Practices

### **Client-Side**

1. **Track subscription IDs**: Store IDs from subscribe responses
2. **Cleanup on completion**: Unsubscribe when monitoring goal achieved
3. **Handle errors gracefully**: Subscription not found is not critical
4. **Batch unsubscribes**: If removing multiple, batch requests

### **Server-Side**

1. **Idempotent operations**: Multiple unsubscribes safe
2. **Automatic cleanup**: Remove all on disconnect
3. **Memory management**: Free resources immediately
4. **Metrics tracking**: Monitor subscription lifecycle

---

## Security Considerations

### **Access Control**

1. **Client validation**: Only client who created subscription can remove it
2. **No cross-client unsubscribe**: Cannot unsubscribe other clients
3. **Thread access not required**: Can unsubscribe even if thread access revoked

### **Resource Management**

1. **Memory freed**: Prevents memory leaks
2. **No orphaned subscriptions**: Cleanup on disconnect
3. **Rate limiting**: Prevent unsubscribe spam (if needed)

---

## Implementation Notes

### **Simplicity**

- **No database operations**: Pure in-memory
- **No async operations**: Synchronous processing
- **No validations**: Minimal checks
- **Fast execution**: <200μs typical

### **Idempotency**

- Multiple unsubscribes safe
- No side effects if already unsubscribed
- Error response but no state corruption

### **Cleanup Guarantee**

- All subscriptions removed on disconnect
- No manual cleanup required
- Memory automatically freed

---

*This completes the detailed flow for CASE 9: unsubscribe*

---

# 🎉 ALL 9 WEBSOCKET CASES COMPLETE! 🎉

This concludes the complete documentation of all WebSocket handler cases in the ThreadifyEngine system.
