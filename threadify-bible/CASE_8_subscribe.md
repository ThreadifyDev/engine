# CASE 8: `subscribe` - Step-Level Notification Subscription

**Handler Entry Point**: `/internal/handlers/thread.go:187-190`

**Purpose**: Subscribe to specific step notifications within a thread, enabling fine-grained filtering of real-time events by step name and event type.

---

## Complete Flow Diagram

```
Client Request (subscribe)
    ↓
WebSocket Handler
    ↓
NotificationRouter.SubscribeToStep
    ↓
├─→ Validate subscription parameters
├─→ Create subscription filter
├─→ Store in client.Subscriptions map
└─→ Log subscription
```

---

## Request Structure

```json
{
  "action": "subscribe",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "stepName": "order_placed",
  "eventTypes": ["violation", "completed"]
}
```

---

## Subscription Filtering Levels

### **Level 1: Thread-Level (Automatic)**

```
Established during:
- startThread (creator)
- joinThread (participant)

Receives:
- ALL notifications for the thread
- No filtering by step or event type

Scope:
- Broad, thread-wide notifications
```

### **Level 2: Step-Level (This Case)**

```
Established via:
- subscribe action (explicit)

Receives:
- Notifications for specific step
- Filtered by event types

Scope:
- Narrow, step-specific notifications
```

---

## Detailed Flow

### **Step 1: WebSocket Handler**

**Location**: `/internal/handlers/thread.go:187-190`

```
Client sends WebSocket message:
{
  action: "subscribe",
  threadId: "550e8400-e29b-41d4-a716-446655440000",
  stepName: "order_placed",
  eventTypes: ["violation", "completed"]
}

Handler processing:
├─ conn.ReadJSON(&msg) → Receive message
├─ Extract action: msg["action"].(string) → "subscribe"
├─ Unmarshal into SubscribeRequest struct:
│  {
│    Action: "subscribe",
│    ThreadID: "550e8400-e29b-41d4-a716-446655440000",
│    StepName: "order_placed",
│    EventTypes: ["violation", "completed"]
│  }
│
└─ Call notificationRouter.SubscribeToStep(session.clientID, req)
```

---

### **Step 2: NotificationRouter - Subscription Setup**

**Location**: `/internal/notification/router.go` (conceptual)

```
NotificationRouter.SubscribeToStep(clientID, req):

┌─────────────────────────────────────────────────────────────────┐
│ 2.1: Validate Subscription Parameters                           │
└─────────────────────────────────────────────────────────────────┘

├─ Get client from router.clients[clientID]
│
├─ IF client == nil:
│  └─> Return error "Client not found"
│
├─ VALIDATION: Thread access
│  ├─ Check if req.ThreadID in client.ThreadIDs
│  └─ IF not found → Return error "Not subscribed to thread"
│
├─ VALIDATION: Step name
│  ├─ Check req.StepName != ""
│  └─ IF empty → Return error "Step name is required"
│
├─ VALIDATION: Event types
│  ├─ IF req.EventTypes == nil OR len(req.EventTypes) == 0:
│  │  └─> Default to ["all"]
│  │
│  └─ Validate each event type:
│     ├─ Valid types: "violation", "completed", "failed", "warning", "info", "all"
│     └─ IF invalid → Return error "Invalid event type: {type}"
│
└─ Continue with subscription


┌─────────────────────────────────────────────────────────────────┐
│ 2.2: Create Subscription Filter                                 │
└─────────────────────────────────────────────────────────────────┘

Build subscription object:
subscription = &ClientSubscription{
  SubscriptionID: uuid.New().String(),
  ThreadID: "550e8400-e29b-41d4-a716-446655440000",
  StepName: "order_placed",
  EventTypes: map[string]bool{
    "violation": true,
    "completed": true
  },
  CreatedAt: time.Now()
}


┌─────────────────────────────────────────────────────────────────┐
│ 2.3: Store Subscription                                         │
└─────────────────────────────────────────────────────────────────┘

├─ Lock client: client.mu.Lock()
│
├─ Generate subscription key:
│  key = threadID + ":" + stepName
│  Example: "550e8400-e29b-41d4-a716-446655440000:order_placed"
│
├─ Store in client's subscriptions map:
│  client.Subscriptions[key] = subscription
│
├─ Unlock: client.mu.Unlock()
│
└─ Log: "Client {clientID} subscribed to step {stepName} in thread {threadID}"


┌─────────────────────────────────────────────────────────────────┐
│ 2.4: Update Subscription Index (Optional Optimization)          │
└─────────────────────────────────────────────────────────────────┘

IF router maintains reverse index:
  ├─ Lock router: router.mu.Lock()
  │
  ├─ Add to index:
  │  router.stepSubscriptions[threadID][stepName] = append(..., clientID)
  │  └─> Enables fast lookup: "Which clients are subscribed to this step?"
  │
  └─ Unlock: router.mu.Unlock()
```

