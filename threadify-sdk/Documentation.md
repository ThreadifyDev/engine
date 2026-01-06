# Threadify SDK Documentation

## Overview

The Threadify SDK is a JavaScript/Node.js client library for interacting with the Threadify Engine. It provides a fluent, promise-based API for managing distributed workflows, thread orchestration, and step execution with built-in validation, idempotency, and real-time WebSocket communication.

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Concepts](#core-concepts)
- [API Reference](#api-reference)
  - [Threadify Class](#threadify-class)
  - [Connection Class](#connection-class)
  - [ThreadInstance Class](#threadinstance-class)
  - [ThreadStep Class](#threadstep-class)
- [Advanced Usage](#advanced-usage)
- [Error Handling](#error-handling)
- [Examples](#examples)

---

## Installation

```bash
npm install threadify-sdk
```

Or if developing locally:

```bash
cd threadify-sdk
npm install
npm link
```

---

## Quick Start

### Basic Non-Contract Workflow

```javascript
import { Threadify } from 'threadify-sdk';

// Connect to Threadify Engine
const connection = await Threadify.connect('your-api-key', 'my-service');

// Start a new thread
const thread = await connection.start();

// Create and execute a step
await thread.step('process_order')
  .addContext({ orderId: '12345', amount: '100.00' })
  .stop('success');

// Close connection
await connection.close();
```

### Contract-Based Workflow

```javascript
import { Threadify } from 'threadify-sdk';

const connection = await Threadify.connect('api-key', 'merchant-service');

// Start thread with contract
const thread = await connection.start('product_delivery', 'merchant-service');

// Execute entry point step
await thread.step('order_placed')
  .addContext({ 
    order_id: 'ORD-001',
    product_id: 'PROD-123',
    quantity: '2'
  })
  .stop('success');

await connection.close();
```

---

## Core Concepts

### 1. **Connection**
A WebSocket connection to the Threadify Engine. Manages authentication and message routing.

### 2. **Thread**
A workflow execution instance. Can be contract-based (with validation rules) or non-contract (free-form).

### 3. **Step**
An atomic unit of work within a thread. Steps have:
- **Name**: Identifies the step type
- **Context**: Business data associated with the step
- **Status**: `in_progress`, `success`, `failed`, `error`, `skipped`
- **Idempotency**: Automatic deduplication based on name + context

### 4. **Contract**
A YAML-defined workflow specification that enforces:
- Entry point validation
- Step existence checks
- Required business context fields
- Role-based access control
- Step transitions (future)

---

## API Reference

## Threadify Class

Main entry point for the SDK.

### `Threadify.connect(apiKey, serviceName, options)`

Establishes a WebSocket connection to the Threadify Engine.

**Parameters:**
- `apiKey` (string, required): Your API key for authentication
- `serviceName` (string, optional): Service identifier (e.g., 'merchant-service')
- `options` (object, optional):
  - `url` (string): WebSocket URL (default: `ws://localhost:8081/threads`)

**Returns:** `Promise<Connection>`

**Example:**
```javascript
const connection = await Threadify.connect(
  'api-key-123',
  'payment-service',
  { url: 'ws://production.example.com/threads' }
);
```

---

### `Threadify.create(config)`

Creates a reusable Threadify instance with pre-configured settings.

**Parameters:**
- `config` (object):
  - `apiKey` (string, required)
  - `url` (string, required)
  - `serviceName` (string, optional)

**Returns:** Object with `connect()` method

**Example:**
```javascript
const threadify = Threadify.create({
  apiKey: 'api-key-123',
  url: 'ws://localhost:8081/threads',
  serviceName: 'order-service'
});

const connection = await threadify.connect();
```

---

## Connection Class

Represents an active WebSocket connection.

### `connection.start(...args)`

Starts a new thread.

**Signatures:**
1. `start()` - Non-contract workflow
2. `start(serviceName)` - Non-contract with specific service
3. `start(contractName, serviceName)` - Contract-based workflow

**Parameters:**
- `contractName` (string, optional): Contract name or "name:version"
- `serviceName` (string, optional): Service name for this thread

**Returns:** `Promise<ThreadInstance>`

**Examples:**
```javascript
// Non-contract
const thread1 = await connection.start();

// Non-contract with service
const thread2 = await connection.start('payment-service');

// Contract-based
const thread3 = await connection.start('product_delivery', 'merchant-service');

// Contract with version
const thread4 = await connection.start('product_delivery:v2', 'merchant-service');
```

---

### `connection.join(tokenOrThreadId, role)`

Joins an existing thread using an invitation token or direct join.

**Parameters:**
- `tokenOrThreadId` (string, required): JWT invitation token OR threadId
- `role` (string, optional): Role for direct join (internal services only)

**Returns:** `Promise<ThreadInstance>`

**Examples:**
```javascript
// Token-based join (external party)
const thread = await connection.join('eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...');

// Direct join (internal service)
const thread = await connection.join('thread-uuid-123', 'logistics');
```

---

### `connection.step(stepName, serviceName, options)`

Creates a step without a thread context (legacy method, prefer `thread.step()`).

**Parameters:**
- `stepName` (string, required): Name of the step
- `serviceName` (string, optional): Service executing the step
- `options` (object, optional):
  - `external_refs` (object): External system references

**Returns:** `ThreadStep`

---

### `connection.onViolation(stepName, handler)`

Register a global handler for validation violation notifications.

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

**Note:** Handlers are global and will receive notifications for ALL threads. Use `notification.threadId` to filter if needed.

---

### `connection.onCompleted(stepName, handler)`

Register a global handler for step completion notifications.

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

Register a global handler for step failure notifications.

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

### `connection.close()`

Closes the WebSocket connection gracefully.

**Returns:** `Promise<void>`

**Example:**
```javascript
await connection.close();
```

---

## ThreadInstance Class

Represents a specific thread execution.

### `thread.step(stepName, serviceName, options)`

Creates a new step in this thread.

**Parameters:**
- `stepName` (string, required): Name of the step
- `serviceName` (string, optional): Service executing the step
- `options` (object, optional):
  - `external_refs` (object): External system references

**Returns:** `ThreadStep`

**Example:**
```javascript
const step = thread.step('validate_payment', 'payment-service', {
  external_refs: { stripe_payment_id: 'pi_123' }
});
```

---

### `thread.inviteParty(options)`

Creates an invitation token for external parties to join this thread.

**Parameters:**
- `options` (object):
  - `role` (string, required): Role for the invited party
  - `permissions` (string, optional): Comma-separated permissions (default: "read,write")
  - `expiresIn` (string, optional): Expiry duration (default: "24h")

**Returns:** `Promise<string>` - JWT invitation token

**Example:**
```javascript
const token = await thread.inviteParty({
  role: 'logistics',
  permissions: 'read,write',
  expiresIn: '48h'
});

console.log('Share this token:', token);
```

**Note:** This method uses WebSocket communication. The invitation token can be used with `connection.join(token)` to allow other parties to join the thread.

---

### `thread.getThreadId()`

Returns the thread's unique identifier.

**Returns:** `string`

---

### `thread.getContractId()`

Returns the contract ID if this is a contract-based thread.

**Returns:** `string | null`

---

### `thread.waitFor(stepName, options)`

Wait for a notification for a specific step on this thread (blocking). This is useful for synchronous workflows where you need to wait for validation results before proceeding.

**Parameters:**
- `stepName` (string, required): Name of the step to wait for
- `options` (object, optional):
  - `timeout` (number): Timeout in milliseconds (default: 5000)
  - `statuses` (array): Only resolve for these statuses (e.g., `['success', 'failed']`)

**Returns:** `Promise<Notification>`

**Example:**
```javascript
// Record a step
await thread.step('payment_authorization')
  .addContext({ amount: '500.00' })
  .stop('success');

// Wait for validation notification (auto-ACKs on resolution)
const result = await thread.waitFor('payment_authorization', {
  timeout: 10000,
  statuses: ['success', 'failed']
});

if (result.isPassed()) {
  console.log('Validation passed!');
} else {
  console.error('Validation failed:', result.message);
}
```

**Note:** `waitFor()` automatically ACKs the notification when the promise resolves. This is ideal for synchronous flows where you need the validation result before continuing.

---

### `thread.close()`

Closes this thread instance and rejects any pending `waitFor()` promises.

**Returns:** `Promise<void>`

---

## ThreadStep Class

Represents a step execution with fluent API.

### `step.addContext(contextData, isPrivate)`

Adds business context data to the step.

**Parameters:**
- `contextData` (object, required): Key-value pairs of context data
- `isPrivate` (boolean, optional): Whether context is private (default: false)

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.addContext({
  order_id: 'ORD-123',
  customer_id: 'CUST-456',
  amount: '99.99'
});
```

**Note:** All values are automatically converted to strings as required by the server schema.

---

### `step.addRefs(refsData)`

Adds references to external systems.

**Parameters:**
- `refsData` (object, required): Key-value pairs of external references

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.addRefs({
  stripe_payment_id: 'pi_123abc',
  shopify_order_id: '987654'
});
```

---

### `step.idempotencyKey(key)`

Sets a manual idempotency key (overrides auto-generated key).

**Parameters:**
- `key` (string, required): Custom idempotency key

**Returns:** `ThreadStep` (for chaining)

**Example:**
```javascript
step.idempotencyKey('order-123-payment-attempt-1');
```

**Default Behavior:** If not set, the SDK automatically generates an idempotency key using FNV-1a hash of `stepName + context`.

---

### `step.stop(status, message, finalContext)`

Completes the step and sends it to the server.

**Parameters:**
- `status` (string, optional): Final status - `'success'`, `'failed'`, `'error'`, `'skipped'` (default: 'success')
- `message` (string, optional): Completion message
- `finalContext` (object, optional): Additional context to add before completion

**Returns:** `Promise<ThreadStep>`

**Example:**
```javascript
await step.stop('success', 'Payment processed', { 
  transaction_id: 'txn_789' 
});
```

---

### `step.success(message, result)`

Convenience method to complete step with success status.

**Parameters:**
- `message` (string, optional): Success message (default: 'Step completed successfully')
- `result` (object, optional): Result data

**Returns:** `Promise<ThreadStep>`

**Example:**
```javascript
await step.success('Order validated', { 
  validation_id: 'val_123' 
});
```

---

### `step.failed(message, error)`

Convenience method to complete step with failed status.

**Parameters:**
- `message` (string, optional): Failure message (default: 'Step failed')
- `error` (object, optional): Error data

**Returns:** `Promise<ThreadStep>`

**Example:**
```javascript
await step.failed('Insufficient inventory', { 
  requested: '10',
  available: '5'
});
```

---

### `step.error(message, error)`

Convenience method to complete step with error status.

**Parameters:**
- `message` (string, optional): Error message (default: 'Step failed with error')
- `error` (object, optional): Error details

**Returns:** `Promise<ThreadStep>`

**Example:**
```javascript
await step.error('Payment gateway timeout', { 
  gateway: 'stripe',
  error_code: 'TIMEOUT'
});
```

---

### Getter Methods

- `step.getStepName()` - Returns step name
- `step.getStatus()` - Returns current status
- `step.getContext()` - Returns copy of context data
- `step.getEventData()` - Returns complete event data (for debugging)

---

## Notification Class

Represents a validation notification received from the server.

### `notification.ack()`

Acknowledge a notification (mark as read/processed). This tells the server that the notification was successfully received and processed.

**Returns:** `void`

**Example:**
```javascript
connection.onViolation('payment_processing', (notification) => {
  console.error('Violation:', notification.message);
  notification.ack(); // Send ACK to server
});
```

**Note:** For `waitFor()`, ACK is sent automatically when the promise resolves. For event handlers (`onViolation`, `onCompleted`, `onFailed`), you must call `ack()` manually.

---

### Notification Properties

**Core Fields:**
- `notificationId` (string) - Unique notification ID
- `threadId` (string) - Thread this notification belongs to
- `stepName` (string) - Name of the step
- `stepStatus` (string) - Step status (`success`, `failed`, `error`)
- `status` (string) - Validation status (`passed`, `violated`)
- `severity` (string) - Severity level (`critical`, `warning`, `info`)
- `message` (string) - Human-readable message
- `violationType` (string) - Type of violation (if applicable)
- `details` (object) - Additional details about the notification

---

### Notification Helper Methods

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

## Advanced Usage

### Fluent API Chaining

```javascript
await thread.step('process_payment')
  .addContext({ amount: '100.00', currency: 'USD' })
  .addRefs({ stripe_id: 'pi_123' })
  .idempotencyKey('payment-order-123')
  .success('Payment completed');
```

### Error Handling with Idempotency

The SDK automatically handles duplicate step submissions:

```javascript
try {
  await thread.step('order_placed')
    .addContext({ order_id: 'ORD-123' })
    .stop('success');
} catch (error) {
  if (error.isDuplicate) {
    console.log('Step already recorded (idempotent)');
    // Continue execution - this is expected behavior
  } else {
    throw error; // Actual error
  }
}
```

### Multi-Party Workflows

```javascript
// Merchant service starts thread
const merchantConn = await Threadify.connect('api-key', 'merchant-service');
const thread = await merchantConn.start('product_delivery', 'merchant-service');

await thread.step('order_placed')
  .addContext({ order_id: 'ORD-123' })
  .success();

// Create invitation for logistics
const token = await thread.inviteParty({ role: 'logistics' });

// Logistics service joins
const logisticsConn = await Threadify.connect('api-key', 'logistics-service');
const logisticsThread = await logisticsConn.join(token);

await logisticsThread.step('shipment_created')
  .addContext({ tracking_number: 'TRACK-456' })
  .success();
```

### Contract Validation

When using contracts, the server validates:

1. **Entry Point**: First step must be a contract entry point
2. **Step Existence**: Step name must exist in contract
3. **Required Context**: All required business context fields must be present
4. **Role Validation**: Service role must match step's allowed roles

**Example Contract (YAML):**
```yaml
name: product_delivery
version: "1.0"
entry_points:
  - order_placed
steps:
  order_placed:
    roles: [merchant]
    business_context:
      required: [order_id, product_id, quantity]
      optional: [customer_notes]
```

**SDK Usage:**
```javascript
// ✅ Valid - entry point with required context
await thread.step('order_placed')
  .addContext({
    order_id: 'ORD-123',
    product_id: 'PROD-456',
    quantity: '2'
  })
  .success();

// ❌ Invalid - missing required context field 'quantity'
await thread.step('order_placed')
  .addContext({
    order_id: 'ORD-123',
    product_id: 'PROD-456'
  })
  .success(); // Server will reject with validation error
```

---

## Error Handling

### Common Errors

**1. Connection Errors**
```javascript
try {
  const connection = await Threadify.connect('invalid-key');
} catch (error) {
  console.error('Connection failed:', error.message);
  // "WebSocket error: ..." or "Connection timeout"
}
```

**2. Validation Errors**
```javascript
try {
  await thread.step('invalid_step').success();
} catch (error) {
  console.error('Validation failed:', error.message);
  // "Step 'invalid_step' does not exist in contract"
}
```

**3. Idempotency (Not an Error)**
```javascript
// First call - succeeds
await thread.step('order_placed')
  .addContext({ order_id: 'ORD-123' })
  .success();

// Second call with same context - silently ignored (idempotent)
await thread.step('order_placed')
  .addContext({ order_id: 'ORD-123' })
  .success(); // No error thrown, returns immediately
```

---

## Examples

### Example 1: Simple Order Processing

```javascript
import { Threadify } from 'threadify-sdk';

async function processOrder(orderId) {
  const connection = await Threadify.connect('api-key', 'order-service');
  
  try {
    const thread = await connection.start();
    
    // Validate order
    await thread.step('validate_order')
      .addContext({ order_id: orderId })
      .success('Order validated');
    
    // Process payment
    await thread.step('process_payment')
      .addContext({ order_id: orderId, amount: '99.99' })
      .addRefs({ stripe_payment_id: 'pi_123' })
      .success('Payment processed');
    
    // Ship order
    await thread.step('ship_order')
      .addContext({ order_id: orderId })
      .success('Order shipped');
    
    console.log('Order processed:', thread.getThreadId());
  } finally {
    await connection.close();
  }
}
```

### Example 2: Contract-Based Workflow with Error Handling

```javascript
import { Threadify } from 'threadify-sdk';

async function deliverProduct(orderData) {
  const connection = await Threadify.connect('api-key', 'merchant-service');
  
  try {
    const thread = await connection.start('product_delivery', 'merchant-service');
    
    // Entry point step
    await thread.step('order_placed')
      .addContext({
        order_id: orderData.orderId,
        product_id: orderData.productId,
        quantity: String(orderData.quantity)
      })
      .success();
    
    // Inventory check
    const inventoryAvailable = await checkInventory(orderData.productId);
    
    if (!inventoryAvailable) {
      await thread.step('order_cancelled')
        .addContext({ reason: 'insufficient_inventory' })
        .failed('Not enough inventory');
      return;
    }
    
    // Continue workflow...
    await thread.step('payment_processed')
      .addContext({ amount: orderData.amount })
      .success();
    
  } catch (error) {
    console.error('Workflow error:', error);
    throw error;
  } finally {
    await connection.close();
  }
}
```

### Example 3: Multi-Service Collaboration

```javascript
// Merchant Service
async function merchantFlow() {
  const conn = await Threadify.connect('api-key', 'merchant-service');
  const thread = await conn.start('product_delivery', 'merchant-service');
  
  await thread.step('order_placed')
    .addContext({ order_id: 'ORD-123', product_id: 'PROD-456' })
    .success();
  
  // Invite logistics partner
  const token = await thread.inviteParty({ role: 'logistics' });
  console.log('Invitation token:', token);
  
  return { threadId: thread.getThreadId(), token };
}

// Logistics Service
async function logisticsFlow(invitationToken) {
  const conn = await Threadify.connect('api-key', 'logistics-service');
  const thread = await conn.join(invitationToken);
  
  await thread.step('shipment_created')
    .addContext({ tracking_number: 'TRACK-789' })
    .success();
  
  await thread.step('in_transit')
    .addContext({ location: 'warehouse' })
    .success();
  
  await conn.close();
}
```

---

## Best Practices

1. **Always close connections**: Use `try/finally` to ensure `connection.close()` is called
2. **Use idempotency keys for critical steps**: Prevents duplicate processing in case of retries
3. **Add meaningful context**: Include all relevant business data for audit and debugging
4. **Handle validation errors**: Contract-based workflows will reject invalid steps
5. **Use external refs**: Link to external systems (payment IDs, order IDs, etc.)
6. **Leverage fluent API**: Chain methods for cleaner, more readable code

---

## Troubleshooting

### WebSocket Connection Issues

**Problem:** Connection timeout or fails to connect

**Solution:**
- Verify the WebSocket URL is correct
- Check that the Threadify Engine is running
- Ensure API key is valid
- Check firewall/network settings

### Validation Errors

**Problem:** Steps are rejected with validation errors

**Solution:**
- Verify step name exists in contract
- Ensure all required business context fields are provided
- Check that the service role matches the step's allowed roles
- Confirm the first step is an entry point

### Idempotency Issues

**Problem:** Steps are silently ignored

**Solution:**
- This is expected behavior for duplicate submissions
- Check if the step was already recorded with the same context
- Use manual idempotency keys if you need different behavior

---

## API Endpoint Reference

The SDK communicates with these WebSocket actions:

- `connect` - Establish connection
- `startThread` - Create new thread
- `joinThread` - Join existing thread
- `recordThreadEvent` - Submit step event
- `inviteParty` - Create invitation token (not yet implemented on server)
- `closeConnection` - Close connection

---

## Support

For issues, questions, or contributions:
- GitHub: [ThreadifyEngine Repository]
- Documentation: This file
- Examples: See `/tests/e2e-validation.test.js`

---

## License

[Your License Here]
