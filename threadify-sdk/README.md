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

### `connection.onViolation(stepName, handler)`

Register a global handler for validation violations on a specific step across all threads.

**Parameters:**
- `stepName` (string, required): Name of the step to listen for
- `handler` (function, required): Handler function that receives a `Notification` object

**Returns:** `Connection` (for chaining)

**Example:**
```javascript
connection.onViolation('payment_processing', (notification) => {
  console.error(`Violation: ${notification.message}`);
  console.error(`Details:`, notification.details);
  notification.ack(); // Acknowledge the notification
});
```

---

### `connection.onCompleted(stepName, handler)`

Register a global handler for step completions on a specific step across all threads.

**Parameters:**
- `stepName` (string, required): Name of the step to listen for
- `handler` (function, required): Handler function that receives a `Notification` object

**Returns:** `Connection` (for chaining)

**Example:**
```javascript
connection.onCompleted('payment_processing', (notification) => {
  console.log(`Payment completed for thread ${notification.threadId}`);
  notification.ack();
});
```

---

### `connection.onFailed(stepName, handler)`

Register a global handler for step failures on a specific step across all threads.

**Parameters:**
- `stepName` (string, required): Name of the step to listen for
- `handler` (function, required): Handler function that receives a `Notification` object

**Returns:** `Connection` (for chaining)

**Example:**
```javascript
connection.onFailed('payment_processing', (notification) => {
  console.error(`Payment failed: ${notification.message}`);
  notification.ack();
});
```

---

### `thread.waitFor(stepName, options)`

Wait for a notification for a specific step on this thread (blocking).

**Parameters:**
- `stepName` (string, required): Name of the step to wait for
- `options` (object, optional):
  - `timeout` (number): Timeout in milliseconds (default: 5000)
  - `statuses` (array): Only resolve for these statuses (e.g., `['success', 'failed']`)

**Returns:** `Promise<Notification>`

**Example:**
```javascript
// Execute step
await thread.step('payment_authorization')
  .context({ amount: 500 })
  .stop('success');

// Wait for validation notification
const result = await thread.waitFor('payment_authorization', {
  timeout: 10000,
  statuses: ['success', 'failed']
});

if (result.isViolated()) {
  throw new Error(`Validation failed: ${result.message}`);
}
```

---

### `notification.ack()`

Acknowledge a notification (mark as read/processed).

**Example:**
```javascript
connection.onViolation('payment_processing', (notification) => {
  console.error('Violation:', notification.message);
  notification.ack(); // Send ACK to server
});
```

---

### `notification` Helper Methods

**Validation Status:**
- `notification.isViolated()` - Returns `true` if validation failed
- `notification.isPassed()` - Returns `true` if validation passed

**Step Status:**
- `notification.isSuccess()` - Returns `true` if step succeeded
- `notification.isFailed()` - Returns `true` if step failed
- `notification.isError()` - Returns `true` if step had an error

**Severity:**
- `notification.isCritical()` - Returns `true` if severity is critical
- `notification.isWarning()` - Returns `true` if severity is warning
- `notification.isInfo()` - Returns `true` if severity is info

**Example:**
```javascript
connection.onViolation('payment_processing', (notification) => {
  if (notification.isCritical()) {
    // Alert ops team
    alertOpsTeam(notification);
  } else if (notification.isWarning()) {
    // Log warning
    console.warn(notification.message);
  }
  notification.ack();
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

## Complete Examples

### Example 1: Basic Workflow with Event Handlers

```javascript
import { Threadify } from './src/index.js';

async function processPayment() {
  // Connect
  const connection = await Threadify.connect('api-key', 'payment-service');
  
  // Set up global notification handlers
  connection.onViolation('payment_processing', (notification) => {
    console.error(`🚨 Violation: ${notification.message}`);
    if (notification.isCritical()) {
      // Alert ops team
      alertOpsTeam(notification);
    }
    notification.ack();
  });
  
  connection.onCompleted('payment_processing', (notification) => {
    console.log(`✅ Payment completed on thread ${notification.threadId}`);
    notification.ack();
  });
  
  // Start thread
  const thread = await connection.start('payment-contract-v1');
  
  // Execute steps
  await thread.step('initialize')
    .addContext({ amount: 100, currency: 'USD' })
    .stop('success');
  
  await thread.step('payment_processing')
    .addContext({ method: 'credit_card' })
    .stop('success');
  
  // Close
  await thread.close();
}

processPayment();
```

### Example 2: Using waitFor() for Synchronous Validation

```javascript
import { Threadify } from './src/index.js';

async function processOrderWithValidation() {
  const connection = await Threadify.connect('api-key', 'order-service');
  const thread = await connection.start('order-fulfillment');
  
  // Execute critical step
  await thread.step('payment_authorization')
    .addContext({ amount: 500, currency: 'USD' })
    .stop('success');
  
  // Wait for validation notification
  const authResult = await thread.waitFor('payment_authorization', {
    timeout: 10000,
    statuses: ['success', 'failed']
  });
  
  if (authResult.isViolated()) {
    console.error(`Validation failed: ${authResult.message}`);
    throw new Error('Payment authorization failed validation');
  }
  
  console.log('✅ Payment authorized, proceeding with order');
  
  // Continue with next steps
  await thread.step('ship_order')
    .addContext({ address: '123 Main St' })
    .stop('success');
  
  await thread.close();
}

processOrderWithValidation();
```

### Example 3: Multi-Thread Monitoring

```javascript
import { Threadify } from './src/index.js';

async function monitorMultipleOrders() {
  const connection = await Threadify.connect('api-key', 'fulfillment-service');
  
  // Global handlers monitor all threads
  connection.onViolation('payment_authorization', (notification) => {
    console.error(`🚨 Payment auth violation on thread ${notification.threadId}`);
    // Send alert to ops
    notification.ack();
  });
  
  connection.onCompleted('payment_authorization', (notification) => {
    console.log(`✅ Payment authorized on thread ${notification.threadId}`);
    // Update metrics
    notification.ack();
  });
  
  // Process multiple orders concurrently
  const orders = ['order1', 'order2', 'order3'];
  const threads = await Promise.all(
    orders.map(orderId => connection.start('order-contract'))
  );
  
  // Execute steps on all threads
  await Promise.all(
    threads.map(thread => 
      thread.step('payment_authorization')
        .addContext({ orderId: thread.threadId })
        .stop('success')
    )
  );
  
  // All notifications handled by global handlers
  await connection.close();
}

monitorMultipleOrders();
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
