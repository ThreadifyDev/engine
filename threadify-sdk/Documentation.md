# Threadify SDK Documentation

Build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

## Installation

```bash
npm install @threadify/sdk
```

## Quick Start

```javascript
import { Threadify } from '@threadify/sdk';

// Connect with your API key
const connection = await Threadify.connect('your-api-key', 'my-service');

// Start tracking a workflow
const thread = await connection.start();

// Record each step with full context
await thread.step('order_placed')
  .addContext({ orderId: 'ORD-12345', amount: 99.99 })
  .success();

await thread.step('payment_processed')
  .addContext({ paymentId: 'PAY-67890' })
  .success();
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
A YAML-defined workflow specification that enforces validation rules for your threads. Contracts define the structure and requirements of your workflows.

**What Contracts Provide:**
- **Entry Points**: Which steps can start a thread
- **Step Definitions**: Valid step names and their requirements
- **Required Context**: Business data fields that must be present
- **Role-Based Access**: Which services/roles can execute each step
- **Validation Rules**: Automatic checking of workflow correctness

**Contract Formats:**
- `contractName` - Uses the latest version
- `contractName:version` - Uses a specific version (e.g., `order_fulfillment:v2`)

**Example Contract (YAML):**
```yaml
name: order_fulfillment
version: "1.0"
entry_points:
  - order_placed
steps:
  order_placed:
    roles: [merchant]
    business_context:
      required: [order_id, product_id, quantity]
      optional: [customer_notes]
  payment_processed:
    roles: [merchant, payment_service]
    business_context:
      required: [payment_id, amount]
```

### 5. **Joining Threads**
There are two ways to join an existing thread:

**Token-Based Join (External Parties):**
- Used when inviting external partners/services outside your organization
- Requires an invitation token (JWT) created by the thread owner
- Token contains thread ID, role, permissions, and expiry
- Secure way to grant temporary access

**Direct Join (Internal Services):**
- Used by services within the same organization
- Only requires the thread ID and your role
- No token needed - authentication via API key
- Faster and simpler for internal collaboration

---

## Common Scenarios

### Track a Simple Workflow

```javascript
const connection = await Threadify.connect('your-api-key');
const thread = await connection.start();

// Each step is automatically validated and tracked
await thread.step('order_received')
  .addContext({ orderId: 'ORD-123', total: 299.99 })
  .success();

await thread.step('inventory_checked')
  .addContext({ inStock: true, warehouse: 'US-EAST' })
  .success();

await thread.step('payment_captured')
  .addContext({ paymentId: 'ch_abc123', amount: 299.99 })
  .success();
```

### Link to External Systems

```javascript
// Connect your workflow to Stripe, Shopify, etc.
await thread.step('process_payment')
  .addContext({ amount: 299.99, currency: 'USD' })
  .addRefs({
    stripe_payment_id: 'pi_abc123',
    shopify_order_id: '12345',
    customer_email: 'customer@example.com'
  })
  .success();

// Now you can trace from Stripe back to your workflow instantly
```

### Handle Failures Gracefully

```javascript
try {
  await processPayment(orderId);
  await thread.step('payment_processed')
    .addContext({ orderId, status: 'success' })
    .success();
} catch (error) {
  // Threadify tracks failures too
  await thread.step('payment_processed')
    .addContext({ orderId, error: error.message })
    .failed('Payment gateway timeout');
  
  // You'll get notified automatically if this violates your workflow rules
}
```

### Work with Contracts (Predefined Workflows)

Contracts enforce workflow structure and validate your steps automatically.

```javascript
// Start with latest version of contract
const thread = await connection.start('order_fulfillment', 'merchant');

// Or use a specific version
const thread2 = await connection.start('order_fulfillment:v2', 'merchant');

// Contract validates: entry point, required fields, role access
await thread.step('order_placed')
  .addContext({ 
    order_id: 'ORD-123',      // Required by contract
    product_id: 'PROD-456',   // Required by contract
    quantity: '2'             // Required by contract
  })
  .success();

// Contract ensures this is a valid next step for your role
await thread.step('payment_processed')
  .addContext({ 
    payment_id: 'PAY-789',
    amount: '99.99'
  })
  .success();
```

### Join an Existing Thread

Two ways to join depending on whether you're internal or external to the organization.

```javascript
// METHOD 1: Token-Based Join (for external partners/services)
// The thread owner creates an invitation token and shares it with you
const thread = await connection.join('eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...');

// METHOD 2: Direct Join (for internal services in same organization)
// You just need the thread ID and specify your role
const thread = await connection.join('thread-uuid-123', 'logistics');

// Once joined, continue the workflow
await thread.step('shipment_created')
  .addContext({ 
    trackingNumber: 'TRACK-456',
    carrier: 'FedEx'
  })
  .success();

