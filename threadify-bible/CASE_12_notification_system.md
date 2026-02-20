# CASE 12: Notification System (Owner-Based Push Model)

## Overview

The notification system delivers real-time validation notifications to client applications via WebSocket using a push-based model with owner-based shared NATS consumers. This design ensures **single delivery per owner** in HPA environments, preventing duplicate processing.

---

## Architecture

### Key Principle

**Server pushes notifications to clients via WebSocket. All sessions of the same owner share one NATS consumer.**

### Components

#### 1. NATS JetStream

**Stream Configuration:**
- **Name**: `NOTIFICATIONS`
- **Subjects**: `notifications.user.{ownerID}.{contract}.{stepName}`
  - With contract: `notifications.user.test-user.product_delivery.order_placed`
  - Without contract: `notifications.user.test-user.*.order_placed`
- **Retention**: WorkQueuePolicy, 3-day MaxAge
- **Storage**: File-based for durability

**Consumer Configuration (Owner-Based):**
- **Name**: `owner-{ownerID}` (e.g., `owner-test-user`)
- **Type**: Durable, shared across all sessions of the same owner
- **Created**: When first session of owner connects
- **Deleted**: When last session of owner disconnects
- **MaxAckPending**: Sum of all sessions' maxInFlight
- **AckWait**: 30 seconds
- **MaxDeliver**: 3 (then moves to DLQ)
- **FilterSubjects**: Union of all sessions' subscriptions

#### 2. Dead Letter Queue (DLQ)

**Stream Configuration:**
- **Name**: `NOTIFICATIONS_DLQ`
- **Purpose**: Store messages that failed after 3 delivery attempts
- **Retention**: 7 days, max 10,000 messages
- **Subjects**: `notifications.dlq.>`

#### 3. Server Components

**NotificationRouter:**
- Manages owner-based consumers
- One router goroutine per owner (not per session)
- Routes notifications to subscribed sessions using O(1) composite key lookup
- Random load balancing across subscribed sessions

**Data Structures:**
```go
type NotificationRouter struct {
    sessions          map[string]*Session            // sessionID -> session
    consumers         map[string]jetstream.Consumer  // ownerID -> consumer
    sessionsByOwner   map[string][]string            // ownerID -> []sessionID
    subscriptionIndex map[string]map[string][]string // ownerID -> "step@contract" -> []sessionID
    ownerContexts     map[string]context.Context     // ownerID -> context
    ownerCancels      map[string]context.CancelFunc  // ownerID -> cancel
}
```

---

## Message Flow

### 1. Session Connect

**Client → Server:**
```json
{
  "action": "connect",
  "apiKey": "test-api-key",
  "serviceName": "order-service",
  "maxInFlight": 20
}
```

**Server Actions:**
1. Authenticate client (get ownerID)
2. Create session object
3. Check if consumer exists for ownerID:
   - **If NO**: Create `owner-{ownerID}` consumer, start router goroutine
   - **If YES**: Add session to existing consumer, update MaxAckPending
4. Add session to `sessionsByOwner[ownerID]`

**Server → Client:**
```json
{
  "action": "connect",
  "status": "success",
  "ownerId": "test-user",
  "sessionId": "session-abc123"
}
```

**NATS Consumer Created:**
```
Name: owner-test-user
MaxAckPending: 20 (sum of all sessions)
FilterSubjects: [] (empty until subscriptions)
```

---

### 2. Subscribe to Notifications

**Client → Server:**
```json
{
  "action": "subscribe",
  "stepName": "order_placed",
  "contract": "product_delivery",
  "eventTypes": ["violation", "completed"]
}
```

**Server Actions:**
1. Store subscription in session
2. Build composite key: `"order_placed@product_delivery"`
3. Update `subscriptionIndex[ownerID]["order_placed@product_delivery"]` with sessionID
4. Update NATS consumer FilterSubjects (union of all sessions)

**Server → Client:**
```json
{
  "action": "subscribe",
  "status": "success",
  "stepName": "order_placed",
  "contract": "product_delivery"
}
```

**NATS Consumer Updated:**
```
FilterSubjects: [
  "notifications.user.test-user.product_delivery.order_placed"
]
```

---

### 3. Notification Published

**When a step completes, the server publishes a notification:**

**Publisher → NATS:**
```
Subject: notifications.user.test-user.product_delivery.order_placed
Payload: {
  "notificationId": "notif-123",
  "threadId": "thread-456",
  "stepId": "step-789",
  "stepName": "order_placed",
  "ownerId": "test-user",
  "contractName": "product_delivery",
  "stepStatus": "success",
  "status": "passed",
  "message": "Step completed successfully",
  "timestamp": "2026-01-19T18:00:00Z"
}
```