---

### **Step 3: Response to Client**

**Location**: `/internal/handlers/thread.go:190`

```
Send SubscribeResponse to client:
conn.WriteJSON(SubscribeResponse{
  action: "subscribe",
  status: "success",
  message: "Successfully subscribed to step notifications",
  subscriptionId: subscription.SubscriptionID,
  threadId: req.ThreadID,
  stepName: req.StepName,
  eventTypes: req.EventTypes
})
```

---

## Notification Filtering Flow

### **When Notification Arrives**

```
NotificationRouter.HandleNotification(notification):

FOR EACH client in router.clients:
  
  ├─ Check thread-level subscription:
  │  IF notification.ThreadID in client.ThreadIDs:
  │    └─> Send notification (broad filter)
  │
  └─ Check step-level subscription:
     ├─ Build key: notification.ThreadID + ":" + notification.StepName
     ├─ Get subscription: client.Subscriptions[key]
     │
     └─ IF subscription exists:
        ├─ Check event type filter:
        │  IF notification.EventType in subscription.EventTypes OR "all" in subscription.EventTypes:
        │    └─> Send notification (narrow filter)
        │
        └─ ELSE: Skip (filtered out)
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

**Note**: Subscriptions are session-specific and transient

---

## Memory Impact

### **Resources Allocated**

1. **client.Subscriptions[key]**
   - Size: ~300 bytes per subscription
   - Lifetime: Until unsubscribe or disconnect

2. **router.stepSubscriptions (if indexed)**
   - Size: ~100 bytes per client per step
   - Purpose: Fast reverse lookup

**Total Memory**: ~400 bytes per subscription

---

## Performance Characteristics

**Total Response Time**: ~100-300μs

1. **Validation**: ~10μs
2. **Subscription creation**: ~50μs
3. **Map insert**: ~10μs
4. **Response send**: ~100-300μs

**Filtering Overhead**: ~1-5μs per notification per client

---

## Use Cases

### **Use Case 1: Monitor Critical Steps**

```
Scenario: Monitor only violations for payment step

Request:
{
  action: "subscribe",
  threadId: "thread-123",
  stepName: "payment_processed",
  eventTypes: ["violation"]
}

Result:
- Receives only violation notifications for payment_processed
- Ignores completed, failed, info notifications
- Reduces notification noise
```

### **Use Case 2: Track Step Completion**

```
Scenario: Track when specific steps complete

Request:
{
  action: "subscribe",
  threadId: "thread-123",
  stepName: "order_shipped",
  eventTypes: ["completed"]
}

Result:
- Receives notification when order_shipped completes
- Can trigger UI update or downstream action
- Ignores other event types
```

### **Use Case 3: Multiple Step Monitoring**

```
Scenario: Monitor multiple steps in same thread

Requests:
1. {action: "subscribe", threadId: "thread-123", stepName: "order_placed", eventTypes: ["all"]}
2. {action: "subscribe", threadId: "thread-123", stepName: "payment_processed", eventTypes: ["all"]}
3. {action: "subscribe", threadId: "thread-123", stepName: "order_shipped", eventTypes: ["all"]}