// Creating an invitation token (if you're the thread owner):
const invitationToken = await thread.inviteParty({
  role: 'logistics',
  permissions: 'read,write',
  expiresIn: '48h'
});
// Share this token with external partner
```

---

## Table of Contents

1. [Core Concepts](#core-concepts) - Connection, Thread, Step, Contract
2. [Common Scenarios](#common-scenarios) - Quick examples to get started
   - [Track a Simple Workflow](#track-a-simple-workflow)
   - [Link to External Systems](#link-to-external-systems)
   - [Handle Failures](#handle-failures-gracefully)
   - [Work with Contracts](#work-with-contracts-predefined-workflows)
   - [Join an Existing Thread](#join-an-existing-thread)
3. [Data Retrieval API](#data-retrieval-api)
   - [Connection Methods](#connection-methods)
   - [ArchivedThread Methods](#archivedthread-methods)
   - [ArchivedStep Methods](#archivedstep-methods)
4. [Real-Time Notifications](#real-time-notifications)
   - [Subscribing to Notifications](#subscribing-to-notifications)
   - [Notification Object](#notification-object)
   - [Subscription Patterns](#subscription-patterns)
   - [Flow Control & HPA Support](#flow-control)
5. [Support](#support)

---

## Data Retrieval API

Threadify lets you build business process graphs with context—track what happened, validate every step, and trigger context-aware actions.

This SDK provides a concise, modern GraphQL-based API for accessing all archived thread data, step history, and validations.

### Connection Methods

#### `connection.getThread(threadId)`

Get a thread by ID with access to all its data.

**Parameters:**
- `threadId` (string, required): Thread ID

**Returns:** `Promise<ArchivedThread>`

**Example:**
```javascript
const thread = await connection.getThread('thread-uuid-123');
// thread.id, thread.status, thread.contractName
```

---

#### `connection.getThreadsByRef({ refKey, refValue, ...filters })`

Find threads by external reference with optional server-side filtering.

**Parameters:**
- `refKey` (string, required): Reference key (e.g., "orderId")
- `refValue` (string, required): Reference value (e.g., "ORDER-12345")
- `status` (string, optional): Filter by status ("active", "completed", etc.)
- `startedAfter` (string, optional): ISO timestamp - only threads started after this time
- `startedBefore` (string, optional): ISO timestamp - only threads started before this time
- `limit` (number, optional): Maximum results (default: 50)
- `offset` (number, optional): Pagination offset (default: 0)

**Returns:** `Promise<Array<ArchivedThread>>`

**Example:**
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

---

### ArchivedThread Methods

#### `thread.steps(filters)`

Get all steps for this thread, optionally filtered.

**Parameters:**
- `filters` (object, optional):
  - `stepName` (string): Filter by step name
  - `idempotencyKey` (string): Filter by idempotency key

**Returns:** `Promise<Array<ArchivedStep>>`

**Example:**
```javascript
const allSteps = await thread.steps(); // all steps
const stepsByName = await thread.steps({ stepName: 'order_placed' }); // filter by name
const stepsByNameAndIdemp = await thread.steps({ stepName: 'order_placed', idempotencyKey: 'order-123' }); // filter by name and idempKey
```

---

#### `thread.getStep(stepIdentifier)`

Get a specific step by name or "name:idempotencyKey".

**Parameters:**
- `stepIdentifier` (string, required): Step name or "stepName:idempKey"

**Returns:** `Promise<ArchivedStep>`

**Example:**
```javascript
const step = await thread.getStep('order_placed'); // by step name
const stepWithIdemp = await thread.getStep('order_placed:order-123'); // by stepName:idempKey
```

---

#### `thread.validationResults(options)`

Get validation results for this thread.

**Parameters:**
- `options` (object, optional):
  - `limit` (number): Maximum results to return
  - `stepName` (string): Filter by step name
  - `validationType` (string): Filter by validation type

**Returns:** `Promise<Array<ValidationResult>>`

**Example:**
```javascript
const validations = await thread.validationResults({ limit: 10 });
// validations is an array of ValidationResult objects
```

---

#### `thread.getCompleteData(options)`

Get complete thread picture with all nested data in a **single GraphQL query**. This is the most efficient way to retrieve all thread data.

**Parameters:**
- `options` (object, optional):
  - `stepHistoryLimit` (number): Limit for step history per step (default: 50)
  - `validationLimit` (number): Limit for validation results (default: 10)
  - `stepName` (string): Filter steps by name
  - `idempotencyKey` (string): Filter steps by idempotency key

**Returns:** `Promise<Object>` with structure:
```javascript
{
  id, contractId, contractVersion, contractName,
  ownerId, companyId, status, lastHash, refs,
  startedAt, completedAt, error,
  steps: [{
    threadId, stepName, idempotencyKey, status,
    retryCount, firstSeenAt, lastUpdatedAt,
    latestStepID, previousStep,
    history: [{ attempt, timestamp, status, context, duration, error }]
  }],
  validationResults: [{
    validationId, threadId, stepId, stepName,
    idempotencyKey, timestamp, overallStatus,
    hasCriticalViolation, criticalCount, warningCount,
    validations: [{ type, message, field, expected, actual, rule }]
  }]
}
```

**Example:**
```javascript
const completeData = await thread.getCompleteData({ stepHistoryLimit: 50, validationLimit: 10 });
// completeData.steps, completeData.validationResults, etc.
```

**Benefits:**
- ✅ Single network request (much faster)
- ✅ Atomic data snapshot
- ✅ Reduced server load
- ✅ Perfect for dashboards and audit trails

---

### ArchivedStep Methods

#### `step.history(options)`

Get execution history for this step.

**Parameters:**
- `options` (object, optional):
  - `limit` (number): Maximum records (default: 100)
  - `offset` (number): Pagination offset (default: 0)
  - `startAt` (string): ISO timestamp to filter from
  - `endAt` (string): ISO timestamp to filter to
  - `activityType` (string): Filter by activity type
  - `actor` (string): Filter by actor

**Returns:** `Promise<Array<StepHistory>>`

**Example:**
```javascript
const step = await thread.getStep('order_placed');
const history = await step.history({ limit: 100 }); // all history
const filtered = await step.history({ limit: 10, activityType: 'step_recorded', startAt: '2026-01-01T00:00:00Z' }); // filtered
```

---

## Data Retrieval Examples

### Example: Complete Thread Audit Trail

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
    console.log(`❌ ${val.stepName}: ${val.criticalCount} critical issues`);
  }
});
```

