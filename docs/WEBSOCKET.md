# WebSocket API

## Overview
Threadify uses WebSocket for real-time, bidirectional communication between clients and the server. All thread operations and notifications are delivered via WebSocket.

## Connection

### Endpoint
```
ws://localhost:8081/ws
```

### Authentication
```javascript
const ws = new WebSocket('ws://localhost:8081/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    action: 'connect',
    apiKey: 'your-api-key-here'
  }));
};
```

## Message Format

All messages follow this structure:
```json
{
  "action": "actionName",
  ...additionalFields
}
```

## Client Actions

### 1. Connect
Authenticate and establish session.

```json
{
  "action": "connect",
  "apiKey": "your-api-key"
}
```

**Response:**
```json
{
  "action": "connect",
  "status": "success",
  "message": "Connected successfully",
  "userId": "user-uuid"
}
```

### 2. Start Thread
Create a new thread.

```json
{
  "action": "startThread",
  "contractName": "payment_flow:3",  // Optional, format: "name:version"
  "role": "merchant",                 // Required if using contract
  "refs": {
    "serviceName": "merchant-service",
    "orderId": "12345"
  }
}
```

**Response:**
```json
{
  "action": "startThread",
  "status": "success",
  "threadId": "thread-uuid",
  "message": "Thread started successfully"
}
```

### 3. Record Thread Event (Step)
Record a step in the thread.

```json
{
  "action": "recordThreadEvent",
  "threadId": "thread-uuid",
  "stepName": "order_placed",
  "status": "success",           // "success", "failed", or "error"
  "context": {
    "order_id": "12345",
    "customer_id": "67890",
    "total_amount": 99.99
  },
  "idempotencyKey": "optional-key",  // Auto-generated if not provided
  "startedAt": "2026-01-06T10:00:00Z",
  "finishedAt": "2026-01-06T10:00:05Z"
}
```

**Response:**
```json
{
  "action": "recordThreadEvent",
  "status": "success",
  "stepId": "step-uuid",
  "message": "Step Event recorded successfully"
}
```

### 4. Invite Party
Invite another user to the thread.

```json
{
  "action": "inviteParty",
  "threadId": "thread-uuid",
  "inviteeEmail": "user@example.com",
  "permissions": ["read", "write"],
  "role": "payment_processor"  // Optional, for contract workflows
}
```

**Response:**
```json
{
  "action": "inviteParty",
  "status": "success",
  "invitationToken": "jwt-token",
  "permissions": "read,write"
}
```

### 5. Join Thread
Join a thread using invitation token.

```json
{
  "action": "joinThread",
  "invitationToken": "jwt-token"
}
```

**Response:**
```json
{
  "action": "joinThread",
  "status": "success",
  "threadId": "thread-uuid",
  "permissions": "read,write",
  "message": "Successfully joined thread"
}
```

## Server Events

### Notification
Real-time validation results and thread events.

```json
{
  "action": "notification",
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
  "details": {
    "fromStep": "order_placed",
    "toStep": "order_placed",
    "allowedSteps": "payment_validation"
  },
  "timestamp": "2026-01-06T13:00:00Z"
}
```

### Error
Error responses for failed operations.

```json
{
  "action": "error",
  "status": "error",
  "message": "Error description",
  "code": "ERROR_CODE"
}
```

## Notification Delivery

### Scope-Based Routing
Notifications are delivered based on user's relationship to the thread:

- **owner**: Thread creator (receives all notifications)
- **participant**: Users with write permission
- **viewer**: Users with read-only permission

### Real-Time Delivery
- Notifications delivered via NATS JetStream
- Typical latency: <100ms
- Guaranteed delivery (durable consumers)
- Automatic reconnection handling

## Connection Management

### Heartbeat
Server sends periodic ping messages to keep connection alive.

### Reconnection
Clients should implement automatic reconnection with exponential backoff:

```javascript
let reconnectDelay = 1000;
const maxReconnectDelay = 30000;

function connect() {
  const ws = new WebSocket('ws://localhost:8081/ws');
  
  ws.onclose = () => {
    console.log(`Reconnecting in ${reconnectDelay}ms...`);
    setTimeout(connect, reconnectDelay);
    reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
  };
  
  ws.onopen = () => {
    reconnectDelay = 1000; // Reset delay on successful connection
    // Re-authenticate
    ws.send(JSON.stringify({
      action: 'connect',
      apiKey: localStorage.getItem('apiKey')
    }));
  };
}
```

