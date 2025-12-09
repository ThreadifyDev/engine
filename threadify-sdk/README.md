# Threadify JavaScript SDK

A simple and elegant JavaScript SDK for connecting to Threadify Engine's WebSocket API.

## Installation

```bash
cd Threadify-sdk
npm install
```

## Quick Start

```javascript
import { Threadify } from './src/index.js';

// Connect to Threadify
const thread = await Threadify.connect('your-api-key', 'my-service');

// Start a thread
await thread.start('contract-id');

// Create a step and add context
const step = thread.step('payment_processing');
step.addContext({ amount: 100, currency: 'USD' });

// Execute the step
step.start();
// ... do work ...
step.complete({ transactionId: 'txn_123' });

// Close connection
await thread.close();
```

## API Reference

### `Threadify.connect(apiKey, serviceName, options)`

Connect to Threadify Engine and authenticate.

**Parameters:**
- `apiKey` (string, required): Your API key
- `serviceName` (string, optional): Service name for identification
- `options` (object, optional):
  - `url` (string): WebSocket URL (default: `ws://localhost:8080/threads`)
  - `ownerId` (string): Owner ID (auto-generated if not provided)
  - `subscribedEvents` (array): Events to subscribe to

**Returns:** `Promise<Thread>`

**Example:**
```javascript
const thread = await Threadify.connect(
  'api-key-123',
  'payment-service',
  {
    url: 'ws://localhost:8080/threads',
    subscribedEvents: ['onSuccess', 'onError']
  }
);
```

---

### `thread.start(contractId, metadata)`

Start a new thread on the server.

**Parameters:**
- `contractId` (string, optional): Contract ID to use
- `metadata` (object, optional): Additional metadata

**Returns:** `Promise<string>` - Thread ID

**Example:**
```javascript
const threadId = await thread.start('contract-v1', {
  environment: 'production',
  version: '1.0.0'
});
```

---

### `thread.step(stepName)`

Create a new step in the thread.

**Parameters:**
- `stepName` (string, required): Name of the step

**Returns:** `ThreadStep`

**Example:**
```javascript
const step = thread.step('validate_payment');
```

---

### `step.addContext(contextData)`

Add context data to the step.

**Parameters:**
- `contextData` (object, required): Key-value pairs to add

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.addContext({
  amount: 100.00,
  currency: 'USD',
  customerId: 'cust_123'
});
```

---

### `step.start()`

Mark the step as started and send event to server.

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.start();
```

---

### `step.complete(metadata)`

Mark the step as completed.

**Parameters:**
- `metadata` (object, optional): Completion metadata

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.complete({ transactionId: 'txn_456' });
```

---

### `step.fail(error, metadata)`

Mark the step as failed.

**Parameters:**
- `error` (Error|string, required): Error object or message
- `metadata` (object, optional): Failure metadata

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.fail('Payment declined', { reason: 'insufficient_funds' });
```

---

### `thread.on(eventType, handler)`

Subscribe to thread events.

**Parameters:**
- `eventType` (string): Event type (`onSuccess`, `onError`, `onViolation`, `onStepProgress`)
- `handler` (function): Event handler function

**Example:**
```javascript
thread.on('onSuccess', (data) => {
  console.log('Success:', data);
});
```

---

### `thread.close()`

Close the WebSocket connection.

**Returns:** `Promise<void>`

**Example:**
```javascript
await thread.close();
```

---

## Complete Example

```javascript
import { Threadify } from './src/index.js';

async function processPayment() {
  // Connect
  const thread = await Threadify.connect('api-key', 'payment-service');
  
  // Subscribe to events
  thread.on('onError', (data) => console.error('Error:', data));
  
  // Start thread
  await thread.start('payment-contract-v1');
  
  // Step 1: Initialize
  const initStep = thread.step('initialize');
  initStep.addContext({ amount: 100, currency: 'USD' });
  initStep.start();
  // ... processing ...
  initStep.complete();
  
  // Step 2: Validate
  const validateStep = thread.step('validate');
  validateStep.addContext({ checks: ['fraud', 'balance'] });
  validateStep.start();
  // ... validation ...
  validateStep.complete({ result: 'passed' });
  
  // Step 3: Process
  const processStep = thread.step('process');
  processStep.start();
  try {
    // ... payment processing ...
    processStep.complete({ chargeId: 'ch_123' });
  } catch (error) {
    processStep.fail(error);
  }
  
  // Close
  await thread.close();
}

processPayment();
```

---

## Testing

Run the example test:

```bash
npm test
```

Or run directly:

```bash
node test/example.js
```

Make sure the Threadify Engine server is running at `ws://localhost:8080/threads` before testing.

---

## Project Structure

```
Threadify-sdk/
├── src/
│   ├── index.js       # Main SDK entry point (Threadify class)
│   ├── Thread.js      # Thread class
│   └── ThreadStep.js  # ThreadStep class
├── test/
│   └── example.js     # Example usage and tests
├── package.json       # Package configuration
└── README.md          # This file
```

---

## License

MIT
