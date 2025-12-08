# WebSocket API Documentation

## Overview

The ThreadifyEngine WebSocket API provides real-time communication for thread management and event tracking. The WebSocket endpoint is available at:

```
ws://localhost:8080/threads
```

## Connection Flow

1. **Connect** - Authenticate and subscribe to events
2. **Start Thread** - Create a new thread (with/without contract)
3. **Record Events** - Track thread events as they occur
4. **Close Connection** - Gracefully disconnect

## Message Format

All messages are JSON-formatted with an `action` field that determines the message type.

### Request Format
```json
{
  "action": "actionName",
  ...additional fields
}
```

### Response Format
```json
{
  "action": "actionName",
  "status": "success|error",
  "message": "Description",
  ...additional fields
}
```

---

## API Actions

### 1. Connect

Authenticate the WebSocket connection and subscribe to events.

**Request:**
```json
{
  "action": "connect",
  "apiKey": "your-api-key",
  "ownerId": "user-123",
  "subscribedEvents": ["onSuccess", "onError", "onViolation", "onStepProgress"]
}
```

**Fields:**
- `action` (string, required): Must be "connect"
- `apiKey` (string, required): Your API key for authentication
- `ownerId` (string, required): Unique identifier for the owner/user
- `subscribedEvents` (array, optional): List of events to receive notifications for
  - `onSuccess` - Thread completed successfully
  - `onError` - Error occurred during thread execution
  - `onViolation` - Contract violation detected
  - `onStepProgress` - Step progress updates

**Success Response:**
```json
{
  "action": "connect",
  "status": "success",
  "message": "Connected successfully",
  "ownerId": "user-123",
  "subscribedEvents": ["onSuccess", "onError", "onViolation", "onStepProgress"]
}
```

**Error Response:**
```json
{
  "action": "connect",
  "status": "error",
  "message": "API key is required"
}
```

---

### 2. Start Thread

Start a new thread with or without a contract.

**Request:**
```json
{
  "action": "startThread",
  "contractId": "contract-456",
  "metadata": {
    "description": "Payment processing thread",
    "priority": "high"
  }
}
```

**Fields:**
- `action` (string, required): Must be "startThread"
- `contractId` (string, optional): ID of the contract to use
- `metadata` (object, optional): Additional metadata for the thread

**Success Response:**
```json
{
  "action": "startThread",
  "status": "success",
  "message": "Thread started successfully",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "contractId": "contract-456"
}
```

**Error Response:**
```json
{
  "action": "startThread",
  "status": "error",
  "message": "Not authenticated. Please connect first."
}
```

---

### 3. Record Thread Event

Record an event for a thread (e.g., step started, step completed, error occurred).

**Request:**
```json
{
  "action": "recordThreadEvent",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "startedAt": "2025-12-08T13:00:00Z",
  "finishedAt": "2025-12-08T13:05:00Z",
  "context": {
    "step": "payment_initiated",
    "amount": "100.00"
  },
  "status": "completed",
  "metadata": {
    "transactionId": "txn-789"
  }
}
```

**Fields:**
- `action` (string, required): Must be "recordThreadEvent"
- `threadId` (string, required): ID of the thread
- `startedAt` (string, optional): ISO 8601 timestamp when event started
- `finishedAt` (string, optional): ISO 8601 timestamp when event finished
- `context` (object, optional): Contextual information about the event
- `status` (string, optional): Status of the event (e.g., "started", "completed", "failed")
- `metadata` (object, optional): Additional metadata

**Success Response:**
```json
{
  "action": "recordThreadEvent",
  "status": "success",
  "message": "Event recorded successfully",
  "threadId": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Error Response:**
```json
{
  "action": "recordThreadEvent",
  "status": "error",
  "message": "Thread ID is required"
}
```

---

### 4. Close Connection

Gracefully close the WebSocket connection.

**Request:**
```json
{
  "action": "closeConnection"
}
```

**Fields:**
- `action` (string, required): Must be "closeConnection"

**Success Response:**
```json
{
  "action": "closeConnection",
  "status": "success",
  "message": "Connection closed successfully"
}
```

---

## Error Handling

### Generic Error Response
```json
{
  "action": "error",
  "status": "error",
  "message": "Error description",
  "details": "Additional error details"
}
```

### Common Error Scenarios

1. **Not Authenticated**
   - Occurs when trying to perform actions before connecting
   - Solution: Send a `connect` message first

2. **Invalid Message Format**
   - Occurs when JSON is malformed or missing required fields
   - Solution: Validate JSON structure before sending

3. **Unknown Action**
   - Occurs when `action` field contains an invalid value
   - Solution: Use one of: "connect", "startThread", "recordThreadEvent", "closeConnection"

---

## Example Flow

```javascript
// 1. Connect
{
  "action": "connect",
  "apiKey": "my-secret-key",
  "ownerId": "user-123",
  "subscribedEvents": ["onSuccess", "onError"]
}

// 2. Start a thread
{
  "action": "startThread",
  "contractId": "contract-456"
}

// 3. Record events as they happen
{
  "action": "recordThreadEvent",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "started",
  "context": {"step": "payment_initiated"}
}

{
  "action": "recordThreadEvent",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "finishedAt": "2025-12-08T13:05:00Z"
}

// 4. Close connection
{
  "action": "closeConnection"
}
```

---

## Testing Tools

### VSCode Extensions
- **REST Client** - Test WebSocket connections directly in VSCode
- **Thunder Client** - Alternative HTTP/WebSocket client
- **WebSocket King** - Dedicated WebSocket testing tool

### Command Line
```bash
# Using wscat
npm install -g wscat
wscat -c ws://localhost:8080/threads

# Using websocat
brew install websocat
websocat ws://localhost:8080/threads
```

### Browser
Open browser console and use:
```javascript
const ws = new WebSocket('ws://localhost:8080/threads');
ws.onopen = () => {
  ws.send(JSON.stringify({
    action: "connect",
    apiKey: "test-key",
    ownerId: "user-123",
    subscribedEvents: ["onSuccess"]
  }));
};
ws.onmessage = (event) => console.log('Received:', event.data);
```

---

## Rate Limits & Best Practices

1. **Connection Limits**: One active connection per `ownerId`
2. **Message Size**: Maximum 1MB per message
3. **Reconnection**: Implement exponential backoff for reconnections
4. **Heartbeat**: Connection times out after 15 seconds of inactivity
5. **Error Handling**: Always handle connection errors and implement retry logic

---

## Support

For issues or questions, please refer to the main README or open an issue in the repository.