---

### 4. Notification Routing (Router Goroutine)

**Router Actions:**
1. Receive message from NATS consumer
2. Parse notification
3. Lookup subscribed sessions: `subscriptionIndex[ownerID]["order_placed@product_delivery"]`
4. If multiple sessions subscribed, pick random one (load balance)
5. Create ACK token: `base64(sequence:replySubject)`
6. Send to selected session's WebSocket

**Server → Client (WebSocket):**
```json
{
  "action": "notification",
  "ackToken": "NjokSlMuQUNLLk5PVElGSUNBVElPTlMub3duZXItdGVzdC11c2VyLjEuNi4y...",
  "notification": {
    "notificationId": "notif-123",
    "threadId": "thread-456",
    "stepId": "step-789",
    "stepName": "order_placed",
    "ownerId": "test-user",
    "contractName": "product_delivery",
    "stepStatus": "success",
    "status": "passed",
    "message": "Step completed successfully",
    "timestamp": "2026-01-19T18:00:00Z"
  }
}
```

**Key Point:** Only ONE session receives the notification, even if 100 pods are running!

---

### 5. Client ACK

**Client → Server:**
```json
{
  "action": "ack_notification",
  "notificationId": "notif-123",
  "ackToken": "NjokSlMuQUNLLk5PVElGSUNBVElPTlMub3duZXItdGVzdC11c2VyLjEuNi4y..."
}
```

**Server Actions:**
1. Decode ACK token to get sequence and reply subject
2. Send ACK to NATS via reply subject
3. NATS removes message from consumer

**Server → Client:**
```json
{
  "action": "ack_notification",
  "status": "success",
  "notificationId": "notif-123"
}
```

---

### 6. Session Disconnect

**Client disconnects (or connection lost)**

**Server Actions:**
1. Remove session from maps
2. Remove session from `subscriptionIndex`
3. Check if last session for owner:
   - **If YES**: Cancel owner context, stop router goroutine, delete NATS consumer
   - **If NO**: Update consumer MaxAckPending and FilterSubjects

**NATS Consumer Deleted (if last session):**
```
Deleted: owner-test-user
Router goroutine stopped
All maps cleaned up
```

---

## HPA Behavior

### Scenario: 3 Pods Running

**Setup:**
```
Pod A (session-1) → \
Pod B (session-2) → → Consumer "owner-test-user" (SHARED)
Pod C (session-3) → /

All 3 pods subscribed to "order_placed"
```

**Notification Published:**
```
1. NATS delivers to consumer "owner-test-user"
2. Router goroutine receives message
3. Router looks up: subscriptionIndex["owner-test-user"]["order_placed@*"]
4. Finds: [session-1, session-2, session-3]
5. Picks random: session-2
6. Sends to Pod B only
```

**Result:** ✅ Only Pod B processes the notification (no duplicates!)

---

## Subscription Patterns

### 1. Subscribe to All Contracts

**Client:**
```json
{
  "action": "subscribe",
  "stepName": "order_placed",
  "contract": "",
  "eventTypes": []
}
```

**Matches:**
- `notifications.user.test-user.*.order_placed`
- Receives notifications for ALL contracts

---

### 2. Subscribe to Specific Contract

**Client:**
```json
{
  "action": "subscribe",
  "stepName": "order_placed",
  "contract": "product_delivery",
  "eventTypes": []
}
```

**Matches:**
- `notifications.user.test-user.product_delivery.order_placed`
- Receives notifications ONLY for product_delivery contract

---

### 3. Subscribe to Specific Event Types

**Client:**
```json
{
  "action": "subscribe",
  "stepName": "payment_received",
  "contract": "",
  "eventTypes": ["violation"]
}
```

**Matches:**
- All contracts, but only violation events
- Server filters by event type before sending

---

## Error Handling

### 1. Failed Delivery (Client Offline)

**Scenario:** Client disconnects while processing notification

**Flow:**
1. Router sends to WebSocket → fails
2. Router calls `msg.Nak()` (negative acknowledgment)
3. NATS redelivers to consumer (up to 3 times)
4. Router picks another session (if available)
5. After 3 failures → moves to DLQ

---

### 2. No Subscribed Sessions

**Scenario:** Notification published but no sessions subscribed

**Flow:**
1. Router receives message
2. Lookup returns empty array
3. Router calls `msg.Ack()` (acknowledge and discard)
4. Message removed from queue

---

### 3. Consumer Cleanup Failure

