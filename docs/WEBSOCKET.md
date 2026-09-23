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

### 2. Create or Resume Thread

Resolve an application-supplied key to one thread within the authenticated
company. This is the WebSocket operation behind all SDK `thread` / `Thread`
methods. Concurrent calls resolve atomically to the same internal ID.

```json
{
  "action": "thread",
  "threadKey": "order:12345",
  "label": "Order 12345",
  "contractName": "payment_flow:3",
  "serviceName": "merchant-service",
  "role": "merchant",
  "tags": ["payments", "high-value"],
  "refs": {
    "orderId": "12345"
  }
}
```

Only `action` and `threadKey` are required. Keys are trimmed, nonblank strings of
at most 1024 UTF-8 bytes. SDK option `contract` maps to wire field `contractName`.
The other fields supply creation defaults. Labels, refs, and tags are preserved
on resume; a supplied conflicting contract or version is rejected.

**Response:**
```json
{
  "action": "thread",
  "status": "success",
  "threadId": "thread-uuid",
  "threadKey": "order:12345",
  "label": "Order 12345",
  "contractId": "contract-uuid",
  "contractName": "payment_flow",
  "contractVersion": 3,
  "tags": ["payments", "high-value"],
  "refs": {"orderId": "12345"}
}
```

Later requests omit the creation defaults and load the stored contract/version:

```json
{
  "action": "thread",
  "threadKey": "order:12345"
}
```

An unknown key without a contract creates a free-form thread. Initialize a
contracted run before workers or telemetry report its steps. A free-form thread
cannot acquire a contract on resume. Normal write permissions apply, and closed
threads reject resolution and further writes; the key is never reassigned.

**Compatibility:** The `startThread` action still creates unkeyed threads for
older SDK clients. New integrations should use `thread` and an application key.

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
Invite another party to join the thread.

```json
{
  "action": "inviteParty",
  "role": "payment_processor",
  "accessLevel": "external",     // Optional: "external" (default), "observer", "participant"
  "expiresIn": "24h"             // Optional, default: "24h"
}
```

**Response:**
```json
{
  "action": "inviteParty",
  "status": "success",
  "threadToken": "jwt-token",
  "role": "payment_processor",
  "accessLevel": "external",
  "expiresAt": 1736769600
}
```

### 5. Join Thread
Join a thread using invitation token or direct thread ID.

**Token-based join:**
```json
{
  "action": "joinThread",
  "threadToken": "jwt-token"
}
```

**Direct join (same company):**
```json
{
  "action": "joinThread",
  "threadId": "thread-uuid",
  "role": "payment_processor"
}
```

**Response:**
```json
{
  "action": "joinThread",
  "status": "success",
  "threadId": "thread-uuid",
  "role": "payment_processor",
  "accessLevel": "external",
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
    this.handlers = new Map();
    this.pending = new Map();
    this.connecting = null;
  }

  connect() {
    if (this.connecting) return this.connecting;
    this.connecting = new Promise((resolve, reject) => {
      const ws = this.ws = new WebSocket('ws://localhost:8081/threads');
      const timer = setTimeout(() => {
        reject(new Error('Connection timed out'));
        ws.close();
      }, 5000);
      ws.onopen = async () => {
        try {
          await this.request({ action: 'connect', apiKey: this.apiKey });
          clearTimeout(timer);
          resolve();
        } catch (error) {
          clearTimeout(timer);
          reject(error);
          ws.close();
        }
      };
      ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        const pending = this.pending.get(data.requestId);
        if (pending) {
          this.pending.delete(data.requestId);
          clearTimeout(pending.timer);
          if (data.status === 'success') pending.resolve(data);
          else pending.reject(new Error(data.message || 'Request failed'));
        } else {
          this.handlers.get(data.action)?.(data);
        }
      };
      ws.onerror = () => ws.close();
      ws.onclose = () => {
        clearTimeout(timer);
        const error = new Error('Connection closed; pending mutation outcomes may be unknown');
        reject(error);
        for (const pending of this.pending.values()) {
          clearTimeout(pending.timer);
          pending.reject(error);
        }
        this.pending.clear();
        this.connecting = null;
      };
    });
    return this.connecting;
  }

  on(action, handler) {
    this.handlers.set(action, handler);
  }

  request(message) {
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState !== WebSocket.OPEN) {
        reject(new Error('Connection is not open'));
        return;
      }
      const requestId = crypto.randomUUID();
      const timer = setTimeout(() => {
        this.pending.delete(requestId);
        reject(new Error('Request timed out; mutation outcome may be unknown'));
      }, 5000);
      this.pending.set(requestId, { resolve, reject, timer });
      try {
        this.ws.send(JSON.stringify({ ...message, requestId }));
      } catch (error) {
        clearTimeout(timer);
        this.pending.delete(requestId);
        reject(error);
      }
    });
  }

  async thread(threadKey, { label, contract, role, refs, tags, serviceName } = {}) {
    await this.connect();
    return this.request({ action: 'thread', threadKey, label, contractName: contract, role, refs, tags, serviceName });
  }

  async recordStep(threadId, stepName, status, context) {
    await this.connect();
    return this.request({ action: 'recordThreadEvent', threadId, stepName, status, context });
  }
}

// Usage
const client = new ThreadifyClient('your-api-key');
await client.connect();

client.on('notification', (notif) => {
  console.log('Notification:', notif);
  if (notif.status === 'violated') {
    console.error('Violation:', notif.message);
  }
});

const thread = await client.thread('order:12345', {
  label: 'Order 12345',
  contract: 'payment_flow:3',
  role: 'merchant',
  serviceName: 'merchant-service',
  refs: { orderId: '12345' },
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
