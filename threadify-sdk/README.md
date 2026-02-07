# Threadify SDK Documentation

Build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

## Installation

```bash
npm install @threadify/sdk
```

### Module Support

The SDK supports both ES Modules and CommonJS:

| Module System | Import Syntax |
|---------------|---------------|
| ES Modules (recommended) | `import { Threadify } from '@threadify/sdk';` |
| CommonJS | `const { Threadify } = require('@threadify/sdk');` |

## Table of Contents

1. [Quick Start](#quick-start)
2. [Core Concepts](#core-concepts)
3. [Common Scenarios](#common-scenarios)
   - [Track a Simple Workflow](#track-a-simple-workflow)
   - [Link to External Systems](#link-to-external-systems)
   - [Handle Failures](#handle-failures)
   - [Work with Contracts](#work-with-contracts)
   - [Inviting Others to a Thread](#inviting-others-to-a-thread)
   - [Joining a Thread](#joining-a-thread)
   - [Working with Joined Threads](#working-with-joined-threads)
4. [Data Retrieval](#data-retrieval)
   - [Connection Methods](#connection-methods)
   - [ArchivedThread Methods](#archivedthread-methods)
   - [ArchivedStep Methods](#archivedstep-methods)
5. [Real-Time Notifications](#real-time-notifications)
   - [Subscribing to Notifications](#subscribing-to-notifications)
   - [Notification Object](#notification-object)
   - [Flow Control & HPA Support](#flow-control--hpa-support)
6. [Data Structures](#data-structures)
   - [ArchivedThread Structure](#archivedthread-structure)
   - [ArchivedStep Structure](#archivedstep-structure)
   - [ValidationResult Structure](#validationresult-structure)
   - [StepHistory Structure](#stephistory-structure)
   - [Complete Thread Data Structure](#complete-thread-data-structure)
7. [Support](#support)

---

## Quick Start

```javascript
import { Threadify } from '@threadify/sdk';

// Connect with your API key
const connection = await Threadify.connect('your-api-key', 'my-service');

// Start tracking a workflow
const thread = await connection.start();

// Record steps with context
await thread.step('order_placed')
  .addContext({ orderId: 'ORD-12345', amount: 99.99 })
  .success();

await thread.step('payment_processed')
  .addContext({ paymentId: 'PAY-67890' })
  .success();
```

> Works with both ES Modules and CommonJS. For CommonJS, use `require('@threadify/sdk')` and wrap in an async function.

---

## Core Concepts

| Concept | Description | Key Features |
|---------|-------------|-------------|
| **Connection** | WebSocket connection to Threadify Engine | Authentication, message routing |
| **Thread** | Workflow execution instance | Contract-based (with validation) or free-form |
| **Step** | Atomic unit of work within a thread | Name, context, status (`success`/`failed`), automatic idempotency |
| **Contract** | YAML-defined workflow specification | Entry points, transitions, required fields, RBAC. Use `contractName` (latest) or `contractName:version` (e.g., `order_fulfillment:2`) |


## Common Scenarios

### Track a Simple Workflow

```javascript
const connection = await Threadify.connect('your-api-key');
const thread = await connection.start();

// Create step first, then execute business logic
const orderStep = thread.step('order_received');
const order = await receiveOrder();
await orderStep.addContext({ orderId: 'ORD-123', total: 299.99 }).success();

const inventoryStep = thread.step('inventory_checked');
const inventory = await checkInventory(order.items);
await inventoryStep.addContext({ inStock: true, warehouse: 'US-EAST' }).success();
```

### Link to External Systems

```javascript
// Connect your workflow to Stripe, Shopify, etc.
const paymentStep = thread.step('process_payment');
const payment = await processStripePayment();
await paymentStep
  .addContext({ amount: 299.99, currency: 'USD' })
  .addRefs({
    stripe_payment_id: payment.id,
    shopify_order_id: '12345'
  })
  .success();

// Now you can trace from Stripe back to your workflow instantly
```

### Handle Failures

```javascript
// Create step first to track execution time
const paymentStep = thread.step('payment_processed');
try {
  const payment = await processPayment(orderId);
  await paymentStep.addContext(payment).success();
} catch (error) {
  await paymentStep.failed({ 
    message: 'Payment gateway timeout',
    errorCode: 'TIMEOUT'
  });
}
```

**Step Completion Options:**

| Method | Parameter | Example |
|--------|-----------|----------|
| `.success()` | None | `await step.success()` |
| `.success(message)` | String | `await step.success('Order placed')` |
| `.success(data)` | Object | `await step.success({ transactionId: 'txn_123' })` |
| `.failed(data)` | Object | `await step.failed({ message: 'Timeout', code: 'ERR_TIMEOUT' })` |

### Work with Contracts

Contracts enforce workflow structure and validate steps automatically.

```javascript
// Start with latest version
const thread = await connection.start('order_fulfillment', 'merchant');

// Or use specific version
const thread = await connection.start('order_fulfillment:2', 'merchant');

// Contract validates entry points, required fields, and transitions
const orderStep = thread.step('order_placed');
const order = await createOrder();
await orderStep.addContext({ 
  order_id: order.id,
  product_id: order.productId,
  quantity: order.quantity
}).success();
```

### Inviting Others to a Thread

```javascript
const thread = await connection.start('order_fulfillment', 'merchant');

const invitation = await thread.inviteParty({
  role: 'logistics',
  accessLevel: 'participant',
  expiresIn: '48h'
});

// Share: invitation.token
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `role` | string | required | Business/contract role |
| `accessLevel` | string | `'external'` | System permission level |
| `expiresIn` | string | `'24h'` | Token expiration |

---

### Joining a Thread

**Two methods:**

| Method | Parameters | Use Case |
|--------|------------|----------|
| Token-based | `join(token)` | External partners with invitation |
| Direct | `join(threadId, role)` | Internal services, same organization |

```javascript
// Token-based (external)
const thread = await connection.join(invitationToken);

// Direct (internal)
const thread = await connection.join('thread-uuid-123', 'logistics');
```

---

### Working with Joined Threads

```javascript
// Record steps normally
const shipmentStep = thread.step('shipment_created');
const shipment = await createShipment();
await shipmentStep.addContext({ trackingNumber: shipment.trackingNumber }).success();

// Access thread properties
console.log(thread.id, thread.role, thread.accessLevel);
```

**Access Levels:**

| Level | Permissions | Notifications | Use Case |
|-------|-------------|---------------|----------|
| `owner` | Full control | All | Thread creator |
| `participant` | Read, write, execute | Critical + own | Active collaborators |
| `observer` | Read-only | Completions | Auditors, viewers |
| `external` | Own data only | Own completions/failures | External partners |

---

## Data Retrieval

Query archived thread data, step history, and validation results.

### Connection Methods

#### `connection.getThread(threadId)`

Get a thread by ID with access to all its data.

```javascript
const thread = await connection.getThread('thread-uuid-123');
// thread.id, thread.status, thread.contractName
```

**Returns:** [`Promise<ArchivedThread>`](#archivedthread-structure)

---

#### `connection.getThreadsByRef(options)`

Find threads by external reference with optional server-side filtering.

```javascript
const threads = await connection.getThreadsByRef({ refKey: 'orderId', refValue: 'ORDER-12345' });

// With filters: status, time range, pagination
const filtered = await connection.getThreadsByRef({ 
  refKey: 'orderId', 
  refValue: 'ORDER-12345',
  status: 'completed',
  startedAfter: '2026-01-01T00:00:00Z',
  limit: 10
});
```

**Returns:** [`Promise<Array<ArchivedThread>>`](#archivedthread-structure)

---

### ArchivedThread Methods

#### `thread.steps(stepIdentifier, options)`

Get all steps for this thread, optionally filtered.

**Usage:**

| Usage | Code | Returns |
|-------|------|----------|
| All steps | `thread.steps()` | All steps |
| By name | `thread.steps('order_placed')` | Filtered array |
| By name + key | `thread.steps('order_placed:order-123')` | Specific step |
| With options | `thread.steps('order_placed', {status: 'success'})` | Filtered array |

```javascript
const steps = await thread.steps('order_placed');
const step = steps[0]; // Get first result
```

**Returns:** [`Promise<Array<ArchivedStep>>`](#archivedstep-structure)

---

#### `thread.validationResults(options)`

Get validation results (contract violations, timeouts, etc.) for this thread.

```javascript
const validations = await thread.validationResults({ limit: 10 });
validations.forEach(v => {
  if (v.hasCriticalViolation) {
    console.log(`${v.stepName}: ${v.validations[0].message}`);
  }
});
```

**Returns:** [`Promise<Array<ValidationResult>>`](#validationresult-structure)

---

#### `thread.getCompleteData(options)`

Get complete thread data with all nested steps, history, and validations in a single request.

```javascript
const data = await thread.getCompleteData({ stepHistoryLimit: 50, validationLimit: 10 });

console.log(`Thread: ${data.id}`);
console.log(`Status: ${data.status}`);
console.log(`Steps: ${data.steps.length}`);
console.log(`Validations: ${data.validationResults.length}`);

// Access nested data
data.steps.forEach(step => {
  console.log(`${step.stepName}: ${step.status}`);
  step.history.forEach(h => {
    console.log(`  ${h.timestamp}: ${h.status}`);
  });
});
```

**Returns:** [Complete thread object](#complete-thread-data-structure) with nested steps, history, and validations

**Benefits:**
- Single network request
- Atomic data snapshot
- Perfect for dashboards and audit trails

---

### ArchivedStep Methods

#### `step.history(options)`

Get execution history for this step.

```javascript
const steps = await thread.steps('order_placed');
const step = steps[0];
const history = await step.history({ limit: 100 }); // all history
const filtered = await step.history({ limit: 10, activityType: 'step_recorded', startAt: '2026-01-01T00:00:00Z' }); // filtered
```

**Returns:** [`Promise<Array<StepHistory>>`](#stephistory-structure)

**Important:** The `status` field in step history shows the **step execution status** (`success` or `failed`) that you set when recording the step. Contract violations are tracked separately in `thread.validationResults()` and do **not** change the step status. A step can have `status: 'success'` but still have validation violations.

---

### Complete Thread Audit Trail

```javascript
import { Threadify } from 'threadify-sdk';

const connection = await Threadify.connect('api-key', 'audit-service');

// Get complete thread picture in one query
const thread = await connection.getThread('thread-uuid');
const completeData = await thread.getCompleteData({
  stepHistoryLimit: 100,
  validationLimit: 50
});

// Generate audit report
console.log('=== Thread Audit Report ===');
console.log(`Thread ID: ${completeData.id}`);
console.log(`Contract: ${completeData.contractName} v${completeData.contractVersion}`);
console.log(`Status: ${completeData.status}`);
console.log(`Duration: ${new Date(completeData.completedAt) - new Date(completeData.startedAt)}ms`);

console.log('\n=== Steps ===');
completeData.steps.forEach(step => {
  console.log(`\n${step.stepName}:${step.idempotencyKey}`);
  console.log(`  Status: ${step.status}`);
  console.log(`  Retries: ${step.retryCount}`);
  console.log(`  History:`);
  step.history.forEach(h => {
    console.log(`    ${h.timestamp}: ${h.status} (${h.duration}ms)`);
  });
});

console.log('\n=== Validations ===');
completeData.validationResults.forEach(val => {
  if (val.hasCriticalViolation) {
    console.log(`${val.stepName}: ${val.criticalCount} critical issues`);
  }
});
```

### Find Threads by Reference

```javascript
// Find all threads for a specific order
const threads = await connection.getThreadsByRef({
  refKey: 'orderId',
  refValue: 'ORDER-12345'
});

console.log(`Found ${threads.length} threads for order ORDER-12345`);

for (const thread of threads) {
  const data = await thread.getCompleteData();
  console.log(`Thread ${data.id}: ${data.status}`);
  console.log(`  Steps: ${data.steps.length}`);
  console.log(`  Started: ${data.startedAt}`);
}
```

### Step-Level Analysis

```javascript
const thread = await connection.getThread('thread-uuid');
const steps = await thread.steps('payment_processing');
const step = steps[0]; // Get first matching step

// Get detailed history
const history = await step.history({ limit: 50 });

console.log(`Payment Processing - ${history.length} attempts`);

const failures = history.filter(h => h.status === 'failed');
console.log(`Failed attempts: ${failures.length}`);

failures.forEach(f => {
  console.log(`  ${f.timestamp}: ${f.error}`);
});
```

### Understanding Step Status vs Violations

```javascript
const thread = await connection.getThread('thread-uuid');

// Step status shows execution outcome (what YOU set)
const steps = await thread.steps('order_placed');
console.log(`Step status: ${steps[0].status}`); // "success" or "failed"

// Validation results show contract violations (what Threadify detected)
const validations = await thread.validationResults();
validations.forEach(v => {
  console.log(`Validation: ${v.overallStatus}`); // "critical", "warning", or "info"
  console.log(`Step was: ${v.stepName}`);
});

// Example: A step can succeed but violate contract rules
// - Step status: "success" (payment processed successfully)
// - Validation: "critical" (invalid transition, missing required field, timeout, etc.)
```

---

## Real-Time Notifications

Threadify provides a push-based notification system for real-time validation alerts. Notifications are delivered via WebSocket with automatic deduplication and flow control.

### Connecting with Notifications

Notifications are enabled automatically when you connect. Use the `maxInFlight` option to control flow (default: 10, max: 100).

---

### Subscribing to Notifications

Subscribe to events using the `.on()` method:

**Event Patterns:**

| Pattern | Triggers On | Example |
|---------|-------------|----------|
| `step.success` | Step succeeded | Order completed |
| `step.failed` | Step failed | Payment declined |
| `rule.violated` | Contract violation | Invalid transition |
| `rule.passed` | Validation passed | All rules satisfied |
| `step.*` | Any step event | All step outcomes |
| `rule.*` | Any validation | All contract checks |
| `*` | All events | Everything |

```javascript
connection.on('step.success', 'order_placed', (notification) => {
  console.log('Order placed successfully');
  notification.ack(); // Must ACK
});

connection.on('rule.violated', 'order_placed', (notification) => {
  console.error('Validation violation:', notification.message);
  notification.ack();
});

// Contract-specific
connection.on('rule.violated', 'product_delivery@order_placed', (notification) => {
  notification.ack();
});
```

---

### Notification Object

| Property | Type | Description |
|----------|------|-------------|
| `notificationId` | string | Unique notification ID |
| `threadId` | string | Thread ID |
| `stepId` | string | Step ID |
| `stepName` | string | Step name |
| `stepStatus` | string | `success` or `failed` |
| `status` | string | `passed` or `violated` |
| `violationType` | string | Type of violation (if any) |
| `severity` | string | `info`, `warning`, `critical` |
| `message` | string | Human-readable message |
| `timestamp` | string | ISO timestamp |
| `ack()` | method | Acknowledge notification |

---

### Notification Methods

#### `notification.ack()`

Acknowledge receipt and processing of the notification. **You must call this** to prevent redelivery.

```javascript
connection.on('rule.violated', 'order_placed', (notification) => {
  // Process the notification
  logToDatabase(notification);
  
  // ACK to confirm processing
  notification.ack();
});
```

**Important:**
- If you don't ACK within 30 seconds, the notification will be redelivered
- After 3 failed deliveries, the notification moves to the Dead Letter Queue
- ACK is idempotent - safe to call multiple times

---

### Flow Control & HPA Support

**Flow Control:** Set `maxInFlight` to limit pending notifications (prevents overwhelming client)

**HPA-Safe:** Each notification delivered to **exactly one pod** - no duplicate processing, automatic load balancing

**Error Handling:**
```javascript
connection.on('rule.violated', 'order_placed', async (notification) => {
  try {
    await processViolation(notification);
    notification.ack();  // ACK on success
  } catch (error) {
    // Don't ACK - notification redelivered after 30s (max 3 attempts)
  }
});
```

---

## Data Structures

Detailed structure of objects returned by data retrieval methods.

### ArchivedThread Structure

Returned by `getThread()` and `getThreadsByRef()`.

```javascript
{
  id: string,
  contractId: string,
  contractVersion: string,
  contractName: string,
  ownerId: string,
  companyId: string,
  status: 'active' | 'completed' | 'failed',
  lastHash: string,
  refs: { [key: string]: string },  // External references
  startedAt: string,                 // ISO timestamp
  completedAt: string,               // ISO timestamp
  error: string                      // Error message if failed
}
```

---

### ArchivedStep Structure

Returned by `thread.steps()`.

```javascript
{
  threadId: string,
  stepName: string,
  idempotencyKey: string,
  status: 'success' | 'failed',
  retryCount: number,
  firstSeenAt: string,    // ISO timestamp
  lastUpdatedAt: string,  // ISO timestamp
  latestStepID: string,   // UUID of latest attempt
  previousStep: string    // Previous step name
}
```

---

### ValidationResult Structure

Returned by `thread.validationResults()`.

```javascript
{
  validationId: string,
  threadId: string,
  stepName: string,
  timestamp: string,      // ISO timestamp
  overallStatus: 'critical' | 'warning' | 'info',
  hasCriticalViolation: boolean,
  criticalCount: number,
  warningCount: number,
  validations: [{
    type: string,        // e.g., 'invalid_transition', 'timeout', 'missing_field'
    message: string,     // Human-readable description
    severity: 'critical' | 'warning' | 'info',
    field: string,       // Affected field (if applicable)
    expected: any,       // Expected value
    actual: any          // Actual value
  }]
}
```

---

### StepHistory Structure

Returned by `step.history()`.

```javascript
{
  attempt: number,        // Retry attempt number
  timestamp: string,      // ISO timestamp
  status: 'success' | 'failed',
  context: object,        // Business context data
  duration: number,       // Execution time in ms
  error: string          // Error message if failed
}
```

---

### Complete Thread Data Structure

Returned by `thread.getCompleteData()`.

```javascript
{
  // Thread metadata (same as ArchivedThread)
  id: string,
  contractId: string,
  contractVersion: string,
  contractName: string,
  ownerId: string,
  companyId: string,
  status: 'active' | 'completed' | 'failed',
  lastHash: string,
  refs: { [key: string]: string },
  startedAt: string,
  completedAt: string,
  error: string,
  
  // Nested step data
  steps: [{
    threadId: string,
    stepName: string,
    idempotencyKey: string,
    status: string,
    retryCount: number,
    firstSeenAt: string,
    lastUpdatedAt: string,
    latestStepID: string,
    previousStep: string,
    
    // Nested history for each step
    history: [{
      attempt: number,
      timestamp: string,
      status: string,
      context: object,
      duration: number,
      error: string
    }]
  }],
  
  // Nested validation results
  validationResults: [{
    validationId: string,
    threadId: string,
    stepName: string,
    timestamp: string,
    overallStatus: string,
    hasCriticalViolation: boolean,
    criticalCount: number,
    warningCount: number,
    validations: [{
      type: string,
      message: string,
      severity: string,
      field: string,
      expected: any,
      actual: any
    }]
  }]
}
```

---

## Support

For issues, questions, or contributions:
- Email: [support@threadify.dev](mailto:support@threadify.dev)

---