**Scenario:** Server crashes before deleting consumer

**Flow:**
1. Consumer remains in NATS with InactiveThreshold=0
2. On server restart, consumer is orphaned
3. Manual cleanup required (or set InactiveThreshold)

**Prevention:** Set InactiveThreshold to auto-delete inactive consumers

---

## Performance Characteristics

### Latency

- **Notification publish to NATS**: <1ms
- **NATS to router goroutine**: <5ms
- **Router to WebSocket**: <5ms
- **Total end-to-end**: <10ms

### Throughput

- **Single consumer**: ~10,000 notifications/sec
- **Multiple owners**: Linear scaling (1 consumer per owner)

### Resource Usage

- **Goroutines**: 1 per owner (not per session)
- **NATS consumers**: 1 per owner (not per session)
- **Memory**: O(sessions + subscriptions)

---

## Monitoring & Metrics

### Key Metrics (Prometheus)

```promql
# Notifications published
rate(notifications_published_total[5m])

# Notifications sent to clients
rate(notifications_sent_total[5m])

# Notifications acknowledged
rate(notifications_acked_total[5m])

# Notifications moved to DLQ
rate(notifications_dlq_total[5m])

# Consumer count
nats_consumer_count{stream="NOTIFICATIONS"}

# Goroutine count
go_goroutines{job="threadify-server"}
```

### Alerts

1. **Duplicate Processing**: `sent / published > 1.1` (should be ~1.0)
2. **High DLQ Rate**: `dlq / published > 0.01` (should be <1%)
3. **Consumer Leak**: `consumer_count` growing unbounded
4. **Goroutine Leak**: `goroutines` growing unbounded

---

## Testing

### Unit Tests

```go
func TestOwnerBasedConsumer(t *testing.T) {
    // Test that multiple sessions share one consumer
}

func TestCompositeKeyLookup(t *testing.T) {
    // Test O(1) subscription lookup
}

func TestConsumerCleanup(t *testing.T) {
    // Test consumer deleted on last session disconnect
}
```

### Integration Tests

```bash
# Test single pod
npm run test:push

# Test HPA scenario (3 pods)
npm run test:hpa

# Test with custom configuration
NUM_CONSUMERS=5 NOTIFICATION_COUNT=10 npm run test:hpa
```

### Expected Results

```
✅ Expected: 10 notifications
✅ Actual: 10 notifications received
✅ Unique: 10 unique thread IDs
✅ Duplicates: 0

SUCCESS: No duplicate processing!
```

---

## Migration from Session-Based

### Before (Session-Based - BROKEN)

```
Consumer naming: session-{sessionID}
Goroutines: 1 per session
Result: Duplicate processing in HPA
```

### After (Owner-Based - FIXED)

```
Consumer naming: owner-{ownerID}
Goroutines: 1 per owner
Result: Single delivery in HPA
```

### Migration Steps

1. Deploy new code (owner-based)
2. Clients reconnect (creates owner-based consumers)
3. Old session-based consumers auto-deleted on disconnect
4. Monitor for duplicates (should be 0%)

---

## Security Considerations

### 1. Authorization

- Only owner can receive their own notifications
- API key authentication on connect
- OwnerID derived from authenticated user

### 2. Rate Limiting

- Max 100 consumers per owner
- Prevents resource exhaustion

### 3. ACK Token Security

- Opaque base64-encoded token
- Contains sequence and reply subject
- Cannot be forged or reused

---

## Future Enhancements

### Short Term

1. **Session takeover** - Reuse consumer on reconnect
2. **SDK auto-resubscribe** - Auto-resubscribe on reconnect
3. **Grafana dashboard** - Visualize metrics

### Long Term

1. **Priority queues** - Critical notifications first
2. **Notification batching** - Batch for high volume
3. **Multi-region** - NATS clusters across regions

---

## Related Cases

- **CASE 1**: `connect` - Session creation and authentication
- **CASE 6**: `closeConnection` - Session cleanup
- **CASE 7**: `ack_notification` - Stateless ACK mechanism
- **CASE 8**: `subscribe` - Subscription management
- **CASE 9**: `unsubscribe` - Unsubscribe from notifications

---

## References

- **Implementation**: `/threadify-go/internal/handlers/notification_router.go`
- **Design Doc**: `/threadify-bible/NOTIFICATION_SYSTEM_DESIGN.md`
- **Test Scripts**: `/test-consumer.js`, `/test-publisher.js`, `/test-hpa-scenario.sh`
- **Verification**: `/HPA_TEST_RESULTS.md`, `/CONSUMER_CLEANUP_VERIFICATION.md`