### Example: Find Threads by Reference

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

### Example: Step-Level Analysis

```javascript
const thread = await connection.getThread('thread-uuid');
const step = await thread.getStep('payment_processing');

// Get detailed history
const history = await step.history({ limit: 50 });

console.log(`Payment Processing - ${history.length} attempts`);

const failures = history.filter(h => h.status === 'failed');
console.log(`Failed attempts: ${failures.length}`);

failures.forEach(f => {
  console.log(`  ${f.timestamp}: ${f.error}`);
});
```

---

## Real-Time Notifications

Threadify provides a push-based notification system for real-time validation alerts. Notifications are delivered via WebSocket with automatic deduplication and flow control.

### Connecting with Notifications

Notifications are enabled automatically when you connect. Use the `maxInFlight` option to control flow (default: 10, max: 100).

---

### Subscribing to Notifications

Subscribe to validation events using these methods:

- **`connection.onViolation(stepName, handler)`** - Validation violations
- **`connection.onCompleted(stepName, handler)`** - Successful completions
- **`connection.onFailed(stepName, handler)`** - Step failures

**Parameters:**
- `stepName` (string): Step name or "contract@stepName" for contract-specific
- `handler` (function): Callback `(notification) => {}`

**Example:**
```javascript
// All contracts
connection.onViolation('order_placed', (notification) => {
  console.log('Violation:', notification.message);
  notification.ack(); // IMPORTANT: Must ACK
});

// Contract-specific
connection.onViolation('product_delivery@order_placed', (notification) => {
  console.log('Product delivery violation');
  notification.ack();
});
```

---

### Notification Object

Each notification has the following properties:

```javascript
{
  notificationId: 'uuid',           // Unique notification ID
  threadId: 'uuid',                 // Thread ID
  stepId: 'uuid',                   // Step ID
  stepName: 'order_placed',         // Step name
  ownerId: 'user-123',              // Owner ID
  contractName: 'product_delivery', // Contract name (or empty)
  stepStatus: 'success',            // Step status: success, failed, error
  status: 'violated',               // Validation status: passed, violated
  violationType: 'timeout',         // Type of violation (if any)
  severity: 'critical',             // Severity: info, warning, critical
  message: 'Step timeout exceeded', // Human-readable message
  details: {},                      // Additional details
  timestamp: '2026-01-19T...',      // ISO timestamp
  
  // Methods
  ack()                             // Acknowledge notification
}
```

---

### Notification Methods

#### `notification.ack()`

Acknowledge receipt and processing of the notification. **You must call this** to prevent redelivery.

**Example:**
```javascript
connection.onViolation('order_placed', (notification) => {
  // Process the notification
  logToDatabase(notification);
  
  // ACK to confirm processing
  notification.ack();
});
```

**Important:**
- ⚠️ If you don't ACK within 30 seconds, the notification will be redelivered
- ⚠️ After 3 failed deliveries, the notification moves to the Dead Letter Queue
- ✅ ACK is idempotent - safe to call multiple times

---

### Subscription Patterns

**Wildcard (all contracts):**
```javascript
connection.onViolation('order_placed', handler); // Any contract
```

**Contract-specific:**
```javascript
connection.onViolation('product_delivery@order_placed', handler); // Specific contract only
```

**Multiple events:**
```javascript
connection.onViolation('order_placed', handleViolation);
connection.onCompleted('order_placed', handleSuccess);
connection.onFailed('order_placed', handleFailure);
```

---

### Flow Control & HPA Support

**Flow Control:** Set `maxInFlight` to limit pending notifications (prevents overwhelming client)

**HPA-Safe:** Each notification delivered to **exactly one pod** - no duplicate processing, automatic load balancing

**Error Handling:**
```javascript
connection.onViolation('order_placed', async (notification) => {
  try {
    await processViolation(notification);
    notification.ack();  // ACK on success
  } catch (error) {
    // Don't ACK - notification redelivered after 30s (max 3 attempts)
  }
});
```

---

## Support

For issues, questions, or contributions:
- GitHub: [ThreadifyEngine Repository]
- Documentation: This file

---
