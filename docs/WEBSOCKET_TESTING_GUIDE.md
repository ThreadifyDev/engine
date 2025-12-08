# WebSocket Testing Guide for VSCode

## Quick Start

Your WebSocket API is now documented and ready to test! Here are the available testing methods:

---

## Method 1: HTML Test Client (Easiest)

### Setup
1. Open `websocket-test-client.html` in your browser
2. The server should be running at `http://localhost:8080`

### Usage
1. Click **Connect** - automatically authenticates
2. Use **Quick: Start Thread** to create a thread
3. Use **Quick: Record Event** to log events
4. Watch the message log for real-time responses

**Features:**
- ✅ Visual interface
- ✅ Pre-filled templates
- ✅ Auto-saves threadId
- ✅ Real-time message log
- ✅ No installation required

---

## Method 2: REST Client Extension (VSCode)

### Setup
1. Install the **REST Client** extension:
   ```
   ext install humao.rest-client
   ```
2. Open `websocket-tests.http` in VSCode

### Usage
1. Click "Send Request" above any `WEBSOCKET` line
2. Messages are sent sequentially
3. View responses in the output panel

**Features:**
- ✅ Test directly in VSCode
- ✅ Multiple test scenarios included
- ✅ Easy to modify and save tests
- ✅ Variables support

---

## Method 3: Node.js Script

### Setup
```bash
npm install ws
```

### Usage
```bash
node websocket-test.js
```

**Features:**
- ✅ Automated test suite
- ✅ Color-coded output
- ✅ Tests all endpoints
- ✅ Error case testing

---

## Method 4: Command Line Tools

### Using wscat
```bash
# Install
npm install -g wscat

# Connect
wscat -c ws://localhost:8080/threads

# Send messages (paste JSON)
{"action":"connect","apiKey":"test-key","ownerId":"user-123","subscribedEvents":[]}
```

### Using websocat (macOS)
```bash
# Install
brew install websocat

# Connect
websocat ws://localhost:8080/threads

# Send messages (paste JSON)
{"action":"connect","apiKey":"test-key","ownerId":"user-123","subscribedEvents":[]}
```

---

## Method 5: Browser Console

### Setup
1. Open browser console (F12)
2. Paste the following code:

```javascript
const ws = new WebSocket('ws://localhost:8080/threads');

ws.onopen = () => {
  console.log('✅ Connected');
  
  // Connect
  ws.send(JSON.stringify({
    action: "connect",
    apiKey: "test-key",
    ownerId: "user-123",
    subscribedEvents: ["onSuccess", "onError"]
  }));
};

ws.onmessage = (event) => {
  console.log('📥 Received:', JSON.parse(event.data));
};

ws.onerror = (error) => {
  console.error('❌ Error:', error);
};

// Helper function to send messages
function send(message) {
  ws.send(JSON.stringify(message));
  console.log('📤 Sent:', message);
}

// Example: Start a thread
send({
  action: "startThread",
  contractId: "contract-123"
});
```

---

## Recommended VSCode Extensions

### 1. REST Client
- **ID**: `humao.rest-client`
- **Purpose**: Test WebSocket directly in `.http` files
- **Install**: `ext install humao.rest-client`

### 2. Thunder Client
- **ID**: `rangav.vscode-thunder-client`
- **Purpose**: Alternative HTTP/WebSocket client with UI
- **Install**: `ext install rangav.vscode-thunder-client`

### 3. WebSocket King
- **ID**: `ramonitor.websocket-king`
- **Purpose**: Dedicated WebSocket testing tool
- **Install**: `ext install ramonitor.websocket-king`

---

## Testing Workflow

### Basic Flow
```
1. Connect → 2. Start Thread → 3. Record Events → 4. Close
```

### Example Test Sequence

**1. Connect**
```json
{
  "action": "connect",
  "apiKey": "your-api-key",
  "ownerId": "user-123",
  "subscribedEvents": ["onSuccess", "onError"]
}
```

**2. Start Thread**
```json
{
  "action": "startThread",
  "contractId": "contract-456"
}
```
*Save the returned `threadId`*

**3. Record Event**
```json
{
  "action": "recordThreadEvent",
  "threadId": "550e8400-e29b-41d4-a716-446655440000",
  "status": "completed",
  "finishedAt": "2025-12-08T19:00:00Z"
}
```

**4. Close**
```json
{
  "action": "closeConnection"
}
```

---

## Troubleshooting

### Connection Refused
- ✅ Ensure server is running: `./gradlew run`
- ✅ Check server is on port 8080
- ✅ Verify WebSocket endpoint: `ws://localhost:8080/threads`

### Authentication Errors
- ✅ Send `connect` message first
- ✅ Provide valid `apiKey` and `ownerId`
- ✅ Check response for error messages

### Message Not Received
- ✅ Verify JSON is valid
- ✅ Check `action` field is correct
- ✅ Ensure connection is still open

### Thread ID Not Found
- ✅ Start a thread first with `startThread`
- ✅ Copy the `threadId` from the response
- ✅ Use exact threadId in subsequent messages

---

## Files Reference

| File | Purpose |
|------|---------|
| `docs/WEBSOCKET_API.md` | Complete API documentation |
| `websocket-tests.http` | REST Client test scenarios |
| `websocket-test-client.html` | Browser-based test client |
| `websocket-test.js` | Node.js automated test script |

---

## Next Steps

1. **Start the server**: `./gradlew run`
2. **Choose a testing method** from above
3. **Open the documentation**: `docs/WEBSOCKET_API.md`
4. **Run your first test**: Connect → Start Thread → Record Event

Happy testing! 🚀
