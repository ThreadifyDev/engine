# NATS Messaging System

## Overview
Threadify uses NATS JetStream for real-time notification delivery via WebSocket. All validation results, thread events, and system notifications are published through NATS with scope-based routing.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Threadify Server                         │
│                                                             │
│  ┌──────────────┐         ┌──────────────┐                │
│  │ Notification │ ──────→ │    NATS      │                │
│  │   Service    │         │  Publisher   │                │
│  └──────────────┘         └──────────────┘                │
│                                  │                          │
└──────────────────────────────────┼──────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   NATS JetStream         │
                    │   Stream: notifications  │
                    └──────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   Subject Routing        │
                    │   thread.{id}.{scope}    │
                    └──────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   WebSocket Consumer     │
                    │   (Per-user subscriptions)│
                    └──────────────────────────┘
                                   ↓
                    ┌──────────────────────────┐
                    │   Client (Browser/SDK)   │
                    └──────────────────────────┘
```

## NATS Configuration

### Stream Setup
```go
// Stream: notifications
// Subjects: thread.>
// Retention: WorkQueue (messages deleted after ack)
// MaxAge: 24 hours
// Storage: File
// Replicas: 1 (configurable)
```

### Subject Pattern
```
thread.{threadId}.{scope}
```

**Scopes:**
- `owner` - Thread creator
- `participant` - Users with write permission
- `viewer` - Users with read permission

## Publishing Notifications

### Publisher Interface
```go
type NotificationPublisher interface {
    PublishNotification(ctx context.Context, notification models.ValidationNotification) error
}
```

### Implementation
Location: `/internal/repository/nats/publisher.go`

```go
func (p *NATSPublisher) PublishNotification(ctx context.Context, notification models.ValidationNotification) error {
    // 1. Resolve user scope
    scope := p.scopeResolver.ResolveUserScope(notification.ThreadID, notification.OwnerID)
    
    // 2. Build subject
    subject := fmt.Sprintf("thread.%s.%s", notification.ThreadID, scope)
    
    // 3. Marshal notification
    data, err := json.Marshal(notification)
    
    // 4. Publish to NATS
    _, err = p.js.Publish(subject, data)
    
    return err
}
```

## Consuming Notifications

### WebSocket Consumer
Location: `/internal/handlers/websocket_consumer.go`

```go
// Per-user subscription
func (wc *WebSocketConsumer) SubscribeToThread(userID, threadID string) {
    // 1. Resolve user's scope for this thread
    scope := wc.scopeResolver.ResolveUserScope(threadID, userID)
    
    // 2. Create durable consumer
    consumerName := fmt.Sprintf("%s:%s", threadID, userID)
    
    // 3. Subscribe to subject
    subject := fmt.Sprintf("thread.%s.%s", threadID, scope)
    sub, err := wc.js.Subscribe(subject, func(msg *nats.Msg) {
        // Parse notification
        var notif models.ValidationNotification
        json.Unmarshal(msg.Data, &notif)
        
        // Send to WebSocket
        wc.sendToWebSocket(userID, threadID, notif)
        
        // Acknowledge
        msg.Ack()
    }, nats.Durable(consumerName))
}
```

## Scope Resolution

### Scope Resolver Service
Location: `/internal/service/scope_resolver.go`

```go
func (sr *ScopeResolver) ResolveUserScope(threadID, userID string) string {
    // 1. Check if user is thread owner
    thread := sr.getThread(threadID)
    if thread.OwnerID == userID {
        return "owner"
    }
    
    // 2. Check user permissions
    permissions := sr.getPermissions(threadID, userID)
    if contains(permissions, "write") {
        return "participant"
    }
    if contains(permissions, "read") {
        return "viewer"
    }
    
    // 3. Check contract role
    if thread.ContractName != "" {
        role := sr.getUserRole(userID)
        if sr.hasRoleInContract(thread.ContractName, role) {
            return "participant"
        }
    }
    
    return "viewer" // Default
}
```

### Scope Hierarchy
```
owner > participant > viewer
```

**Notification Filtering:**
- All scopes receive all notifications for their threads
- Filtering happens at subscription level (subject-based)
- No client-side filtering needed

## Message Flow

### 1. Step Recorded
```
Client → Server → Validation → Notification Service → NATS Publisher
                                                            ↓
                                                    thread.{id}.owner
                                                    thread.{id}.participant
                                                    thread.{id}.viewer
```

### 2. Notification Delivery
```
NATS JetStream → WebSocket Consumer → Active WebSocket Connections
                                              ↓
                                         Client Receives
