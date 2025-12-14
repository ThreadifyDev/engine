# Threadify SDK Updates

## Changes Made

### Thread Creation Support
The SDK has been updated to support thread creation matching the current server implementation.

### Key Changes

1. **Thread.start() now requires contractId**
   - `contractId` is now a required parameter (not optional)
   - Throws error if contractId is not provided
   - Returns a Promise that resolves to the threadId

2. **Fixed WebSocket Message Handling**
   - Updated `_onceResponse()` to work with Node.js WebSocket (ws library)
   - Fixed message parsing to use `.toString()` for Node.js buffers
   - Added proper cleanup of message listeners

3. **Improved Error Handling**
   - Better error messages for thread creation failures
   - Added debug logging for thread start operations

## Usage

### Basic Thread Creation

```javascript
import { Threadify } from './src/index.js';

// Connect to Threadify
const thread = await Threadify.connect(
  'your-api-key',
  'your-service-name',
  {
    url: 'ws://localhost:8081/threads'
  }
);

// Start a thread (contractId is required)
const threadId = await thread.start('your-contract-id', {
  environment: 'production',
  version: '1.0.0'
});

console.log('Thread ID:', threadId);
console.log('Contract ID:', thread.getContractId());

// Close connection when done
await thread.close();
```

### Testing

Run the simple test:
```bash
cd threadify-sdk
node test/simple-test.js
```

## Server Requirements

The server must be running with the following endpoints:
- WebSocket endpoint: `ws://localhost:8081/threads`
- Actions supported:
  - `connect` - Authenticate and establish connection
  - `startThread` - Create a new thread with a contract
  - `closeConnection` - Close the WebSocket connection

## Current Limitations

Based on the current server implementation, the SDK supports:
- ✅ Connection establishment
- ✅ Thread creation with contract ID
- ✅ Connection closing
- ⏳ Step recording (not yet implemented on server)
- ⏳ Event notifications (not yet implemented on server)

## Next Steps

Once the server implements additional features, the SDK will support:
- Recording thread events and step progress
- Real-time event notifications (onSuccess, onError, onViolation, onStepProgress)
- Thread state management
- Contract validation