### Missed Messages
- Durable NATS consumers ensure no messages are lost
- Messages delivered upon reconnection
- Automatic catch-up for offline periods

## Error Handling

### Common Errors

**Authentication Failed**
```json
{
  "action": "error",
  "status": "error",
  "message": "Invalid API key",
  "code": "AUTH_FAILED"
}
```

**Permission Denied**
```json
{
  "action": "error",
  "status": "error",
  "message": "You do not have permission to perform this action",
  "code": "PERMISSION_DENIED"
}
```

**Invalid Request**
```json
{
  "action": "error",
  "status": "error",
  "message": "Missing required field: threadId",
  "code": "INVALID_REQUEST"
}
```

**Thread Not Found**
```json
{
  "action": "error",
  "status": "error",
  "message": "Thread not found",
  "code": "THREAD_NOT_FOUND"
}
```

## Best Practices

1. **Always handle reconnection** - Network issues are inevitable
2. **Store API key securely** - Never hardcode in client code
3. **Validate responses** - Check `status` field before processing
4. **Handle notifications asynchronously** - Don't block UI on notification receipt
5. **Use idempotency keys** - Prevent duplicate step recordings
6. **Implement timeouts** - Don't wait indefinitely for responses
7. **Log errors** - Help with debugging and monitoring

## Example Client Implementation

```javascript
class ThreadifyClient {
  constructor(apiKey) {
    this.apiKey = apiKey;
    this.ws = null;
    this.handlers = {};
    this.reconnectDelay = 1000;
  }
  
  connect() {
    this.ws = new WebSocket('ws://localhost:8081/ws');
    
    this.ws.onopen = () => {
      this.send({ action: 'connect', apiKey: this.apiKey });
      this.reconnectDelay = 1000;
    };
    
    this.ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      const handler = this.handlers[data.action];
      if (handler) handler(data);
    };
    
    this.ws.onclose = () => {
      setTimeout(() => this.connect(), this.reconnectDelay);
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000);
    };
  }
  
  on(action, handler) {
    this.handlers[action] = handler;
  }
  
  send(message) {
    if (this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message));
    }
  }
  
  async startThread(contractName, role, refs) {
    return new Promise((resolve, reject) => {
      const handler = (data) => {
        if (data.status === 'success') {
          resolve(data);
        } else {
          reject(new Error(data.message));
        }
        delete this.handlers.startThread;
      };
      
      this.on('startThread', handler);
      this.send({ action: 'startThread', contractName, role, refs });
      
      setTimeout(() => reject(new Error('Timeout')), 5000);
    });
  }
  
  recordStep(threadId, stepName, status, context) {
    this.send({
      action: 'recordThreadEvent',
      threadId,
      stepName,
      status,
      context
    });
  }
}

// Usage
const client = new ThreadifyClient('your-api-key');
client.connect();

client.on('notification', (notif) => {
  console.log('Notification:', notif);
  if (notif.status === 'violated') {
    console.error('Violation:', notif.message);
  }
});

const thread = await client.startThread('payment_flow:3', 'merchant', {
  serviceName: 'merchant-service'
});

client.recordStep(thread.threadId, 'order_placed', 'success', {
  order_id: '12345',
  customer_id: '67890'
});
```

## Security Considerations

1. **Use WSS in production** - Encrypt WebSocket connections
2. **Validate API keys server-side** - Never trust client
3. **Implement rate limiting** - Prevent abuse
4. **Sanitize inputs** - Prevent injection attacks
5. **Use CORS properly** - Restrict origins in production
6. **Rotate API keys** - Implement key rotation policy
7. **Monitor connections** - Detect suspicious activity

## Performance

- **Max concurrent connections**: 10,000+ per server instance
- **Message throughput**: 50,000+ messages/second
- **Average latency**: <10ms for local operations
- **Notification delivery**: <100ms end-to-end

## Monitoring

Track these metrics:
- Active WebSocket connections
- Messages sent/received per second
- Error rate
- Average message latency
- Reconnection rate
- Notification delivery time