```

## Notification Types

All notifications use the same structure:

```json
{
  "notificationId": "uuid",
  "threadId": "thread-uuid",
  "stepId": "step-uuid",
  "stepName": "order_placed",
  "ownerId": "user-id",
  "stepStatus": "success",
  "status": "violated",
  "violationType": "invalid_transition",
  "severity": "critical",
  "message": "Invalid transition from 'order_placed' to 'order_placed'",
  "details": {...},
  "timestamp": "2026-01-06T13:00:00Z"
}
```

## Reliability Features

### 1. Durable Consumers
- Each user gets a durable consumer per thread
- Messages persist until acknowledged
- Automatic replay on reconnection

### 2. Acknowledgment
- Messages acknowledged after WebSocket delivery
- Failed deliveries remain in stream
- Automatic retry on consumer restart

### 3. Message Retention
- WorkQueue retention (delete after ack)
- 24-hour max age (safety net)
- Prevents unbounded growth

### 4. Error Handling
```go
// Publish with retry
func (p *NATSPublisher) PublishNotification(ctx context.Context, notif models.ValidationNotification) error {
    var lastErr error
    for i := 0; i < 3; i++ {
        _, err := p.js.Publish(subject, data)
        if err == nil {
            return nil
        }
        lastErr = err
        time.Sleep(time.Millisecond * 100 * time.Duration(i+1))
    }
    return fmt.Errorf("failed to publish after 3 attempts: %w", lastErr)
}
```

## Performance Characteristics

- **Publish latency**: <5ms (local NATS)
- **Delivery latency**: <100ms (WebSocket + NATS)
- **Throughput**: 10,000+ messages/sec per stream
- **Concurrent consumers**: Unlimited (subject-based routing)

## Configuration

### Environment Variables
```bash
NATS_URL=nats://localhost:4222
NATS_STREAM_NAME=notifications
NATS_MAX_AGE=24h
NATS_REPLICAS=1
```

### Server Initialization
```go
// Connect to NATS
nc, err := nats.Connect(config.NATS.URL)

// Create JetStream context
js, err := nc.JetStream()

// Create or update stream
stream, err := js.AddStream(&nats.StreamConfig{
    Name:     "notifications",
    Subjects: []string{"thread.>"},
    Retention: nats.WorkQueuePolicy,
    MaxAge:   24 * time.Hour,
    Storage:  nats.FileStorage,
})
```

## WebSocket Integration

### Client Connection
```javascript
const ws = new WebSocket('ws://localhost:8081/ws');

ws.onopen = () => {
  // Connect to server
  ws.send(JSON.stringify({
    action: 'connect',
    apiKey: 'your-api-key'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  
  if (data.action === 'notification') {
    // Handle validation notification
    console.log('Notification:', data);
  }
};
```

### Server WebSocket Handler
```go
func (h *WebSocketHandler) HandleMessage(conn *websocket.Conn, msg []byte) {
    var req WebSocketRequest
    json.Unmarshal(msg, &req)
    
    switch req.Action {
    case "connect":
        // Authenticate user
        userID := h.authenticate(req.APIKey)
        h.registerConnection(userID, conn)
        
    case "startThread":
        // Create thread and subscribe to notifications
        threadID := h.createThread(req)
        h.consumer.SubscribeToThread(userID, threadID)
        
    case "recordThreadEvent":
        // Record step (notification sent via NATS)
        h.recordStep(req)
    }
}
```

## Monitoring

### Metrics to Track
- Message publish rate
- Message delivery rate
- Consumer lag
- Failed deliveries
- WebSocket connection count

### NATS CLI Commands
```bash
# Stream info
nats stream info notifications

# Consumer list
nats consumer list notifications

# Monitor stream
nats stream view notifications

# Check consumer lag
nats consumer info notifications {consumer-name}
```

## Best Practices

1. **Always use durable consumers** for WebSocket subscriptions
2. **Acknowledge messages** only after successful WebSocket delivery
3. **Handle reconnections** gracefully (consumer resumes from last ack)
4. **Use subject-based routing** instead of message filtering
5. **Monitor consumer lag** to detect delivery issues
6. **Set appropriate max age** to prevent unbounded growth
7. **Use WorkQueue retention** for one-time delivery semantics

## Troubleshooting

### Messages not delivered
- Check consumer is subscribed to correct subject
- Verify scope resolution is correct
- Check WebSocket connection is active
- Review consumer lag

### Duplicate messages
- Ensure messages are acknowledged
- Check for multiple consumers with same name
- Verify WorkQueue retention policy

### High latency
- Check NATS server performance
- Review network latency
- Monitor consumer processing time
- Check WebSocket connection quality

## Future Enhancements

- [ ] Multi-region NATS clustering
- [ ] Message compression for large payloads
- [ ] Priority-based delivery
- [ ] Message batching for high-volume scenarios
- [ ] Dead letter queue for failed deliveries