Result:
- Receives notifications for all three steps
- Each subscription independent
- Can unsubscribe individually
```

### **Use Case 4: Violation-Only Monitoring**

```
Scenario: Alert system monitoring for violations

Request:
{
  action: "subscribe",
  threadId: "thread-123",
  stepName: "*",  // All steps (if supported)
  eventTypes: ["violation"]
}

Result:
- Receives only violation notifications
- For any step in the thread
- Ideal for alerting/monitoring systems
```

---

## Event Type Reference

### **Available Event Types**

1. **violation**: Contract violations (critical)
2. **completed**: Step completed successfully
3. **failed**: Step failed
4. **error**: Step encountered error
5. **warning**: Non-critical warnings
6. **info**: Informational notifications
7. **all**: Subscribe to all event types

### **Event Type Mapping**

```
Validation Result → Event Type:
- hasCriticalViolation=true → "violation"
- status="success" → "completed"
- status="failed" → "failed"
- status="error" → "error"
- hasWarnings=true → "warning"
- default → "info"
```

---

## Subscription Patterns

### **Pattern 1: Broad Monitoring**

```
Subscribe to all events for all steps:
- Thread-level subscription (automatic on join)
- No step-level subscriptions needed
- Highest notification volume
```

### **Pattern 2: Selective Monitoring**

```
Subscribe to specific steps and event types:
- Step-level subscriptions for key steps
- Filter by event type (e.g., violations only)
- Reduced notification volume
```

### **Pattern 3: Critical-Only Monitoring**

```
Subscribe only to violations:
- Step-level subscriptions with eventTypes: ["violation"]
- Minimal notification volume
- Ideal for alerting systems
```

---

## Subscription Lifecycle

### **Creation**

```
1. Client sends subscribe action
2. Server validates and creates subscription
3. Subscription stored in client.Subscriptions
4. Client receives confirmation
```

### **Active**

```
1. Notifications arrive at server
2. Server filters by subscription rules
3. Matching notifications sent to client
4. Client ACKs notifications
```

### **Removal**

```
1. Client sends unsubscribe action (CASE 9)
2. Server removes subscription
3. Client stops receiving filtered notifications
4. Thread-level subscription remains active
```

### **Cleanup on Disconnect**

```
1. Client disconnects
2. Server cleans up all subscriptions
3. Memory freed
4. No persistent state
```

---

## Error Scenarios

### **Error 1: Not Subscribed to Thread**

```
Cause: Client tries to subscribe to step in thread they haven't joined

Response:
{
  action: "subscribe",
  status: "error",
  message: "Not subscribed to thread. Join thread first."
}

Action: Client must call joinThread first
```

### **Error 2: Invalid Event Type**

```
Cause: Client specifies invalid event type

Response:
{
  action: "subscribe",
  status: "error",
  message: "Invalid event type: 'invalid_type'. Valid types: violation, completed, failed, warning, info, all"
}

Action: Client must use valid event type
```

### **Error 3: Duplicate Subscription**

```
Cause: Client subscribes to same step twice

Response:
{
  action: "subscribe",
  status: "success",
  message: "Subscription updated"
}

Action: Overwrites previous subscription (idempotent)
```

---

## Monitoring & Metrics

### **Key Metrics**

1. **subscriptionCount**: Total active subscriptions
2. **subscriptionsPerClient**: Average subscriptions per client
3. **filteredNotificationCount**: Notifications filtered out
4. **subscriptionCreateRate**: Subscriptions created per second

### **Logging**

```
INFO: Client {clientID} subscribed to step {stepName} in thread {threadID}
INFO: Subscription filter: eventTypes={eventTypes}
DEBUG: Notification filtered out (no matching subscription)
```

---

## Comparison with Thread-Level Subscription

### **Thread-Level (Automatic)**

- **Scope**: All steps in thread
- **Filter**: None (receives all)
- **Setup**: Automatic on join
- **Use Case**: General monitoring

### **Step-Level (This Case)**

- **Scope**: Specific step
- **Filter**: By event type
- **Setup**: Explicit subscribe action
- **Use Case**: Targeted monitoring

---

*This completes the detailed flow for CASE 8: subscribe*
